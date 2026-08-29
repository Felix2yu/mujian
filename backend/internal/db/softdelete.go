package db

import (
	"fmt"
	"strings"
	"time"

	"mujian/internal/models"
)

// Soft delete: records carry deleted_at (unix seconds, 0 = live). Every read
// path in the db package filters deleted_at = 0; DeleteRecord/BatchDelete
// only set the marker. Hard removal happens exclusively through PurgeRecord /
// PurgeExpiredDeletedRecords (回收站).

const notDeleted = "records.deleted_at = 0"

// SoftDeleteRecord marks one record deleted (回收站). Idempotent: deleting an
// unknown (or already deleted) id succeeds silently, matching the old
// hard-delete API semantics.
func (db *DB) SoftDeleteRecord(id string) error {
	_, err := db.conn.Exec("UPDATE records SET deleted_at = strftime('%s','now') WHERE id = ? AND deleted_at = 0", id)
	return err
}

// SoftDeleteRecords marks many records deleted (批量删除 → 回收站).
func (db *DB) SoftDeleteRecords(ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	res, err := db.conn.Exec(
		"UPDATE records SET deleted_at = strftime('%s','now') WHERE deleted_at = 0 AND id IN ("+strings.Join(ph, ",")+")",
		args...,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RestoreRecord moves a record out of the trash.
func (db *DB) RestoreRecord(id string) error {
	res, err := db.conn.Exec("UPDATE records SET deleted_at = 0 WHERE id = ? AND deleted_at != 0", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("deleted record not found: %s", id)
	}
	return nil
}

// DeletedRecord is a trashed record plus its deletion timestamp.
type DeletedRecord struct {
	models.Record
	DeletedAt int64 `json:"deleted_at"`
}

// ListDeletedRecords returns trashed records (newest deletion first).
func (db *DB) ListDeletedRecords(limit, offset int) ([]DeletedRecord, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.conn.Query(
		"SELECT "+recordColumns+", deleted_at FROM records WHERE deleted_at != 0 ORDER BY deleted_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeletedRecord
	for rows.Next() {
		var dr DeletedRecord
		r, err := scanRecord(rows, &dr.DeletedAt)
		if err != nil {
			continue
		}
		dr.Record = *r
		out = append(out, dr)
	}
	if out == nil {
		out = []DeletedRecord{}
	}
	return out, rows.Err()
}

// PurgeRecord hard-deletes a trashed record and its relation rows.
func (db *DB) PurgeRecord(id string) error {
	for _, tbl := range []string{"record_artists", "record_dramas", "record_zhezis", "record_photos"} {
		if _, err := db.conn.Exec("DELETE FROM "+tbl+" WHERE record_id = ?", id); err != nil {
			return err
		}
	}
	_, err := db.conn.Exec("DELETE FROM records WHERE id = ? AND deleted_at != 0", id)
	return err
}

// PurgeExpiredDeletedRecords hard-deletes trashed records older than maxAge.
// Returns how many records were removed.
func (db *DB) PurgeExpiredDeletedRecords(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Unix()
	rows, err := db.conn.Query("SELECT id FROM records WHERE deleted_at != 0 AND deleted_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var purged int64
	for _, id := range ids {
		if err := db.PurgeRecord(id); err != nil {
			return purged, err
		}
		purged++
	}
	return purged, nil
}

// DeletedCount returns the number of records currently in the trash.
func (db *DB) DeletedCount() (int, error) {
	var n int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at != 0").Scan(&n)
	return n, err
}
