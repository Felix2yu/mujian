package db

import "database/sql"

// ---------- 破坏性操作的影响面预览 ----------
//
// HTTP 侧此前只有费用补全支持 dry_run，而 MCP 侧所有变更工具默认只预览。
// 下面这些方法提供同样的「先看影响面、再决定是否执行」能力，供 handlers 在
// 收到 ?dry_run=1（或 body 里 dry_run:true）时调用。

// PreviewDeleteCategory reports how many live records carry this 剧种.
func (db *DB) PreviewDeleteCategory(id string) (int, error) {
	var n int
	err := db.conn.QueryRow(`
		SELECT COUNT(*) FROM records r, json_each(r.category_names) je
		WHERE r.deleted_at = 0
		  AND je.value = (SELECT name FROM categories WHERE id = ?)`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// PreviewDeleteDrama reports how many live records are linked to this 剧目.
func (db *DB) PreviewDeleteDrama(id string) (int, error) {
	return db.countLinkedRecords("record_dramas", "drama_id", id)
}

// PreviewDeleteArtist reports how many live records are linked to this 演员.
func (db *DB) PreviewDeleteArtist(id string) (int, error) {
	return db.countLinkedRecords("record_artists", "artist_id", id)
}

// PreviewDeleteZhezi reports how many live records reference this 折子.
func (db *DB) PreviewDeleteZhezi(id string) (int, error) {
	return db.countLinkedRecords("record_zhezis", "zhezi_id", id)
}

// countLinkedRecords counts non-deleted records joined through a relation table.
// table/col are internal constants, never user input.
func (db *DB) countLinkedRecords(table, col, id string) (int, error) {
	var n int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM "+table+" l JOIN records r ON r.id = l.record_id AND r.deleted_at = 0 WHERE l."+col+" = ?",
		id).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// CountDeletedRecords reports how many records currently sit in the recycle bin.
func (db *DB) CountDeletedRecords() (int, error) {
	var n int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at != 0").Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// RecordExists reports whether a record id exists, including soft-deleted ones
// (purging a record that is not in the bin is refused).
func (db *DB) RecordExists(id string) (bool, error) {
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE id = ?", id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountRecordsByIDs reports how many of the supplied ids are live (non-deleted),
// so batch operations can preview their impact before writing.
func (db *DB) CountRecordsByIDs(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := ""
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	var n int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM records WHERE deleted_at = 0 AND id IN ("+placeholders+")", args...).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}
