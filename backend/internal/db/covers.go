package db

import (
	"log/slog"
	"mujian/internal/models"
	"mujian/internal/storage"
	"strings"
)

// ---------- covers metadata table ----------

func (db *DB) UpsertCoverMeta(hash, fileName, ext string, size int64) error {
	_, err := db.conn.Exec(`
		INSERT INTO covers (hash, file_name, ext, size) VALUES (?, ?, ?, ?)
		ON CONFLICT(file_name) DO UPDATE SET hash = excluded.hash, ext = excluded.ext, size = excluded.size
	`, hash, fileName, ext, size)
	return err
}

func (db *DB) CoverMetaExists(fileName string) bool {
	var n int
	// A failed COUNT must not be reported as "meta missing": callers rely on
	// this to decide whether to re-import a cover, which would duplicate rows.
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM covers WHERE file_name = ?", fileName).Scan(&n); err != nil {
		slog.Warn("cover meta exists", "file", fileName, "err", err)
		return false
	}
	return n > 0
}

// GetCoverByHash returns one covers row for the given content hash.
func (db *DB) GetCoverByHash(hash string) (*models.Cover, error) {
	var c models.Cover
	err := db.conn.QueryRow(
		"SELECT hash, file_name, ext, size, created_at FROM covers WHERE hash = ? LIMIT 1", hash,
	).Scan(&c.Hash, &c.FileName, &c.Ext, &c.Size, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CoverSize returns the recorded size of a cover file.
func (db *DB) CoverSize(fileName string) (int64, bool) {
	var s int64
	err := db.conn.QueryRow("SELECT size FROM covers WHERE file_name = ?", fileName).Scan(&s)
	return s, err == nil
}

func (db *DB) DeleteCoverMeta(fileName string) error {
	_, err := db.conn.Exec("DELETE FROM covers WHERE file_name = ?", fileName)
	return err
}

// SyncCovers hashes every distinct cover_file referenced by records that is
// not yet in the covers table and upserts its metadata. Returns how many new
// entries were computed.
func (db *DB) SyncCovers(store storage.Storage) (int, error) {
	// Run the whole sync inside one transaction. Under MaxOpenConns(1) this
	// keeps a single connection busy for the entire operation and avoids the
	// "acquire connection while one is held by live rows" deadlock.
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT DISTINCT cover_file FROM records WHERE deleted_at = 0 AND cover_file != ''`)
	if err != nil {
		return 0, err
	}
	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			continue
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	added := 0
	for _, f := range files {
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM covers WHERE file_name = ?", f).Scan(&n); err != nil {
			continue
		}
		if n > 0 {
			continue
		}
		data, err := store.ReadCover(f)
		if err != nil {
			continue
		}
		hash := storage.HashBytes(data)
		ext := storage.DetectExt(data)
		if _, err := tx.Exec(`
			INSERT INTO covers (hash, file_name, ext, size) VALUES (?, ?, ?, ?)
			ON CONFLICT(file_name) DO UPDATE SET hash = excluded.hash, ext = excluded.ext, size = excluded.size
		`, hash, f, ext, int64(len(data))); err == nil {
			added++
		}
	}
	return added, tx.Commit()
}

// ---------- duplicates ----------

// GetDuplicateGroups returns groups of records whose covers share the same
// content hash (more than one record per hash).
func (db *DB) GetDuplicateGroups() ([]models.DupGroup, error) {
	rows, err := db.conn.Query(`
		SELECT r.id, r.name, r.cover_file, c.hash, c.ext, c.size
		FROM records r
		JOIN covers c ON c.file_name = r.cover_file
		WHERE r.deleted_at = 0 AND r.cover_file != '' AND c.hash != ''
		ORDER BY c.hash
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type groupAcc struct {
		g *models.DupGroup
	}
	groups := make(map[string]*models.DupGroup)
	for rows.Next() {
		var id, name, coverFile, hash, ext string
		var size int64
		if err := rows.Scan(&id, &name, &coverFile, &hash, &ext, &size); err != nil {
			continue
		}
		g, ok := groups[hash]
		if !ok {
			g = &models.DupGroup{Hash: hash, Ext: ext, Size: size, Canonical: "covers/" + hash + ext}
			groups[hash] = g
		}
		g.Count++
		g.Records = append(g.Records, models.DupRecord{ID: id, Name: name, CoverFile: coverFile})
	}

	out := make([]models.DupGroup, 0, len(groups))
	for _, g := range groups {
		// Only a real duplicate when the same content is stored under more
		// than one distinct file (after merging, all members share the
		// canonical file and this group should disappear).
		distinct := map[string]bool{}
		for _, r := range g.Records {
			distinct[r.CoverFile] = true
		}
		if len(distinct) > 1 {
			out = append(out, *g)
		}
	}
	return out, nil
}

// GetRecordsByCoverHash returns records referencing any file with the given
// content hash.
func (db *DB) GetRecordsByCoverHash(hash string) ([]models.DupRecord, error) {
	rows, err := db.conn.Query(`
		SELECT r.id, r.name, r.cover_file
		FROM records r
		JOIN covers c ON c.file_name = r.cover_file
		WHERE c.hash = ?
	`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DupRecord
	for rows.Next() {
		var d models.DupRecord
		if err := rows.Scan(&d.ID, &d.Name, &d.CoverFile); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// ---------- reference updates / counts ----------

// UpdateRecordsCoverFile repoints ids to a canonical cover file (single tx).
func (db *DB) UpdateRecordsCoverFile(ids []string, coverFile string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+2)
	args = append(args, coverFile)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, coverFile)
	inClause := "(" + strings.Join(placeholders, ",") + ")"
	res, err := db.conn.Exec(
		"UPDATE records SET cover_file = ? WHERE deleted_at = 0 AND id IN "+inClause+" AND cover_file != ?",
		args...,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (db *DB) CountCoverRefs(fileName string) (int, error) {
	var n int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at = 0 AND cover_file = ?", fileName).Scan(&n)
	return n, err
}

// RepointCoverRefs updates all records that reference oldKey to point to newKey.
func (db *DB) RepointCoverRefs(oldKey, newKey string) error {
	_, err := db.conn.Exec(
		"UPDATE records SET cover_file = ? WHERE cover_file = ?",
		newKey, oldKey,
	)
	return err
}

// GetRecordsByCoverFile returns records whose cover_file equals the given key.
func (db *DB) GetRecordsByCoverFile(coverFile string) ([]struct{ ID, CoverFile string }, error) {
	rows, err := db.conn.Query("SELECT id, cover_file FROM records WHERE deleted_at = 0 AND cover_file = ?", coverFile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, CoverFile string }
	for rows.Next() {
		var id, cf string
		if err := rows.Scan(&id, &cf); err != nil {
			continue
		}
		out = append(out, struct{ ID, CoverFile string }{id, cf})
	}
	return out, nil
}

func (db *DB) SetRecordThumb(id, thumb string) error {
	_, err := db.conn.Exec("UPDATE records SET cover_thumb = ? WHERE id = ?", thumb, id)
	return err
}

// ---------- reuse picker ----------

// ListCoverPicker returns distinct covers with reference counts and a sample
// record's name/category, filtered by q (name or category), paginated.
func (db *DB) ListCoverPicker(q string, limit, offset int) ([]models.CoverRef, int, error) {
	like := "%" + q + "%"
	base := `
		SELECT r.cover_file,
		       COUNT(*) AS ref_count,
		       (SELECT name FROM records r2 WHERE r2.cover_file = r.cover_file ORDER BY r2.date DESC LIMIT 1) AS sample_name,
		       (SELECT category_name FROM records r2 WHERE r2.cover_file = r.cover_file ORDER BY r2.date DESC LIMIT 1) AS sample_category
		FROM records r
		WHERE r.deleted_at = 0 AND r.cover_file != ''
		GROUP BY r.cover_file
		HAVING (? = '' OR sample_name LIKE ? OR sample_category LIKE ?)`

	var total int
	if err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM ("+base+")", q, like, like,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := db.conn.Query(
		base+" ORDER BY ref_count DESC LIMIT ? OFFSET ?", q, like, like, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []models.CoverRef
	for rows.Next() {
		var c models.CoverRef
		if err := rows.Scan(&c.FileName, &c.RefCount, &c.SampleName, &c.Category); err != nil {
			continue
		}
		if c.FileName == "" {
			continue
		}
		c.Ext = extOf(c.FileName)
		c.Size = 0
		if meta, ok := db.coverSize(c.FileName); ok {
			c.Size = meta
		}
		out = append(out, c)
	}
	if out == nil {
		out = []models.CoverRef{}
	}
	return out, total, nil
}

func (db *DB) coverSize(fileName string) (int64, bool) {
	var s int64
	err := db.conn.QueryRow("SELECT size FROM covers WHERE file_name = ?", fileName).Scan(&s)
	return s, err == nil
}

func extOf(fileName string) string {
	if i := strings.LastIndex(fileName, "."); i >= 0 {
		return fileName[i+1:]
	}
	return ""
}

// ListCoverFiles returns id + cover_file for every record that has a cover,
// used for thumbnail regeneration.
func (db *DB) ListCoverFiles() ([]struct{ ID, CoverFile string }, error) {
	rows, err := db.conn.Query(`SELECT id, cover_file FROM records WHERE deleted_at = 0 AND cover_file != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct{ ID, CoverFile string }
	for rows.Next() {
		var id, cf string
		if err := rows.Scan(&id, &cf); err != nil {
			continue
		}
		out = append(out, struct{ ID, CoverFile string }{id, cf})
	}
	return out, nil
}
