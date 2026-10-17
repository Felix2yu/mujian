package db

import (
	"database/sql"
	"fmt"
)

// RFC 6578 (DAV Sync) change tracking for the read-only CalDAV projection.
//
// Sync model: the server is the sole writer; Apple Calendar (and any other
// CalDAV client) is read-only. Therefore every record change — including
// deletions — is recorded here as an append-only, monotonically increasing
// event rather than being inferred from "the collection no longer contains
// it". A client sync token is just the seq value it last saw; the
// sync-collection REPORT returns every record that changed after that seq
// (as an add/update or as an explicit 404 removal) plus the new token.
//
// Triggers make the journal cover every write path (API, MCP, import, merge,
// restore, trash) without business code knowing about it. The journal only
// stores record ids: the authoritative object state (and whether the record
// is still a member of either collection) is resolved at sync time, so
// status flips that move an object between the VEVENT and VTODO collections
// are reported correctly and UPDATE noise on ICS-irrelevant columns is
// harmless (the rendered ETag is unchanged and the client no-ops).
//
// Retention is a sliding window of the newest caldavChangelogKeep entries.
// A pruned token fails validation (DAV:valid-sync-token, HTTP 403) and the
// client transparently falls back to a full sync.
const caldavChangelogKeep = 20000

// createCaldavChangelog creates the sync journal table and the triggers that
// populate it. Idempotent; safe to run on every startup. Must run after the
// records / relation tables exist.
func (db *DB) createCaldavChangelog() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS caldav_changelog (
			seq       INTEGER PRIMARY KEY AUTOINCREMENT,
			record_id TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_caldav_changelog_record ON caldav_changelog(record_id)`,

		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_records_insert
			AFTER INSERT ON records
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (new.id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_records_update
			AFTER UPDATE ON records
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (new.id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_records_delete
			AFTER DELETE ON records
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (old.id);
		END`,

		// Relation edits (artist/zhezi/drama re-linking, merges) change the
		// rendered ICS even though records itself is not updated. SQLite
		// triggers take exactly one event, hence one trigger per operation.
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_artists_rel_insert
			AFTER INSERT ON record_artists
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (new.record_id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_artists_rel_delete
			AFTER DELETE ON record_artists
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (old.record_id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_zhezis_rel_insert
			AFTER INSERT ON record_zhezis
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (new.record_id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_zhezis_rel_delete
			AFTER DELETE ON record_zhezis
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (old.record_id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_dramas_rel_insert
			AFTER INSERT ON record_dramas
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (new.record_id);
		END`,
		`CREATE TRIGGER IF NOT EXISTS caldav_changelog_dramas_rel_delete
			AFTER DELETE ON record_dramas
		BEGIN
			INSERT INTO caldav_changelog(record_id) VALUES (old.record_id);
		END`,
	}
	for _, q := range stmts {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("create caldav changelog object: %w", err)
		}
	}
	return nil
}

// CaldavCurrentSyncSeq returns the newest journal seq (0 when no change has
// ever been recorded). The value doubles as the opaque DAV sync-token.
func (db *DB) CaldavCurrentSyncSeq() (int64, error) {
	var seq int64
	if err := db.conn.QueryRow(
		"SELECT COALESCE(MAX(seq), 0) FROM caldav_changelog",
	).Scan(&seq); err != nil {
		return 0, fmt.Errorf("read caldav sync seq: %w", err)
	}
	return seq, nil
}

// CaldavChangedRecordIDs returns the distinct record ids with journal events
// after afterSeq, ordered by the first change in the window so a record that
// changed several times within one sync collapses to its latest state in a
// deterministic order.
func (db *DB) CaldavChangedRecordIDs(afterSeq int64) ([]string, error) {
	rows, err := db.conn.Query(
		`SELECT record_id FROM caldav_changelog
		 WHERE seq > ?
		 GROUP BY record_id
		 ORDER BY MIN(seq)`,
		afterSeq,
	)
	if err != nil {
		return nil, fmt.Errorf("query caldav changes: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan caldav change: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CaldavSyncTokenValid reports whether seq is a token the server can still
// diff from. seq 0 (initial sync, no token) is always valid. A seq below the
// retained window (pruned), from a foreign database (restore without
// journal) or in the future is invalid and the client must resync fully.
func (db *DB) CaldavSyncTokenValid(seq int64) (bool, error) {
	if seq < 0 {
		return false, nil
	}
	if seq == 0 {
		return true, nil
	}
	var minSeq, maxSeq sql.NullInt64
	if err := db.conn.QueryRow(
		"SELECT MIN(seq), COALESCE(MAX(seq), 0) FROM caldav_changelog",
	).Scan(&minSeq, &maxSeq); err != nil {
		return false, fmt.Errorf("validate caldav sync token: %w", err)
	}
	if !minSeq.Valid {
		return false, nil // journal empty: no positive token can be valid
	}
	return seq >= minSeq.Int64 && seq <= maxSeq.Int64, nil
}

// PruneCaldavChangelog drops journal entries older than the newest `keep`
// entries and returns how many were removed.
func (db *DB) PruneCaldavChangelog(keep int64) (int64, error) {
	if keep <= 0 {
		keep = caldavChangelogKeep
	}
	res, err := db.conn.Exec(
		`DELETE FROM caldav_changelog WHERE seq <= (
			SELECT MAX(seq) - ? FROM caldav_changelog
		)`,
		keep,
	)
	if err != nil {
		return 0, fmt.Errorf("prune caldav changelog: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// PruneCaldavChangelogDefault applies the default retention window.
func (db *DB) PruneCaldavChangelogDefault() (int64, error) {
	return db.PruneCaldavChangelog(caldavChangelogKeep)
}
