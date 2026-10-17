package db

import (
	"database/sql"

	"mujian/internal/models"
)

// 票根/现场照：图片本体走内容寻证的 covers/ 存储（与封面共用去重），
// 这里只维护记录 → 图片 key 的关联与顺序。

func (db *DB) ListRecordPhotos(recordID string) ([]models.RecordPhoto, error) {
	rows, err := db.conn.Query(
		"SELECT id, record_id, file_name, sort FROM record_photos WHERE record_id = ? ORDER BY sort ASC, id ASC",
		recordID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RecordPhoto{}
	for rows.Next() {
		var p models.RecordPhoto
		if err := rows.Scan(&p.ID, &p.RecordID, &p.FileName, &p.Sort); err == nil {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// AddRecordPhoto appends one photo association at the end of the list.
func (db *DB) AddRecordPhoto(recordID, fileName string) (models.RecordPhoto, error) {
	var sort int
	if err := db.conn.QueryRow("SELECT COALESCE(MAX(sort), 0) + 1 FROM record_photos WHERE record_id = ?", recordID).Scan(&sort); err != nil {
		return models.RecordPhoto{}, err
	}
	id := newID()
	if _, err := db.conn.Exec(
		"INSERT INTO record_photos (id, record_id, file_name, sort) VALUES (?, ?, ?, ?)",
		id, recordID, fileName, sort,
	); err != nil {
		return models.RecordPhoto{}, err
	}
	return models.RecordPhoto{ID: id, RecordID: recordID, FileName: fileName, Sort: sort}, nil
}

func (db *DB) DeleteRecordPhoto(recordID, photoID string) error {
	_, err := db.conn.Exec("DELETE FROM record_photos WHERE record_id = ? AND id = ?", recordID, photoID)
	return err
}

// ReorderRecordPhotos persists the display order (full id list).
func (db *DB) ReorderRecordPhotos(recordID string, ids []string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE record_photos SET sort = ? WHERE record_id = ? AND id = ?", i+1, recordID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListAllRecordPhotos returns every photo association (for export).
func (db *DB) ListAllRecordPhotos() ([]models.RecordPhoto, error) {
	rows, err := db.conn.Query("SELECT id, record_id, file_name, sort FROM record_photos ORDER BY record_id ASC, sort ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RecordPhoto{}
	for rows.Next() {
		var p models.RecordPhoto
		if err := rows.Scan(&p.ID, &p.RecordID, &p.FileName, &p.Sort); err == nil {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// ReplaceRecordPhotos rewrites the associations of one record (import path).
func (db *DB) ReplaceRecordPhotos(recordID string, photos []models.RecordPhoto) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM record_photos WHERE record_id = ?", recordID); err != nil {
		return err
	}
	for i, p := range photos {
		sort := p.Sort
		if sort == 0 {
			sort = i + 1
		}
		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO record_photos (id, record_id, file_name, sort) VALUES (?, ?, ?, ?)",
			p.ID, recordID, p.FileName, sort,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// replaceRecordPhotosTx swaps one record's photo associations inside a
// transaction (import path). RecordID comes from the payload so rows land
// under the just-imported record.
func (db *DB) replaceRecordPhotosTx(tx interface {
	Exec(string, ...any) (sql.Result, error)
}, recordID string, p models.RecordPhoto) error {
	if _, err := tx.Exec("DELETE FROM record_photos WHERE record_id = ?", recordID); err != nil {
		return err
	}
	sort := p.Sort
	if sort == 0 {
		sort = 1
	}
	_, err := tx.Exec(
		"INSERT OR REPLACE INTO record_photos (id, record_id, file_name, sort) VALUES (?, ?, ?, ?)",
		p.ID, recordID, p.FileName, sort,
	)
	return err
}

// ---------- tests ----------

// 测试入口挂在 db 包内以便访问未导出字段；见 photos_test.go。
