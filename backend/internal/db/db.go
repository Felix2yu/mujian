package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"mujian/internal/models"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	loc  *time.Location
	// Prepared statements, prepared once at open time and reused for every
	// execution (database/sql binds them to the right connection / tx).
	stmtUpsertRecord *sql.Stmt
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// SQLite pragmas (WAL + safe performance tuning):
	//  - journal_mode(WAL): concurrent readers while a writer is active
	//  - busy_timeout(30000): wait instead of erroring on lock contention. Long
	//    enough for a second import to queue behind a running one.
	//  - _txlock=immediate: every transaction acquires the write lock at BEGIN,
	//    not lazily at first write. This matters because a DEFERRED tx that
	//    upgrades mid-transaction returns SQLITE_BUSY immediately (busy_timeout
	//    is not consulted when a read snapshot would be invalidated), which is
	//    how imports failed with "database is locked" under concurrent writers.
	//    With IMMEDIATE, contenders simply queue on BEGIN and then run lock-free.
	//  - foreign_keys(0): the schema uses JSON-in-TEXT links, not FK constraints
	//  - synchronous(NORMAL): WAL already protects against corruption; avoids a
	//    fsync per transaction while keeping crash safety (recommended for WAL)
	//  - mmap_size / cache_size: keep more of the db in memory for read speed
	conn, err := sql.Open("sqlite", dbPath+"?"+
		"_pragma=journal_mode(WAL)"+
		"&_pragma=busy_timeout(30000)"+
		"&_txlock=immediate"+
		"&_pragma=foreign_keys(0)"+
		"&_pragma=synchronous(NORMAL)"+
		"&_pragma=cache_size(-8000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite is a single-writer database. With _txlock=immediate + busy_timeout
	// (both set in the DSN above), concurrent writers serialize on BEGIN instead
	// of failing mid-transaction. We deliberately do NOT pin MaxOpenConns(1):
	// with modernc.org/sqlite that can deadlock when successive calls briefly
	// contend on the single pooled connection. The pool default is safe for this
	// low-concurrency app.

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	db := &DB{conn: conn, loc: time.UTC}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Prepare the hot upsert statement once; reused for every record write.
	// Done after migrate() so the records table already exists.
	stmt, err := conn.Prepare(recordUpsertSQL)
	if err != nil {
		return nil, fmt.Errorf("prepare record upsert: %w", err)
	}
	db.stmtUpsertRecord = stmt
	// 剧名与剧目等价：从既有记录的剧名(play)补齐剧目档案并关联记录。
	// 幂等，可安全地在每次启动时运行。
	if err := db.BackfillDramasFromRecords(); err != nil {
		return nil, fmt.Errorf("backfill dramas: %w", err)
	}

	return db, nil
}

func (db *DB) SetLocation(loc *time.Location) {
	db.loc = loc
}

// SQLStats exposes the underlying database/sql pool statistics for the
// /metrics endpoint (in-use, idle, wait counts...). Read-only.
func (db *DB) SQLStats() sql.DBStats {
	return db.conn.Stats()
}

func (db *DB) Ping() error {
	return db.conn.Ping()
}

func (db *DB) Close() {
	db.conn.Close()
}

// migrate drops the legacy schema (from the previous multi-user version) and
// creates the export-aligned schema. The previous app is replaced wholesale,
// so any old tables are removed to guarantee a clean, exact match.
func (db *DB) migrate() error {
	// Drop legacy tables unconditionally — the new schema replaces them.
	for _, t := range []string{"users", "shows", "scene_sorts", "actors"} {
		if _, err := db.conn.Exec("DROP TABLE IF EXISTS " + t); err != nil {
			return fmt.Errorf("drop legacy %s: %w", t, err)
		}
	}
	// The old `categories` table had (user_id, color, sort_order); the new one
	// does not. Drop it only if it is the legacy shape.
	var legacyCatCols int
	db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('categories') WHERE name IN ('user_id','color')").Scan(&legacyCatCols)
	if legacyCatCols > 0 {
		if _, err := db.conn.Exec("DROP TABLE IF EXISTS categories"); err != nil {
			return fmt.Errorf("drop legacy categories: %w", err)
		}
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS records (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			city TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			coordinate TEXT NOT NULL DEFAULT '',
			cover TEXT NOT NULL DEFAULT '',
			cover_file TEXT NOT NULL DEFAULT '',
			cover_thumb TEXT NOT NULL DEFAULT '',
			custom_category_id TEXT NOT NULL DEFAULT '',
			category_name TEXT NOT NULL DEFAULT '',
			category_names TEXT NOT NULL DEFAULT '[]',
			artist_names TEXT NOT NULL DEFAULT '[]',
			guest TEXT NOT NULL DEFAULT '[]',
			play TEXT NOT NULL DEFAULT '[]',
			tag_ids TEXT NOT NULL DEFAULT '[]',
			date INTEGER NOT NULL DEFAULT 0,
			date_text TEXT NOT NULL DEFAULT '',
			rating INTEGER NOT NULL DEFAULT 0,
			seat TEXT NOT NULL DEFAULT '',
			friends TEXT NOT NULL DEFAULT '',
			company TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '',
			active_status INTEGER NOT NULL DEFAULT 0,
			price REAL NOT NULL DEFAULT 0,
			price_currency TEXT NOT NULL DEFAULT 'CNY',
			pay_price REAL NOT NULL DEFAULT 0,
			pay_price_currency TEXT NOT NULL DEFAULT 'CNY',
			other_cost REAL NOT NULL DEFAULT 0,
			other_cost_currency TEXT NOT NULL DEFAULT 'CNY',
			total_cost REAL NOT NULL DEFAULT 0,
			duration INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_records_date ON records(date)`,
		`CREATE INDEX IF NOT EXISTS idx_records_category ON records(category_name)`,
		`CREATE INDEX IF NOT EXISTS idx_records_city ON records(city)`,
		`CREATE INDEX IF NOT EXISTS idx_records_cover_file ON records(cover_file)`,
		`CREATE INDEX IF NOT EXISTS idx_records_active_status ON records(active_status)`,
		`CREATE INDEX IF NOT EXISTS idx_records_name ON records(name)`,
		// Group-by / order-by columns used by stats and venue listing. On the
		// current data volume a scan is cheaper, but the record count grows
		// linearly and these turn quadratic group-bys into index scans.
		`CREATE INDEX IF NOT EXISTS idx_records_company ON records(company)`,
		`CREATE INDEX IF NOT EXISTS idx_records_channel ON records(channel)`,
		`CREATE INDEX IF NOT EXISTS idx_records_address ON records(address)`,
		`CREATE INDEX IF NOT EXISTS idx_records_rating ON records(rating)`,
		`CREATE TABLE IF NOT EXISTS covers (
			hash TEXT NOT NULL,
			file_name TEXT PRIMARY KEY,
			ext TEXT NOT NULL DEFAULT '',
			size INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_covers_hash ON covers(hash)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			active_ids TEXT NOT NULL DEFAULT '[]',
			record_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS dramas (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			aliases TEXT NOT NULL DEFAULT '[]',
			category_name TEXT NOT NULL DEFAULT '',
			category_names TEXT NOT NULL DEFAULT '[]',
			remark TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_dramas_name ON dramas(name)`,
		`CREATE INDEX IF NOT EXISTS idx_dramas_category ON dramas(category_name)`,
		`CREATE TABLE IF NOT EXISTS zhezis (
			id TEXT PRIMARY KEY,
			drama_id TEXT NOT NULL,
			name TEXT NOT NULL,
			aliases TEXT NOT NULL DEFAULT '[]',
			sort_order INTEGER NOT NULL DEFAULT 0,
			remark TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_zhezis_drama ON zhezis(drama_id)`,
		// Relation table: record <-> drama (replaces the JSON-in-TEXT drama_ids
		// column for all cross-table lookups so we can use real indexes instead
		// of instr() scans). records.drama_ids is kept only as a legacy fallback
		// for reading old backups; the relation table is the source of truth.
		`CREATE TABLE IF NOT EXISTS record_dramas (
			record_id TEXT NOT NULL,
			drama_id TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (record_id, drama_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_record_dramas_drama ON record_dramas(drama_id)`,
		`CREATE INDEX IF NOT EXISTS idx_record_dramas_record ON record_dramas(record_id)`,
		// 票根/现场照等附加图片：file_name 指向内容寻址的 covers/ 存储 key，
		// 图片本体与封面共用去重存储；记录彻底删除时级联清理关联行。
		`CREATE TABLE IF NOT EXISTS record_photos (
			id TEXT PRIMARY KEY,
			record_id TEXT NOT NULL,
			file_name TEXT NOT NULL,
			sort INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_record_photos_record ON record_photos(record_id)`,
		// Relation table: record <-> zhezi. Same rationale as record_dramas:
		// replaces the JSON-in-TEXT zhezi_ids column so cross-table lookups use
		// real indexes instead of instr() scans. records.zhezi_ids is kept only as
		// a legacy fallback for reading old backups; the relation table is truth.
		`CREATE TABLE IF NOT EXISTS record_zhezis (
		record_id TEXT NOT NULL,
		zhezi_id TEXT NOT NULL,
		sort_order INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (record_id, zhezi_id)
	)`,
		`CREATE INDEX IF NOT EXISTS idx_record_zhezis_zhezi ON record_zhezis(zhezi_id)`,
		`CREATE INDEX IF NOT EXISTS idx_record_zhezis_record ON record_zhezis(record_id)`,
		// Actor (演员) is a first-class entity, mirroring dramas. It owns a cover,
		// bio and aliases so it can power a dedicated actor home page.
		`CREATE TABLE IF NOT EXISTS artists (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		aliases TEXT NOT NULL DEFAULT '[]',
		remark TEXT NOT NULL DEFAULT '',
		cover TEXT NOT NULL DEFAULT '',
		cover_file TEXT NOT NULL DEFAULT '',
		cover_thumb TEXT NOT NULL DEFAULT '',
		bio TEXT NOT NULL DEFAULT '',
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
		`CREATE INDEX IF NOT EXISTS idx_artists_name ON artists(name)`,
		// Relation table: record <-> artist. Replaces the JSON-in-TEXT artist_names
		// column for cross-table lookups (actor home page reverse query).
		// records.artist_names is kept only as a legacy fallback for old backups.
		`CREATE TABLE IF NOT EXISTS record_artists (
		record_id TEXT NOT NULL,
		artist_id TEXT NOT NULL,
		sort_order INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (record_id, artist_id)
	)`,
		`CREATE INDEX IF NOT EXISTS idx_record_artists_artist ON record_artists(artist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_record_artists_record ON record_artists(record_id)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// Add the drama/zhezi link columns to existing records tables that were
	// created before this schema addition.
	if err := db.addColumn("records", "drama_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := db.addColumn("records", "zhezi_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	// Multi-category support: category_names is a JSON array; category_name
	// stays as the primary (first) category for compatibility.
	if err := db.addColumn("records", "category_names", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := db.addColumn("dramas", "category_names", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	// Backfill: wrap existing scalar categories into single-element arrays.
	// Idempotent — only touches rows still on the empty-array default.
	if _, err := db.conn.Exec(`UPDATE records SET category_names = json_array(category_name)
		WHERE category_name != '' AND category_names = '[]'`); err != nil {
		return fmt.Errorf("backfill records.category_names: %w", err)
	}
	if _, err := db.conn.Exec(`UPDATE dramas SET category_names = json_array(category_name)
		WHERE category_name != '' AND category_names = '[]'`); err != nil {
		return fmt.Errorf("backfill dramas.category_names: %w", err)
	}
	// Manual ordering for dramas/categories (0 = alphabetical).
	if err := db.addColumn("dramas", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Drama aliases: different 剧种 may have different names for the same play.
	if err := db.addColumn("dramas", "aliases", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	// Total cost: pre-computed sum of effective price + other_cost for quick display.
	if err := db.addColumn("records", "total_cost", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Soft delete: unix timestamp of deletion, 0 = live. All read paths must
	// filter deleted_at = 0 (see softdelete.go).
	if err := db.addColumn("records", "deleted_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// Backfill total_cost for existing records.
	if _, err := db.conn.Exec(`UPDATE records SET total_cost = CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)`); err != nil {
		return fmt.Errorf("backfill records.total_cost: %w", err)
	}
	if err := db.addColumn("categories", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	// 演出时长（分钟）：新增字段，旧库加列并默认 0（未知）。
	if err := db.addColumn("records", "duration", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	// One-time migration: expand legacy records.drama_ids JSON into the
	// record_dramas relation table. Idempotent — existing relation rows are
	// preserved and only missing links are inserted.
	if err := db.migrateDramaRelations(); err != nil {
		return err
	}
	// Same expansion for legacy records.zhezi_ids JSON into record_zhezis.
	if err := db.migrateZheziRelations(); err != nil {
		return err
	}

	// Expand legacy records.artist_names (names, not ids) into the artists
	// entity table + record_artists relation table.
	if err := db.migrateArtistRelations(); err != nil {
		return err
	}

	// active_ids on categories is now derived from records; drop the redundant
	// column. Existing data is reconstructable from records.active_status.
	if err := db.dropColumnIfExists("categories", "active_ids"); err != nil {
		return err
	}

	return nil
}

// migrateDramaRelations backfills record_dramas from the legacy drama_ids TEXT
// column. It only inserts links that are not already present, so re-running is
// safe. After this, record_dramas is the source of truth for record<->drama
// edges and records.drama_ids is only read when importing old backups.
func (db *DB) migrateDramaRelations() error {
	rows, err := db.conn.Query("SELECT id, drama_ids FROM records WHERE drama_ids IS NOT NULL AND drama_ids != '' AND drama_ids != '[]'")
	if err != nil {
		return fmt.Errorf("migrate drama relations: %w", err)
	}
	type rec struct {
		id   string
		json string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.json); err != nil {
			rows.Close()
			return err
		}
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range recs {
		ids := unmarshalStrings(r.json)
		if len(ids) == 0 {
			continue
		}
		// Skip ids already linked (handles re-runs / partial prior runs).
		placeholders := make([]string, len(ids))
		args := make([]interface{}, 0, len(ids)+1)
		args = append(args, r.id)
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		var existing int
		if err := db.conn.QueryRow(
			"SELECT COUNT(*) FROM record_dramas WHERE record_id = ? AND drama_id IN ("+strings.Join(placeholders, ",")+")",
			args...,
		).Scan(&existing); err != nil {
			return err
		}
		if existing == len(ids) {
			continue
		}
		for i, id := range ids {
			if _, err := db.conn.Exec(
				"INSERT OR IGNORE INTO record_dramas (record_id, drama_id, sort_order) VALUES (?, ?, ?)",
				r.id, id, i,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *DB) migrateZheziRelations() error {
	rows, err := db.conn.Query("SELECT id, zhezi_ids FROM records WHERE zhezi_ids IS NOT NULL AND zhezi_ids != '' AND zhezi_ids != '[]'")
	if err != nil {
		return fmt.Errorf("migrate zhezi relations: %w", err)
	}
	type rec struct {
		id   string
		json string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.json); err != nil {
			rows.Close()
			return err
		}
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range recs {
		ids := unmarshalStrings(r.json)
		if len(ids) == 0 {
			continue
		}
		placeholders := make([]string, len(ids))
		args := make([]interface{}, 0, len(ids)+1)
		args = append(args, r.id)
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		var existing int
		if err := db.conn.QueryRow(
			"SELECT COUNT(*) FROM record_zhezis WHERE record_id = ? AND zhezi_id IN ("+strings.Join(placeholders, ",")+")",
			args...,
		).Scan(&existing); err != nil {
			return err
		}
		if existing == len(ids) {
			continue
		}
		for i, id := range ids {
			if _, err := db.conn.Exec(
				"INSERT OR IGNORE INTO record_zhezis (record_id, zhezi_id, sort_order) VALUES (?, ?, ?)",
				r.id, id, i,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveArtistByName finds an existing artist by exact name (or alias), or
// creates a new one. Used by the legacy artist_names migration and by the
// upsert path so free-typed actor names become first-class entities.
//
// exec must be the caller's execution context (*sql.DB for standalone use,
// *sql.Tx inside a transaction). This is critical for ImportData: creating a
// missing artist writes to the table, and doing so on a pool connection while
// the import transaction holds the write lock self-deadlocks until
// busy_timeout expires (SQLITE_BUSY).
func (db *DB) resolveArtistByName(exec sqlExecutor, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty artist name")
	}
	// Exact name match first.
	var id string
	if err := exec.QueryRow("SELECT id FROM artists WHERE name = ?", name).Scan(&id); err == nil {
		return id, nil
	}
	// Alias match.
	rows, err := db.conn.Query("SELECT id, aliases FROM artists")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var aid, aliases string
		if err := rows.Scan(&aid, &aliases); err != nil {
			continue
		}
		for _, a := range unmarshalStrings(aliases) {
			if a == name {
				return aid, nil
			}
		}
	}
	rows.Close()
	// Not found: create (inside the caller's transaction, if any).
	id = newID()
	if _, err := exec.Exec(
		"INSERT INTO artists (id, name, aliases, sort_order) VALUES (?, ?, '[]', (SELECT COALESCE(MAX(sort_order),0)+1 FROM artists))",
		id, name,
	); err != nil {
		return "", err
	}
	return id, nil
}

// migrateArtistRelations expands the legacy records.artist_names TEXT column
// (which stores actor *names*) into the artists entity table + record_artists
// relation. Idempotent: existing artists are matched by name/alias and already
// linked edges are skipped.
func (db *DB) migrateArtistRelations() error {
	rows, err := db.conn.Query("SELECT id, artist_names FROM records WHERE artist_names IS NOT NULL AND artist_names != '' AND artist_names != '[]'")
	if err != nil {
		return fmt.Errorf("migrate artist relations: %w", err)
	}
	type rec struct {
		id   string
		json string
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.json); err != nil {
			rows.Close()
			return err
		}
		recs = append(recs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range recs {
		names := unmarshalStrings(r.json)
		if len(names) == 0 {
			continue
		}
		for i, name := range names {
			aid, err := db.resolveArtistByName(db.conn, name)
			if err != nil {
				return err
			}
			if _, err := db.conn.Exec(
				"INSERT OR IGNORE INTO record_artists (record_id, artist_id, sort_order) VALUES (?, ?, ?)",
				r.id, aid, i,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// addColumn adds a column to a table if it does not already exist. SQLite 3
// versions bundled with modernc.org/sqlite support ADD COLUMN; we guard on
// pragma_table_info to make migration idempotent.
func (db *DB) addColumn(table, col, ddl string) error {
	var cnt int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, col).Scan(&cnt); err != nil {
		return fmt.Errorf("pragma %s.%s: %w", table, col, err)
	}
	if cnt > 0 {
		return nil
	}
	if _, err := db.conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, col+" "+ddl)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, col, err)
	}
	return nil
}

// dropColumnIfExists removes a column if present. SQLite 3.35+ (modernc) supports
// DROP COLUMN; we guard on pragma_table_info so re-running is a no-op.
func (db *DB) dropColumnIfExists(table, col string) error {
	var cnt int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, col).Scan(&cnt); err != nil {
		return fmt.Errorf("pragma %s.%s: %w", table, col, err)
	}
	if cnt == 0 {
		return nil
	}
	if _, err := db.conn.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, col)); err != nil {
		return fmt.Errorf("drop column %s.%s: %w", table, col, err)
	}
	return nil
}

// ---------- JSON helpers for array / nested columns ----------

func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalStrings(s string) []string {
	if s == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func unmarshalCoordinate(s string) *models.Coordinate {
	if s == "" || s == "null" {
		return nil
	}
	var c models.Coordinate
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil
	}
	return &c
}

// ---------- Record queries ----------

// recordColumns is the set of columns selected when reading records. drama_ids
// and artist_names are intentionally NOT included: those links live in relation
// tables (record_dramas / record_artists) and are backfilled into
// models.Record.DramaIDs / ArtistIDs after the query. (recordUpsertSQL still
// writes the legacy drama_ids / artist_names columns for backward-compatible
// backups; the relation tables are the source of truth.)
// Every column is qualified with the `records` table alias so that queries
// which JOIN record_artists / artists / record_dramas / record_zhezis do not
// trip SQLite's "ambiguous column name" error on `id`.
const recordColumns = `records.id, records.name, records.channel, records.city, records.address,
	records.coordinate, records.cover, records.cover_file, records.cover_thumb,
	records.custom_category_id, records.category_name, records.category_names, records.guest, records.play,
	records.zhezi_ids, records.tag_ids, records.date, records.date_text, records.rating, records.duration,
	records.seat, records.friends, records.company, records.remark, records.active_status,
	records.price, records.price_currency, records.pay_price, records.pay_price_currency,
	records.other_cost, records.other_cost_currency, records.total_cost`

// scanRecord scans recordColumns in order; extra destinations (appended after
// the fixed columns, e.g. deleted_at) are supported via extra.
func scanRecord(rows *sql.Rows, extra ...any) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, guest, play, zheziIDs, tagIDs, categoryNames string
	)
	dests := []any{
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &categoryNames, &guest, &play, &zheziIDs, &tagIDs,
		&r.Date, &r.DateText, &r.Rating, &r.Duration, &r.Seat, &r.Friends, &r.Company, &r.Remark, &r.ActiveStatus,
		&r.Price, &r.PriceCurrency, &r.PayPrice, &r.PayPriceCurrency, &r.OtherCost, &r.OtherCostCurrency, &r.TotalCost,
	}
	err := rows.Scan(append(dests, extra...)...)
	if err != nil {
		return nil, err
	}
	r.Coordinate = unmarshalCoordinate(coordinate)
	r.Guest = unmarshalStrings(guest)
	r.Play = unmarshalStrings(play)
	r.ZheziIDs = unmarshalStrings(zheziIDs)
	r.TagIDs = unmarshalStrings(tagIDs)
	applyCategoryFallback(&r, categoryNames)
	return &r, nil
}

// applyCategoryFallback reconciles the scalar primary category with the
// category_names JSON array read from disk. Rows written before the
// multi-category migration only carry the scalar column; rows that somehow
// carry an array but no scalar get the scalar backfilled from element 0.
func applyCategoryFallback(r *models.Record, raw string) {
	r.CategoryNames = unmarshalStrings(raw)
	if len(r.CategoryNames) == 0 {
		if r.CategoryName != "" {
			r.CategoryNames = []string{r.CategoryName}
		}
		return
	}
	if r.CategoryName == "" {
		r.CategoryName = r.CategoryNames[0]
	}
}

type RecordFilter struct {
	Year     int
	Month    int
	Start    string // date string YYYY-MM-DD or unix
	End      string
	Query    string
	Category string
	City     string
	DramaID  string // a record whose drama_ids contains this id
	ZheziID  string // a record whose zhezi_ids contains this id
	ArtistID string // a record whose artist_ids contains this id
	// Missing is a comma-separated list of field tokens; a record matches if
	// ANY listed field is empty. Drives data-hygiene queries such as "find
	// records without a category". Supported tokens: category, city, address,
	// company, channel, rating, price, cover, coordinate, artist, drama, zhezi,
	// friends, remark, seat, play.
	Missing string
	// 扩展筛选维度（前端筛选面板新增的字段）
	Channel      string  // 渠道精确匹配
	Company      string  // 剧团精确匹配
	RatingMin    int     // 评分下限（含）
	PriceMin     float64 // 票价下限（含）
	PriceMax     float64 // 票价上限（含）；仅当 >0 时生效
	ActiveStatus int     // 演出状态：0=正常 1=想看 2=已取消 3=未赴约；仅当 >0 时生效
	// Statuses is a multi-select of active_status values (the client's status
	// preferences). When non-empty it takes precedence over ActiveStatus and
	// is applied to BOTH listing and counting, so the UI's "已加载 X / 共 Y"
	// counter agrees with what the user actually sees.
	Statuses []int
	Exact    bool // 关键词精确匹配（按 name 全等，而非模糊 LIKE）
	Limit    int
	Offset   int
	// NoLimit disables the default/hard row caps applied by ListRecords.
	// Only for endpoints whose contract is "everything": /api/export,
	// /api/calendar.ics and the explicit /api/records/all.
	NoLimit bool
}

// Default row caps for paginated list endpoints. They exist so a growing
// table degrades into "many pages" instead of "one multi-megabyte response".
const (
	defaultListLimit = 500
	hardListLimit    = 2000
)

// slowQueryMs above which a hot-path query is logged at WARN level. Chosen so
// normal single-user traffic (sub-ms to a few ms) never logs, while a missing
// index or table growth shows up in the server log / metrics immediately.
const slowQueryMs = 100

// searchLikeWhere builds the keyword-search predicate. Actor and drama-alias
// search use EXISTS subqueries instead of LEFT JOINs: joining record_artists
// (≈9.6 artists per record) against record_dramas produced a ~24× row
// cartesian blowup that DISTINCT then had to collapse over 30 columns.
// The actor/drama predicates are appended to `args` after the 10 record-column
// placeholders, in the order: artists LIKE, dramas aliases LIKE.
const searchLikeCols = `(records.name LIKE ? OR records.city LIKE ? OR records.address LIKE ? OR records.company LIKE ? OR records.channel LIKE ? OR records.remark LIKE ? OR records.friends LIKE ? OR records.category_name LIKE ? OR records.category_names LIKE ? OR records.play LIKE ?
	OR EXISTS (SELECT 1 FROM record_artists ra_q JOIN artists a_q ON a_q.id = ra_q.artist_id WHERE ra_q.record_id = records.id AND a_q.name LIKE ?)
	OR EXISTS (SELECT 1 FROM record_dramas rd_q JOIN dramas d_q ON d_q.id = rd_q.drama_id WHERE rd_q.record_id = records.id AND d_q.aliases LIKE ?))`

// ListRecords runs the query with a background context; request-scoped
// callers should prefer ListRecordsContext so client cancellation propagates.
func (db *DB) ListRecords(f RecordFilter) ([]models.Record, error) {
	return db.ListRecordsContext(context.Background(), f)
}

// appendStatusPredicate adds the active_status predicate for the filter.
// Statuses (multi-select from the client's status preferences) takes
// precedence over the single-value ActiveStatus param. Shared by
// ListRecordsContext and CountRecordsContext so the list and its total always
// agree on which statuses are visible.
func appendStatusPredicate(f RecordFilter, where *[]string, args *[]interface{}) {
	if len(f.Statuses) > 0 {
		ph := make([]string, len(f.Statuses))
		for i := range f.Statuses {
			ph[i] = "?"
			*args = append(*args, f.Statuses[i])
		}
		*where = append(*where, "active_status IN ("+strings.Join(ph, ",")+")")
		return
	}
	if f.ActiveStatus > 0 {
		*where = append(*where, "active_status = ?")
		*args = append(*args, f.ActiveStatus)
	}
}

// ListRecordsContext is ListRecords bound to ctx: the SQLite driver honors
// cancellation, so an abandoned HTTP request stops burning query time.
// VacuumInto writes a consistent snapshot of the whole database to path via
// SQLite's VACUUM INTO (single statement, safe under WAL with concurrent
// readers; fails if the target file already exists).
func (db *DB) VacuumInto(path string) error {
	_, err := db.conn.Exec("VACUUM INTO ?", path)
	return err
}

// WalCheckpoint truncates the write-ahead log into the main database file.
// Called after a successful snapshot backup: the on-disk .db is then fully
// self-contained even if the process later crashes before a clean shutdown.
func (db *DB) Checkpoint() error {
	_, err := db.conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// appendSearchPredicate adds the keyword-search predicate. Space-separated
// tokens are AND-ed: "牡丹亭 上海" matches records that contain 牡丹亭 in some
// searchable column AND 上海 in some (possibly different) column. A single
// token behaves exactly like the old whole-string LIKE; Exact still compares
// records.name to the raw query string verbatim. Shared by ListRecordsContext
// and CountRecordsContext so the list and its total always agree.
func appendSearchPredicate(f RecordFilter, where *[]string, args *[]interface{}) {
	if f.Query == "" {
		return
	}
	if f.Exact {
		*where = append(*where, "records.name = ?")
		*args = append(*args, f.Query)
		return
	}
	for _, tok := range strings.Fields(f.Query) {
		*where = append(*where, searchLikeCols)
		for range 12 {
			*args = append(*args, "%"+tok+"%")
		}
	}
}

func (db *DB) ListRecordsContext(ctx context.Context, f RecordFilter) ([]models.Record, error) {
	started := time.Now()
	defer func() {
		if ms := time.Since(started).Milliseconds(); ms >= slowQueryMs {
			slog.Warn("slow query", "op", "ListRecords", "ms", ms)
		}
	}()

	// DISTINCT is only needed when a filter can multiply rows: the
	// json_each category expansion, or the drama/zhezi/artist relation JOINs
	// (a record matching two such JOINs at once cross-products). The search
	// predicate no longer joins anything (EXISTS subqueries), so the common
	// keyword-search path skips the DISTINCT temp B-tree entirely.
	needsDistinct := f.Category != "" || f.DramaID != "" || f.ZheziID != "" || f.ArtistID != ""
	query := `SELECT `
	if needsDistinct {
		query += `DISTINCT `
	}
	query += recordColumns + ` FROM records`
	where := []string{notDeleted}
	args := []interface{}{}

	appendSearchPredicate(f, &where, &args)
	if f.Category != "" {
		// Multi-category: match any element of the category_names JSON array
		// (single-category rows are stored as one-element arrays).
		query += " JOIN json_each(records.category_names) je_cat ON je_cat.value = ?"
		args = append(args, f.Category)
	}
	if f.City != "" {
		where = append(where, "city = ?")
		args = append(args, f.City)
	}
	if f.DramaID != "" {
		// Use the relation table (indexed) instead of instr() over the JSON
		// text column.
		query += " JOIN record_dramas rd ON rd.record_id = records.id"
		where = append(where, "rd.drama_id = ?")
		args = append(args, f.DramaID)
	}
	if f.ZheziID != "" {
		// Use the relation table (indexed) instead of instr() over the JSON
		// text column.
		query += " JOIN record_zhezis rz ON rz.record_id = records.id"
		where = append(where, "rz.zhezi_id = ?")
		args = append(args, f.ZheziID)
	}
	if f.ArtistID != "" {
		// Use the relation table (indexed) instead of instr() over the JSON
		// text column.
		query += " JOIN record_artists ra ON ra.record_id = records.id"
		where = append(where, "ra.artist_id = ?")
		args = append(args, f.ArtistID)
	}
	if f.Year > 0 && f.Month > 0 {
		// filter by calendar month of the unix `date`
		start := time.Date(f.Year, time.Month(f.Month), 1, 0, 0, 0, 0, db.loc)
		end := start.AddDate(0, 1, 0)
		where = append(where, "date >= ? AND date < ?")
		args = append(args, start.Unix(), end.Unix())
	} else if f.Start != "" || f.End != "" {
		if t, ok := parseTimeArg(f.Start, db.loc); ok {
			where = append(where, "date >= ?")
			args = append(args, t.Unix())
		}
		if t, ok := parseTimeArg(f.End, db.loc); ok {
			where = append(where, "date < ?")
			args = append(args, t.AddDate(0, 0, 1).Unix())
		}
	}

	if f.Missing != "" {
		if p := buildMissingPredicate(f.Missing); p != "" {
			where = append(where, p)
		}
	}

	// 扩展维度：渠道/剧团精确匹配、评分下限、票价区间、状态过滤
	if f.Channel != "" {
		where = append(where, "channel = ?")
		args = append(args, f.Channel)
	}
	if f.Company != "" {
		where = append(where, "company = ?")
		args = append(args, f.Company)
	}
	if f.RatingMin > 0 {
		where = append(where, "rating >= ?")
		args = append(args, f.RatingMin)
	}
	if f.PriceMin > 0 {
		where = append(where, "price >= ?")
		args = append(args, f.PriceMin)
	}
	if f.PriceMax > 0 {
		where = append(where, "price <= ?")
		args = append(args, f.PriceMax)
	}
	appendStatusPredicate(f, &where, &args)

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY date DESC"
	if !f.NoLimit {
		// Row caps: unlimited queries would return one multi-megabyte
		// response as the table grows. Explicit "all" endpoints opt out
		// via NoLimit; everything else gets a sane default and a hard cap.
		if f.Limit <= 0 {
			f.Limit = defaultListLimit
		}
		if f.Limit > hardListLimit {
			f.Limit = hardListLimit
		}
	}
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			slog.Warn("scan record", "err", err)
			continue
		}
		out = append(out, *r)
	}
	if out == nil {
		out = []models.Record{}
	}
	if err := db.backfillDramaIDs(ctx, out); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(ctx, out); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	if err := db.backfillArtistIDs(ctx, out); err != nil {
		slog.Warn("backfill artist ids", "err", err)
	}
	return out, nil
}

// CountRecords runs the count with a background context; prefer
// CountRecordsContext in request scope.
func (db *DB) CountRecords(f RecordFilter) (int, error) {
	return db.CountRecordsContext(context.Background(), f)
}

// CountRecordsContext is CountRecords bound to ctx.
func (db *DB) CountRecordsContext(ctx context.Context, f RecordFilter) (int, error) {
	started := time.Now()
	defer func() {
		if ms := time.Since(started).Milliseconds(); ms >= slowQueryMs {
			slog.Warn("slow query", "op", "CountRecords", "ms", ms)
		}
	}()
	query := `SELECT COUNT(DISTINCT records.id) FROM records`
	where := []string{notDeleted}
	args := []interface{}{}

	appendSearchPredicate(f, &where, &args)
	if f.Category != "" {
		query += " JOIN json_each(records.category_names) je_cat ON je_cat.value = ?"
		args = append(args, f.Category)
	}
	if f.City != "" {
		where = append(where, "city = ?")
		args = append(args, f.City)
	}
	if f.DramaID != "" {
		query += " JOIN record_dramas rd ON rd.record_id = records.id"
		where = append(where, "rd.drama_id = ?")
		args = append(args, f.DramaID)
	}
	if f.ZheziID != "" {
		query += " JOIN record_zhezis rz ON rz.record_id = records.id"
		where = append(where, "rz.zhezi_id = ?")
		args = append(args, f.ZheziID)
	}
	if f.ArtistID != "" {
		query += " JOIN record_artists ra ON ra.record_id = records.id"
		where = append(where, "ra.artist_id = ?")
		args = append(args, f.ArtistID)
	}
	if f.Year > 0 && f.Month > 0 {
		start := time.Date(f.Year, time.Month(f.Month), 1, 0, 0, 0, 0, db.loc)
		end := start.AddDate(0, 1, 0)
		where = append(where, "date >= ? AND date < ?")
		args = append(args, start.Unix(), end.Unix())
	} else if f.Start != "" || f.End != "" {
		if t, ok := parseTimeArg(f.Start, db.loc); ok {
			where = append(where, "date >= ?")
			args = append(args, t.Unix())
		}
		if t, ok := parseTimeArg(f.End, db.loc); ok {
			where = append(where, "date < ?")
			args = append(args, t.AddDate(0, 0, 1).Unix())
		}
	}

	if f.Missing != "" {
		if p := buildMissingPredicate(f.Missing); p != "" {
			where = append(where, p)
		}
	}

	// 扩展维度：渠道/剧团精确匹配、评分下限、票价区间、状态过滤
	if f.Channel != "" {
		where = append(where, "channel = ?")
		args = append(args, f.Channel)
	}
	if f.Company != "" {
		where = append(where, "company = ?")
		args = append(args, f.Company)
	}
	if f.RatingMin > 0 {
		where = append(where, "rating >= ?")
		args = append(args, f.RatingMin)
	}
	if f.PriceMin > 0 {
		where = append(where, "price >= ?")
		args = append(args, f.PriceMin)
	}
	if f.PriceMax > 0 {
		where = append(where, "price <= ?")
		args = append(args, f.PriceMax)
	}
	appendStatusPredicate(f, &where, &args)

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	err := db.conn.QueryRowContext(ctx, query, args...).Scan(&total)
	return total, err
}

// missingFieldPredicates maps a "missing <field>" token to a SQL predicate that
// is TRUE when that field is empty. Numeric (rating) and JSON-array (category,
// guest) fields need dedicated handling; cover needs both the inline blob and
// the stored key to be absent before it counts as missing.
var missingFieldPredicates = map[string]string{
	"category":   "COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')",
	"city":       "city IS NULL OR city = ''",
	"address":    "address IS NULL OR address = ''",
	"company":    "company IS NULL OR company = ''",
	"channel":    "channel IS NULL OR channel = ''",
	"rating":     "rating IS NULL OR rating = 0",
	"price":      "price IS NULL OR price = ''",
	"cover":      "(cover IS NULL OR cover = '') AND (cover_file IS NULL OR cover_file = '')",
	"coordinate": "coordinate IS NULL OR coordinate = ''",
	"friends":    "friends IS NULL OR friends = ''",
	"remark":     "remark IS NULL OR remark = ''",
	"seat":       "seat IS NULL OR seat = ''",
	"play":       "play IS NULL OR play = ''",
	"guest":      "COALESCE(json_array_length(guest), 0) = 0",
}

// missingRelPredicates tests relation-backed fields via NOT EXISTS. These tables
// are kept in sync by backfill, so they are the canonical source for "does this
// record reference an artist/drama/zhezi".
var missingRelPredicates = map[string]string{
	"artist": "NOT EXISTS (SELECT 1 FROM record_artists ra WHERE ra.record_id = records.id)",
	"drama":  "NOT EXISTS (SELECT 1 FROM record_dramas rd WHERE rd.record_id = records.id)",
	"zhezi":  "NOT EXISTS (SELECT 1 FROM record_zhezis rz WHERE rz.record_id = records.id)",
}

// buildMissingPredicate turns a comma-separated Missing token list into a single
// parenthesized OR predicate (or "" when no known token is present). A record
// matches the filter when ANY listed field is empty — this is the data-hygiene
// query ("show me records missing a category", etc.).
func buildMissingPredicate(missing string) string {
	parts := make([]string, 0, 4)
	for _, raw := range strings.Split(missing, ",") {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		if p, ok := missingFieldPredicates[tok]; ok {
			parts = append(parts, "("+p+")")
		} else if p, ok := missingRelPredicates[tok]; ok {
			parts = append(parts, "("+p+")")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func parseTimeArg(s string, loc *time.Location) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	// Only treat the value as unix seconds when it is a pure integer; date
	// strings like "2026-08-22" start with digits and must not be misread as
	// an epoch timestamp.
	if isAllDigits(s) {
		if n, err := parseInt64(s); err == nil {
			return time.Unix(n, 0).In(loc), true
		}
	}
	for _, f := range []string{"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseDateText parses the human-facing date text accepted by record forms
// ("2026-08-23 19:30" etc). Returns ok=false when no format matches.
func parseDateText(s string, loc *time.Location) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, f := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func parseInt64(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func (db *DB) GetRecord(id string) (*models.Record, error) {
	row := db.conn.QueryRow(`SELECT `+recordColumns+` FROM records WHERE id = ? AND deleted_at = 0`, id)
	r, err := scanRecordRow(row)
	if err != nil {
		return nil, fmt.Errorf("record not found: %w", err)
	}
	rs := []models.Record{*r}
	if err := db.backfillDramaIDs(context.Background(), rs); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(context.Background(), rs); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	if err := db.backfillArtistIDs(context.Background(), rs); err != nil {
		slog.Warn("backfill artist ids", "err", err)
	}
	return &rs[0], nil
}

func scanRecordRow(row *sql.Row) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, guest, play, zheziIDs, tagIDs, categoryNames string
	)
	err := row.Scan(
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &categoryNames, &guest, &play, &zheziIDs, &tagIDs,
		&r.Date, &r.DateText, &r.Rating, &r.Duration, &r.Seat, &r.Friends, &r.Company, &r.Remark, &r.ActiveStatus,
		&r.Price, &r.PriceCurrency, &r.PayPrice, &r.PayPriceCurrency, &r.OtherCost, &r.OtherCostCurrency, &r.TotalCost,
	)
	if err != nil {
		return nil, err
	}
	r.Coordinate = unmarshalCoordinate(coordinate)
	r.Guest = unmarshalStrings(guest)
	r.Play = unmarshalStrings(play)
	r.ZheziIDs = unmarshalStrings(zheziIDs)
	r.TagIDs = unmarshalStrings(tagIDs)
	applyCategoryFallback(&r, categoryNames)
	return &r, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// sqlExecutor abstracts *sql.DB and *sql.Tx so the same write logic can run in
// a single statement or inside an explicit transaction (needed because we pin
// MaxOpenConns(1): within a tx every write must go through the tx, never the
// pool, or the only connection deadlocks).
type sqlExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

// recordUpsertSQL is the parameterized upsert statement for a single record.
// Kept as a package var so it can be prepared once and reused (see stmt cache).
const recordUpsertSQL = `
	INSERT INTO records (
		id, name, channel, city, address, coordinate, cover, cover_file, cover_thumb,
		custom_category_id, category_name, category_names, artist_names, guest, play, drama_ids, zhezi_ids, tag_ids,
		date, date_text, rating, duration, seat, friends, company, remark, active_status,
		price, price_currency, pay_price, pay_price_currency, other_cost, other_cost_currency, total_cost
	) VALUES (
		?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
	)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name, channel=excluded.channel, city=excluded.city, address=excluded.address,
		coordinate=excluded.coordinate, cover=excluded.cover, cover_file=excluded.cover_file, cover_thumb=excluded.cover_thumb,
		custom_category_id=excluded.custom_category_id, category_name=excluded.category_name,
		category_names=excluded.category_names,
		artist_names=excluded.artist_names, guest=excluded.guest, play=excluded.play,
		drama_ids=excluded.drama_ids, zhezi_ids=excluded.zhezi_ids, tag_ids=excluded.tag_ids,
		date=excluded.date, date_text=excluded.date_text, rating=excluded.rating, duration=excluded.duration,
		seat=excluded.seat, friends=excluded.friends, company=excluded.company, remark=excluded.remark, active_status=excluded.active_status,
		price=excluded.price, price_currency=excluded.price_currency, pay_price=excluded.pay_price,
		pay_price_currency=excluded.pay_price_currency, other_cost=excluded.other_cost, other_cost_currency=excluded.other_cost_currency,
		total_cost=excluded.total_cost
`

// normalizeCategories reconciles the scalar primary category with the
// multi-category array on any record about to be written:
//   - array wins: a non-empty CategoryNames defines the truth and
//     CategoryName is pinned to element 0;
//   - scalar fallback: an empty array promotes CategoryName into a
//     single-element array so reads never see an out-of-sync pair.
//
// The same invariant is applied to dramas via normalizeDramaCategories; on
// read a manually stored list wins over the performance aggregation.
func normalizeCategories(r *models.Record) {
	names := make([]string, 0, len(r.CategoryNames))
	for _, n := range r.CategoryNames {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 && strings.TrimSpace(r.CategoryName) != "" {
		names = []string{strings.TrimSpace(r.CategoryName)}
	}
	r.CategoryNames = names
	if len(names) > 0 {
		r.CategoryName = names[0]
	}
}

// normalizeDramaCategories trims the manually maintained category list and
// keeps the scalar primary in sync (element 0). An empty list means "derive
// from performances" on read.
func normalizeDramaCategories(d *models.Drama) {
	names := make([]string, 0, len(d.CategoryNames))
	for _, n := range d.CategoryNames {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	d.CategoryNames = names
	if len(names) > 0 {
		d.CategoryName = names[0]
	} else {
		d.CategoryName = ""
	}
}

// UpsertRecord inserts or updates a single record. For bulk imports prefer
// BulkUpsertRecords, which wraps all rows in one transaction (far fewer fsyncs).
func (db *DB) UpsertRecord(r models.Record) error {
	if r.ID == "" {
		r.ID = newID()
	}
	normalizeCategories(&r)
	// Compute total_cost: effective price + other_cost
	r.TotalCost = (func() float64 {
		if r.PayPrice > 0 {
			return r.PayPrice
		}
		return r.Price
	})() + r.OtherCost
	if _, err := db.stmtUpsertRecord.Exec(
		r.ID, r.Name, r.Channel, r.City, r.Address, marshalJSON(r.Coordinate), r.Cover, r.CoverFile, r.CoverThumb,
		r.CustomCategoryID, r.CategoryName, marshalJSON(r.CategoryNames), marshalJSON(r.ArtistNames), marshalJSON(r.Guest), marshalJSON(r.Play),
		marshalJSON(r.DramaIDs), marshalJSON(r.ZheziIDs), marshalJSON(r.TagIDs),
		r.Date, r.DateText, r.Rating, r.Duration, r.Seat, r.Friends, r.Company, r.Remark, r.ActiveStatus,
		r.Price, r.PriceCurrency, r.PayPrice, r.PayPriceCurrency, r.OtherCost, r.OtherCostCurrency, r.TotalCost,
	); err != nil {
		return err
	}
	// Keep the drama/zhezi/artist relation tables in sync with the upserted record.
	if err := db.setRecordDramas(db.conn, r.ID, r.DramaIDs); err != nil {
		return err
	}
	if err := db.setRecordZhezis(db.conn, r.ID, r.ZheziIDs); err != nil {
		return err
	}
	return db.setRecordArtists(db.conn, r.ID, r.ArtistIDs, r.ArtistNames)
}

// UpsertRecordTx is like UpsertRecord but runs inside the supplied transaction.
// Used by ImportData so the whole import shares one transaction. It executes the
// raw SQL directly (not the pool-prepared statement) to avoid a connection-pool
// deadlock when MaxOpenConns(1): binding a pool stmt into a tx would try to
// re-acquire the only connection, which the tx already holds.
func (db *DB) UpsertRecordTx(tx *sql.Tx, r models.Record) error {
	if r.ID == "" {
		r.ID = newID()
	}
	normalizeCategories(&r)
	// Compute total_cost: effective price + other_cost
	r.TotalCost = (func() float64 {
		if r.PayPrice > 0 {
			return r.PayPrice
		}
		return r.Price
	})() + r.OtherCost
	if _, err := tx.Exec(recordUpsertSQL,
		r.ID, r.Name, r.Channel, r.City, r.Address, marshalJSON(r.Coordinate), r.Cover, r.CoverFile, r.CoverThumb,
		r.CustomCategoryID, r.CategoryName, marshalJSON(r.CategoryNames), marshalJSON(r.ArtistNames), marshalJSON(r.Guest), marshalJSON(r.Play),
		marshalJSON(r.DramaIDs), marshalJSON(r.ZheziIDs), marshalJSON(r.TagIDs),
		r.Date, r.DateText, r.Rating, r.Duration, r.Seat, r.Friends, r.Company, r.Remark, r.ActiveStatus,
		r.Price, r.PriceCurrency, r.PayPrice, r.PayPriceCurrency, r.OtherCost, r.OtherCostCurrency, r.TotalCost,
	); err != nil {
		return err
	}
	// Keep the drama/zhezi/artist relation tables in sync within the same transaction.
	if err := db.setRecordDramas(tx, r.ID, r.DramaIDs); err != nil {
		return err
	}
	if err := db.setRecordZhezis(tx, r.ID, r.ZheziIDs); err != nil {
		return err
	}
	return db.setRecordArtists(tx, r.ID, r.ArtistIDs, r.ArtistNames)
}

// BulkUpsertRecords inserts/updates many records inside a single transaction.
// On SQLite this collapses N fsyncs into one, making imports orders of magnitude
// faster than calling UpsertRecord in a loop.
func (db *DB) BulkUpsertRecords(records []models.Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin bulk upsert: %w", err)
	}
	defer tx.Rollback()
	for i := range records {
		if err := db.UpsertRecordTx(tx, records[i]); err != nil {
			return fmt.Errorf("bulk upsert record %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// dramaNames resolves a list of drama ids to their names, preserving order.
func (db *DB) dramaNames(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := db.conn.Query("SELECT id, name FROM dramas WHERE id IN ("+strings.Join(placeholders, ",")+")", args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	byID := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			byID[id] = name
		}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byID[id]; ok {
			out = append(out, name)
		}
	}
	return out
}

// setRecordDramas replaces all drama links for a record with the given ids,
// preserving order. Used after upserting a record so the relation table stays
// the source of truth (records.drama_ids is only a legacy fallback column).
func (db *DB) setRecordDramas(exec sqlExecutor, recordID string, ids []string) error {
	if _, err := exec.Exec("DELETE FROM record_dramas WHERE record_id = ?", recordID); err != nil {
		return err
	}
	for i, id := range ids {
		if id == "" {
			continue
		}
		if _, err := exec.Exec(
			"INSERT OR IGNORE INTO record_dramas (record_id, drama_id, sort_order) VALUES (?, ?, ?)",
			recordID, id, i,
		); err != nil {
			return err
		}
	}
	return nil
}

// setRecordZhezis mirrors setRecordDramas for the zhezi relation table.
func (db *DB) setRecordZhezis(exec sqlExecutor, recordID string, ids []string) error {
	if _, err := exec.Exec("DELETE FROM record_zhezis WHERE record_id = ?", recordID); err != nil {
		return err
	}
	for i, id := range ids {
		if id == "" {
			continue
		}
		if _, err := exec.Exec(
			"INSERT OR IGNORE INTO record_zhezis (record_id, zhezi_id, sort_order) VALUES (?, ?, ?)",
			recordID, id, i,
		); err != nil {
			return err
		}
	}
	return nil
}

// setRecordArtists mirrors setRecordDramas but links actor entities via the
// record_artists relation table. It accepts both artist IDs (preferred, linked
// directly) and actor names (resolved to an entity by name/alias, created on
// demand). The IDs path is what the new record form sends (artist_ids picked
// from the tree); the names path preserves backward compatibility with legacy
// artist_names text. resolveArtistByName runs on the same exec context as the
// link writes, so a name resolution inside an import transaction never writes
// through the pool while that transaction holds the SQLite write lock.
func (db *DB) setRecordArtists(exec sqlExecutor, recordID string, ids, names []string) error {
	if _, err := exec.Exec("DELETE FROM record_artists WHERE record_id = ?", recordID); err != nil {
		return err
	}
	order := 0
	// 1) Link by IDs first (these are the canonical entities from the picker).
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := exec.Exec(
			"INSERT OR IGNORE INTO record_artists (record_id, artist_id, sort_order) VALUES (?, ?, ?)",
			recordID, id, order,
		); err != nil {
			return err
		}
		order++
	}
	// 2) Fall back to names: resolve to an entity (create if missing). Skip any
	// name already represented by an ID we just linked.
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		aid, err := db.resolveArtistByName(exec, name)
		if err != nil {
			return err
		}
		if seen[aid] {
			continue
		}
		seen[aid] = true
		if _, err := exec.Exec(
			"INSERT OR IGNORE INTO record_artists (record_id, artist_id, sort_order) VALUES (?, ?, ?)",
			recordID, aid, order,
		); err != nil {
			return err
		}
		order++
	}
	return nil
}

// backfillDramaIDs loads drama ids for the given records from the relation
// table in a single batched query and fills models.Record.DramaIDs.
func (db *DB) backfillDramaIDs(ctx context.Context, records []models.Record) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT record_id, drama_id FROM record_dramas WHERE record_id IN ("+strings.Join(ph, ",")+") ORDER BY record_id, sort_order",
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	byRecord := map[string][]string{}
	for rows.Next() {
		var recordID, dramaID string
		if err := rows.Scan(&recordID, &dramaID); err != nil {
			continue
		}
		byRecord[recordID] = append(byRecord[recordID], dramaID)
	}
	for i := range records {
		if links, ok := byRecord[records[i].ID]; ok {
			records[i].DramaIDs = links
		} else {
			records[i].DramaIDs = []string{}
		}
	}
	return nil
}

// backfillZheziIDs mirrors backfillDramaIDs for the zhezi relation table.
func (db *DB) backfillZheziIDs(ctx context.Context, records []models.Record) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT record_id, zhezi_id FROM record_zhezis WHERE record_id IN ("+strings.Join(ph, ",")+") ORDER BY record_id, sort_order",
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	byRecord := map[string][]string{}
	for rows.Next() {
		var recordID, zheziID string
		if err := rows.Scan(&recordID, &zheziID); err != nil {
			continue
		}
		byRecord[recordID] = append(byRecord[recordID], zheziID)
	}
	for i := range records {
		if links, ok := byRecord[records[i].ID]; ok {
			records[i].ZheziIDs = links
		} else {
			records[i].ZheziIDs = []string{}
		}
	}
	return nil
}

// backfillArtistIDs loads actor ids+names for the given records from the
// record_artists relation table in a single batched query and fills both
// models.Record.ArtistIDs and models.Record.ArtistNames (names resolved by id,
// preserving sort order).
func (db *DB) backfillArtistIDs(ctx context.Context, records []models.Record) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.QueryContext(ctx,
		"SELECT ra.record_id, ra.artist_id, a.name FROM record_artists ra JOIN artists a ON a.id = ra.artist_id WHERE ra.record_id IN ("+strings.Join(ph, ",")+") ORDER BY ra.record_id, ra.sort_order",
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	type link struct{ id, name string }
	byRecord := map[string][]link{}
	for rows.Next() {
		var recordID, artistID, name string
		if err := rows.Scan(&recordID, &artistID, &name); err != nil {
			continue
		}
		byRecord[recordID] = append(byRecord[recordID], link{artistID, name})
	}
	for i := range records {
		if links, ok := byRecord[records[i].ID]; ok {
			idsOut := make([]string, len(links))
			namesOut := make([]string, len(links))
			for j, l := range links {
				idsOut[j] = l.id
				namesOut[j] = l.name
			}
			records[i].ArtistIDs = idsOut
			records[i].ArtistNames = namesOut
		} else {
			records[i].ArtistIDs = []string{}
			records[i].ArtistNames = []string{}
		}
	}
	return nil
}

func (db *DB) CreateRecord(r models.RecordRequest) (*models.Record, error) {
	rec := requestToRecord(r)
	rec.ID = newID()
	if names := db.dramaNames(r.DramaIDs); names != nil {
		rec.Play = names
	}
	if err := db.UpsertRecord(rec); err != nil {
		return nil, err
	}
	// 同场馆（同地址）坐标保持一致：把其他同地址演出的坐标同步为本次值
	db.SyncVenueCoordinates(rec.Address, rec.Coordinate, rec.ID)
	return db.GetRecord(rec.ID)
}

func (db *DB) UpdateRecord(id string, r models.RecordRequest) (*models.Record, error) {
	existing, err := db.GetRecord(id)
	if err != nil {
		return nil, err
	}
	rec := requestToRecord(r)
	rec.ID = existing.ID
	rec.Cover = existing.Cover
	// 封面仅在请求携带非空且不同的 coverFile 时才更新：PUT 可能来自不带
	// 封面字段的客户端，直接透传空值会把已有关联清空。更换新文件时缩略图
	// 跟随请求值；同一张图则保留原有缩略图不被覆盖。
	rec.CoverFile = existing.CoverFile
	rec.CoverThumb = existing.CoverThumb
	if r.CoverFile != "" && r.CoverFile != existing.CoverFile {
		rec.CoverFile = r.CoverFile
		rec.CoverThumb = r.CoverThumb
	}
	rec.CustomCategoryID = existing.CustomCategoryID // keep legacy mapping unless provided
	_ = rec.CustomCategoryID
	if r.CustomCategoryID != "" {
		rec.CustomCategoryID = r.CustomCategoryID
	}
	if names := db.dramaNames(r.DramaIDs); names != nil {
		rec.Play = names
	}
	if err := db.UpsertRecord(rec); err != nil {
		return nil, err
	}
	// 同场馆（同地址）坐标保持一致：把其他同地址演出的坐标同步为本次值
	db.SyncVenueCoordinates(rec.Address, rec.Coordinate, rec.ID)
	return db.GetRecord(id)
}

// SyncVenueCoordinates 将同一 address 下（排除 excludeID）的所有演出坐标
// 同步为 coord。addr 为空或 coord 为空时不操作（避免把其他记录错误清空）。
func (db *DB) SyncVenueCoordinates(addr string, coord *models.Coordinate, excludeID string) (int64, error) {
	if addr == "" || coord == nil {
		return 0, nil
	}
	res, err := db.conn.Exec(
		"UPDATE records SET coordinate = ? WHERE address = ? AND id != ? AND deleted_at = 0",
		marshalJSON(coord), addr, excludeID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AlignVenueResult 统计批量对齐的结果。
type AlignVenueResult struct {
	GroupsTotal    int   `json:"groups_total"`
	GroupsAligned  int   `json:"groups_aligned"`
	RecordsUpdated int64 `json:"records_updated"`
}

// AlignVenueCoordinates 存量对齐：按 address 分组，用各组里第一个已有坐标
// 回填同地址下坐标不同的其他记录。整组都无坐标的地址跳过。
func (db *DB) AlignVenueCoordinates() (*AlignVenueResult, error) {
	res := &AlignVenueResult{}
	rows, err := db.conn.Query("SELECT DISTINCT address FROM records WHERE deleted_at = 0 AND address != ''")
	if err != nil {
		return nil, err
	}
	var addrs []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return nil, err
		}
		addrs = append(addrs, a)
	}
	rows.Close()

	for _, addr := range addrs {
		res.GroupsTotal++
		var rep string
		gr, err := db.conn.Query("SELECT coordinate FROM records WHERE deleted_at = 0 AND address = ?", addr)
		if err != nil {
			return nil, err
		}
		for gr.Next() {
			var c string
			if err := gr.Scan(&c); err != nil {
				gr.Close()
				return nil, err
			}
			if rep == "" && c != "" && c != "null" {
				rep = c
			}
		}
		gr.Close()
		if rep == "" {
			continue // 整组都无坐标，跳过
		}
		res.GroupsAligned++
		upd, err := db.conn.Exec(
			"UPDATE records SET coordinate = ? WHERE address = ? AND coordinate != ? AND deleted_at = 0",
			rep, addr, rep,
		)
		if err != nil {
			return nil, err
		}
		n, _ := upd.RowsAffected()
		res.RecordsUpdated += n
	}
	return res, nil
}

func requestToRecord(r models.RecordRequest) models.Record {
	a := r.ArtistNames
	if a == nil {
		a = []string{}
	}
	aid := r.ArtistIDs
	if aid == nil {
		aid = []string{}
	}
	g := r.Guest
	if g == nil {
		g = []string{}
	}
	p := r.Play
	if p == nil {
		p = []string{}
	}
	d := r.DramaIDs
	if d == nil {
		d = []string{}
	}
	z := r.ZheziIDs
	if z == nil {
		z = []string{}
	}
	t := r.TagIDs
	if t == nil {
		t = []string{}
	}
	return models.Record{
		Name: r.Name, Channel: r.Channel, City: r.City, Address: r.Address,
		Coordinate: r.Coordinate, Cover: r.Cover, CoverFile: r.CoverFile, CoverThumb: r.CoverThumb,
		CustomCategoryID: r.CustomCategoryID, CategoryName: r.CategoryName, CategoryNames: r.CategoryNames,
		ArtistIDs: aid, ArtistNames: a, Guest: g, Play: p, DramaIDs: d, ZheziIDs: z, TagIDs: t,
		Date: r.Date, DateText: r.DateText, Rating: r.Rating, Duration: r.Duration, Seat: r.Seat,
		Friends: r.Friends, Company: r.Company, Remark: r.Remark, ActiveStatus: r.ActiveStatus,
		Price: r.Price, PriceCurrency: r.PriceCurrency, PayPrice: r.PayPrice,
		PayPriceCurrency: r.PayPriceCurrency, OtherCost: r.OtherCost, OtherCostCurrency: r.OtherCostCurrency,
	}
}

func (db *DB) DeleteRecord(id string) error {
	// Soft delete: the record moves to the trash (回收站). Relation rows are
	// kept so a restore brings everything back; every read path filters
	// deleted_at = 0, so counts and lists drop the record immediately.
	return db.SoftDeleteRecord(id)
}

// BatchUpdateRecords accepts models.BatchUpdateParams for batch field updates.
// All scalar fields are applied in a single UPDATE ... WHERE id IN (...) query;
// JSON-array fields (drama_ids / zhezi_ids / play / guest / artist_names /
// tag_ids) are processed row-by-row so each record's existing values are
// combined correctly (set / append / remove).
func (db *DB) BatchUpdateRecords(params models.BatchUpdateParams) (int64, error) {
	if len(params.IDs) == 0 {
		return 0, nil
	}

	// 1. Build simple (scalar) SET clause — can update in one SQL
	simpleSets := []string{}
	simpleArgs := []interface{}{}

	if params.Name != nil {
		simpleSets = append(simpleSets, "name = ?")
		simpleArgs = append(simpleArgs, *params.Name)
	}
	if params.CategoryName != nil {
		simpleSets = append(simpleSets, "category_name = ?")
		simpleArgs = append(simpleArgs, *params.CategoryName)
	}
	if params.Rating != nil {
		simpleSets = append(simpleSets, "rating = ?")
		simpleArgs = append(simpleArgs, *params.Rating)
	}
	if params.ActiveStatus != nil {
		simpleSets = append(simpleSets, "active_status = ?")
		simpleArgs = append(simpleArgs, *params.ActiveStatus)
	}
	if params.City != nil {
		simpleSets = append(simpleSets, "city = ?")
		simpleArgs = append(simpleArgs, *params.City)
	}
	if params.Address != nil {
		simpleSets = append(simpleSets, "address = ?")
		simpleArgs = append(simpleArgs, *params.Address)
	}
	if params.Channel != nil {
		simpleSets = append(simpleSets, "channel = ?")
		simpleArgs = append(simpleArgs, *params.Channel)
	}
	if params.Company != nil {
		simpleSets = append(simpleSets, "company = ?")
		simpleArgs = append(simpleArgs, *params.Company)
	}
	if params.Friends != nil {
		simpleSets = append(simpleSets, "friends = ?")
		simpleArgs = append(simpleArgs, *params.Friends)
	}
	if params.Remark != nil {
		simpleSets = append(simpleSets, "remark = ?")
		simpleArgs = append(simpleArgs, *params.Remark)
	}
	if params.Seat != nil {
		simpleSets = append(simpleSets, "seat = ?")
		simpleArgs = append(simpleArgs, *params.Seat)
	}
	if params.DateText != nil {
		// 演出时间以文本为准，解析后联动 unix date 列；空串表示清空。
		if t, ok := parseDateText(*params.DateText, db.loc); ok {
			simpleSets = append(simpleSets, "date = ?", "date_text = ?")
			simpleArgs = append(simpleArgs, t.Unix(), t.Format("2006-01-02 15:04"))
		} else if *params.DateText == "" {
			simpleSets = append(simpleSets, "date = ?", "date_text = ?")
			simpleArgs = append(simpleArgs, 0, "")
		}
	}
	if params.Coordinate != nil {
		simpleSets = append(simpleSets, "coordinate = ?")
		simpleArgs = append(simpleArgs, marshalJSON(params.Coordinate))
	}
	if params.Price != nil {
		simpleSets = append(simpleSets, "price = ?")
		simpleArgs = append(simpleArgs, *params.Price)
	}
	if params.PriceCurrency != nil {
		simpleSets = append(simpleSets, "price_currency = ?")
		simpleArgs = append(simpleArgs, *params.PriceCurrency)
	}
	if params.PayPrice != nil {
		simpleSets = append(simpleSets, "pay_price = ?")
		simpleArgs = append(simpleArgs, *params.PayPrice)
	}
	if params.PayPriceCurrency != nil {
		simpleSets = append(simpleSets, "pay_price_currency = ?")
		simpleArgs = append(simpleArgs, *params.PayPriceCurrency)
	}
	if params.OtherCost != nil {
		simpleSets = append(simpleSets, "other_cost = ?")
		simpleArgs = append(simpleArgs, *params.OtherCost)
	}
	if params.OtherCostCurrency != nil {
		simpleSets = append(simpleSets, "other_cost_currency = ?")
		simpleArgs = append(simpleArgs, *params.OtherCostCurrency)
	}

	hasArrayOps := params.DramaIDs != nil || params.ZheziIDs != nil ||
		params.Play != nil || params.Guest != nil ||
		params.ArtistNames != nil ||
		params.CategoryNames != nil

	// 2. Apply simple scalar updates (one SQL for all)
	if len(simpleSets) > 0 {
		placeholders := make([]string, len(params.IDs))
		inArgs := make([]interface{}, len(params.IDs))
		for i, id := range params.IDs {
			placeholders[i] = "?"
			inArgs[i] = id
		}
		sql := "UPDATE records SET " + strings.Join(simpleSets, ", ") + " WHERE deleted_at = 0 AND id IN (" + strings.Join(placeholders, ",") + ")"
		args := append(simpleArgs, inArgs...)
		if _, err := db.conn.Exec(sql, args...); err != nil {
			return 0, err
		}
	}

	// 3. Handle array ops row-by-row (each row may have different existing values)
	if hasArrayOps {
		updated, err := db.applyArrayOps(params)
		if err != nil {
			return 0, err
		}
		return updated, nil
	}

	return int64(len(params.IDs)), nil
}

// applyArrayOps applies set/append/remove operations on JSON-array columns
// for each affected record.
func (db *DB) applyArrayOps(params models.BatchUpdateParams) (int64, error) {
	arrayCols := map[string]*models.BatchArrayOp{}
	if params.DramaIDs != nil {
		arrayCols["drama_ids"] = params.DramaIDs
	}
	if params.ZheziIDs != nil {
		arrayCols["zhezi_ids"] = params.ZheziIDs
	}
	if params.Play != nil {
		arrayCols["play"] = params.Play
	}
	if params.Guest != nil {
		arrayCols["guest"] = params.Guest
	}
	if params.ArtistNames != nil {
		arrayCols["artist_names"] = params.ArtistNames
	}
	if params.CategoryNames != nil {
		arrayCols["category_names"] = params.CategoryNames
	}

	var total int64
	for _, id := range params.IDs {
		colUpdates := []string{}
		colArgs := []interface{}{}

		for col, op := range arrayCols {
			var existing []string
			row := db.conn.QueryRow("SELECT "+col+" FROM records WHERE id = ? AND deleted_at = 0", id)
			var raw string
			if err := row.Scan(&raw); err != nil {
				continue
			}
			if raw != "" && raw != "[]" {
				_ = json.Unmarshal([]byte(raw), &existing)
			}

			newVal := applyArrayOp(existing, op)
			newRaw, _ := json.Marshal(newVal)
			colUpdates = append(colUpdates, col+" = ?")
			colArgs = append(colArgs, string(newRaw))
			// Keep the scalar primary category in sync with the array.
			if col == "category_names" {
				primary := ""
				if len(newVal) > 0 {
					primary = newVal[0]
				}
				colUpdates = append(colUpdates, "category_name = ?")
				colArgs = append(colArgs, primary)
			}
		}

		if len(colUpdates) > 0 {
			colArgs = append(colArgs, id)
			sql := "UPDATE records SET " + strings.Join(colUpdates, ", ") + " WHERE id = ? AND deleted_at = 0"
			if _, err := db.conn.Exec(sql, colArgs...); err != nil {
				return total, err
			}
			// 关联表同步：读取路径从 record_dramas / record_zhezis /
			// record_artists 回填（JSON 列只是遗留兜底），批量改写后必须
			// 镜像到关联表，否则变更对查询"不可见"。
			if err := db.syncRecordRelationsAfterBatch(db.conn, id, arrayCols); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
}

// syncRecordRelationsAfterBatch mirrors the JSON-array columns rewritten by a
// batch array op into the relation tables, mirroring what UpsertRecord does
// for whole-record writes.
func (db *DB) syncRecordRelationsAfterBatch(exec sqlExecutor, recordID string, cols map[string]*models.BatchArrayOp) error {
	selectCol := func(col string) ([]string, error) {
		var raw string
		if err := exec.QueryRow("SELECT "+col+" FROM records WHERE id = ? AND deleted_at = 0", recordID).Scan(&raw); err != nil {
			return nil, err
		}
		return unmarshalStrings(raw), nil
	}
	if cols["drama_ids"] != nil {
		ids, err := selectCol("drama_ids")
		if err != nil {
			return err
		}
		if err := db.setRecordDramas(exec, recordID, ids); err != nil {
			return err
		}
	}
	if cols["zhezi_ids"] != nil {
		ids, err := selectCol("zhezi_ids")
		if err != nil {
			return err
		}
		if err := db.setRecordZhezis(exec, recordID, ids); err != nil {
			return err
		}
	}
	if cols["artist_names"] != nil {
		// records 表没有 artist_ids 列：按名字解析（resolveArtistByName
		// 会自动补建缺失档案），与 UpsertRecord 的 names 兜底路径一致。
		names, err := selectCol("artist_names")
		if err != nil {
			return err
		}
		if err := db.setRecordArtists(exec, recordID, nil, names); err != nil {
			return err
		}
	}
	return nil
}

// applyArrayOp applies the op (set/append/remove) to existing values.
func applyArrayOp(existing []string, op *models.BatchArrayOp) []string {
	if op == nil {
		return existing
	}
	switch op.Op {
	case "set":
		return op.Value
	case "append":
		// 保持既有顺序、新值按传入顺序去重追加 —— 主元素（如主剧种）
		// 必须稳定，不能因 map 遍历顺序而漂移。
		set := make(map[string]struct{}, len(existing)+len(op.Value))
		for _, v := range existing {
			set[v] = struct{}{}
		}
		out := slices.Clone(existing)
		for _, v := range op.Value {
			if _, dup := set[v]; dup {
				continue
			}
			out = append(out, v)
			set[v] = struct{}{}
		}
		return out
	case "remove":
		removeSet := make(map[string]struct{}, len(op.Value))
		for _, v := range op.Value {
			removeSet[v] = struct{}{}
		}
		return slices.DeleteFunc(existing, func(v string) bool {
			_, ok := removeSet[v]
			return ok
		})
	default:
		return existing
	}
}

func (db *DB) BatchDeleteRecords(ids []string) (int64, error) {
	// 批量删除同样进入回收站（软删除）
	return db.SoftDeleteRecords(ids)
}

// ---------- Categories ----------

// ListCategories returns all categories ordered by manual sort order then name.
// active_ids is no longer stored: it was a redundant copy of
// "records WHERE category_name = ? AND active_status = <watching>" and is now
// derived on demand (see GetCategory). We keep models.Category.ActiveIDs as an
// empty slice for backward-compatible JSON.
func (db *DB) ListCategories() ([]models.Category, error) {
	rows, err := db.conn.Query(`SELECT id, name, record_count, sort_order FROM categories ORDER BY sort_order ASC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.RecordCount, &c.SortOrder); err != nil {
			continue
		}
		c.ActiveIDs = []string{}
		out = append(out, c)
	}
	if out == nil {
		out = []models.Category{}
	}
	return out, nil
}

// upsertCategoryExec inserts/updates a category against the given executor.
// active_ids is no longer persisted (derived from records); we ignore the
// incoming ActiveIDs field.
func upsertCategoryExec(exec sqlExecutor, c *models.Category) error {
	if c.ID == "" {
		c.ID = newID()
		// New categories append after any manually ordered ones.
		exec.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM categories").Scan(&c.SortOrder)
	}
	_, err := exec.Exec(`
		INSERT INTO categories (id, name, record_count, sort_order) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, record_count=excluded.record_count
	`, c.ID, c.Name, c.RecordCount, c.SortOrder)
	return err
}

func (db *DB) UpsertCategory(c *models.Category) error {
	return upsertCategoryExec(db.conn, c)
}

// UpsertCategoryTx runs inside the supplied transaction (used by ImportData so
// the whole import shares one connection under MaxOpenConns(1)).
func (db *DB) UpsertCategoryTx(tx *sql.Tx, c *models.Category) error {
	return upsertCategoryExec(tx, c)
}

// ReorderCategories sets the manual sort order of categories from an explicit
// ordered id list (first = top). Categories not in the list keep their order.
func (db *DB) ReorderCategories(orderedIDs []string) error {
	for i, id := range orderedIDs {
		if _, err := db.conn.Exec("UPDATE categories SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) DeleteCategory(id string) error {
	_, err := db.conn.Exec("DELETE FROM categories WHERE id = ?", id)
	return err
}

// ---------- Dramas & Zhezis ----------

// dramaCategoriesAgg aggregates the categories actually used by the
// performances that reference each drama, ordered by usage count descending.
// The drama archive itself no longer stores editable categories: the shows
// that stage it are the single source of truth.
func (db *DB) dramaCategoriesAll() (map[string][]string, error) {
	rows, err := db.conn.Query(`
		WITH solo AS (
			SELECT r.id AS record_id
			FROM records r, json_each(r.category_names) je
			WHERE r.deleted_at = 0 AND je.value != ''
			GROUP BY r.id HAVING COUNT(DISTINCT je.value) = 1
		)
		SELECT rd.drama_id, je.value AS cat, COUNT(*) AS cnt
		FROM record_dramas rd
		JOIN records r ON r.id = rd.record_id
		JOIN json_each(r.category_names) je
		JOIN solo s ON s.record_id = rd.record_id
		WHERE r.deleted_at = 0 AND je.value != ''
		GROUP BY rd.drama_id, cat
		ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var dramaID, cat string
		var cnt int
		if err := rows.Scan(&dramaID, &cat, &cnt); err != nil {
			continue
		}
		out[dramaID] = append(out[dramaID], cat)
	}
	return out, rows.Err()
}

// applyDramaCategories fills the derived categories: a manually set
// category_names on the drama archive wins; otherwise fall back to the
// aggregation of categories used by the performances staging it (多剧种拼盘
// 演出会把无关剧种带进聚合，手动覆盖用于修正这类场景).
func applyDramaCategories(d *models.Drama, raw string, agg map[string][]string) {
	d.CategoryNames = unmarshalStrings(raw)
	if len(d.CategoryNames) == 0 {
		d.CategoryNames = agg[d.ID]
	}
	if d.CategoryNames == nil {
		d.CategoryNames = []string{}
	}
	if len(d.CategoryNames) > 0 {
		d.CategoryName = d.CategoryNames[0]
	} else {
		d.CategoryName = ""
	}
}

func (db *DB) ListDramas() ([]models.Drama, error) {
	type rawDrama struct {
		d      models.Drama
		manual string
	}
	agg, err := db.dramaCategoriesAll()
	if err != nil {
		return nil, err
	}
	rows, err := db.conn.Query(`
		SELECT d.id, d.name, d.aliases, d.category_names, d.remark, d.sort_order,
			(SELECT COUNT(*) FROM zhezis z WHERE z.drama_id = d.id) AS zhezi_count,
			(SELECT COUNT(*) FROM record_dramas rd JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0 WHERE rd.drama_id = d.id) AS record_count
		FROM dramas d ORDER BY d.sort_order ASC, d.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	var raw []rawDrama
	for rows.Next() {
		var r rawDrama
		var aliases string
		if err := rows.Scan(&r.d.ID, &r.d.Name, &aliases, &r.manual, &r.d.Remark, &r.d.SortOrder, &r.d.ZheziCount, &r.d.RecordCount); err != nil {
			continue
		}
		r.d.Aliases = unmarshalStrings(aliases)
		raw = append(raw, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.Drama, 0, len(raw))
	for i := range raw {
		applyDramaCategories(&raw[i].d, raw[i].manual, agg)
		out = append(out, raw[i].d)
	}
	return out, nil
}

func (db *DB) GetDrama(id string) (*models.Drama, error) {
	var d models.Drama
	var manual string
	var aliases string
	err := db.conn.QueryRow(`
		SELECT d.id, d.name, d.aliases, d.category_names, d.remark, d.sort_order,
			(SELECT COUNT(*) FROM zhezis z WHERE z.drama_id = d.id),
			(SELECT COUNT(*) FROM record_dramas rd JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0 WHERE rd.drama_id = d.id)
		FROM dramas d WHERE d.id = ?`, id).
		Scan(&d.ID, &d.Name, &aliases, &manual, &d.Remark, &d.SortOrder, &d.ZheziCount, &d.RecordCount)
	if err != nil {
		return nil, fmt.Errorf("drama not found: %w", err)
	}
	d.Aliases = unmarshalStrings(aliases)
	cats, err := db.dramaCategoriesFor(id)
	if err != nil {
		return nil, err
	}
	applyDramaCategories(&d, manual, map[string][]string{id: cats})
	return &d, nil
}

// dramaCategoriesFor aggregates used categories for a single drama.
func (db *DB) dramaCategoriesFor(dramaID string) ([]string, error) {
	rows, err := db.conn.Query(`
		WITH solo AS (
			SELECT r.id AS record_id
			FROM records r, json_each(r.category_names) je
			WHERE r.deleted_at = 0 AND je.value != ''
			GROUP BY r.id HAVING COUNT(DISTINCT je.value) = 1
		)
		SELECT je.value, COUNT(*) AS cnt
		FROM record_dramas rd
		JOIN records r ON r.id = rd.record_id
		JOIN json_each(r.category_names) je
		JOIN solo s ON s.record_id = rd.record_id
		WHERE r.deleted_at = 0 AND rd.drama_id = ? AND je.value != ''
		GROUP BY je.value ORDER BY cnt DESC`, dramaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			continue
		}
		out = append(out, cat)
	}
	return out, rows.Err()
}

// ReorderDramas sets the manual sort order of dramas from an explicit ordered
// id list (first = top). Dramas not in the list keep their previous order.
func (db *DB) ReorderDramas(orderedIDs []string) error {
	for i, id := range orderedIDs {
		if _, err := db.conn.Exec("UPDATE dramas SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) ListZhezisByDrama(dramaID string) ([]models.Zhezi, error) {
	rows, err := db.conn.Query(
		`SELECT id, drama_id, name, aliases, sort_order, remark FROM zhezis WHERE drama_id = ? ORDER BY sort_order ASC, created_at ASC`,
		dramaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Zhezi
	for rows.Next() {
		var z models.Zhezi
		var aliases string
		if err := rows.Scan(&z.ID, &z.DramaID, &z.Name, &aliases, &z.SortOrder, &z.Remark); err != nil {
			continue
		}
		z.Aliases = unmarshalStrings(aliases)
		out = append(out, z)
	}
	if out == nil {
		out = []models.Zhezi{}
	}
	return out, nil
}

func (db *DB) ListDramaTree() ([]models.DramaTree, error) {
	dramas, err := db.ListDramas()
	if err != nil {
		return nil, err
	}
	out := make([]models.DramaTree, 0, len(dramas))
	for _, d := range dramas {
		zs, err := db.ListZhezisByDrama(d.ID)
		if err != nil {
			continue
		}
		out = append(out, models.DramaTree{ID: d.ID, Name: d.Name, Aliases: d.Aliases, CategoryName: d.CategoryName, CategoryNames: d.CategoryNames, Zhezis: zs})
	}
	if out == nil {
		out = []models.DramaTree{}
	}
	return out, nil
}

func (db *DB) GetDramaDetail(id string) (*models.DramaDetail, error) {
	d, err := db.GetDrama(id)
	if err != nil {
		return nil, err
	}
	zhezis, err := db.ListZhezisByDrama(id)
	if err != nil {
		return nil, err
	}
	records, err := db.ListRecords(RecordFilter{DramaID: id})
	if err != nil {
		return nil, err
	}
	return &models.DramaDetail{Drama: *d, Zhezis: zhezis, Records: records}, nil
}

func (db *DB) SaveDrama(d models.Drama) (*models.Drama, error) {
	create := d.ID == ""
	normalizeDramaCategories(&d)
	if create {
		d.ID = newID()
		// New dramas append after any manually ordered ones.
		db.conn.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM dramas").Scan(&d.SortOrder)
	}
	_, err := db.conn.Exec(`
		INSERT INTO dramas (id, name, aliases, category_name, category_names, remark, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, aliases=excluded.aliases, category_name=excluded.category_name,
			category_names=excluded.category_names, remark=excluded.remark`,
		d.ID, d.Name, marshalJSON(d.Aliases), d.CategoryName, marshalJSON(d.CategoryNames), d.Remark, d.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("save drama: %w", err)
	}
	return db.GetDrama(d.ID)
}

func (db *DB) DeleteDrama(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM zhezis WHERE drama_id = ?", id); err != nil {
		return err
	}
	// Cascade: drop drama links so record_dramas has no orphan rows.
	if _, err := tx.Exec("DELETE FROM record_dramas WHERE drama_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM dramas WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- Artists (演员) ----------

// ListArtists returns all actors ordered by manual sort order then name.
func (db *DB) ListArtists() ([]models.Artist, error) {
	rows, err := db.conn.Query(`
		SELECT a.id, a.name, a.aliases, a.remark, a.cover, a.cover_file, a.cover_thumb, a.bio, a.sort_order,
			(SELECT COUNT(*) FROM record_artists ra JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0 WHERE ra.artist_id = a.id) AS record_count
		FROM artists a ORDER BY a.sort_order ASC, a.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Artist
	for rows.Next() {
		var a models.Artist
		var aliases string
		if err := rows.Scan(&a.ID, &a.Name, &aliases, &a.Remark, &a.Cover, &a.CoverFile, &a.CoverThumb, &a.Bio, &a.SortOrder, &a.RecordCount); err != nil {
			continue
		}
		a.Aliases = unmarshalStrings(aliases)
		out = append(out, a)
	}
	if out == nil {
		out = []models.Artist{}
	}
	return out, nil
}

// GetArtist returns a single actor by id.
func (db *DB) GetArtist(id string) (*models.Artist, error) {
	var a models.Artist
	var aliases string
	err := db.conn.QueryRow(`
		SELECT a.id, a.name, a.aliases, a.remark, a.cover, a.cover_file, a.cover_thumb, a.bio, a.sort_order,
			(SELECT COUNT(*) FROM record_artists ra JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0 WHERE ra.artist_id = a.id)
		FROM artists a WHERE a.id = ?`, id).
		Scan(&a.ID, &a.Name, &aliases, &a.Remark, &a.Cover, &a.CoverFile, &a.CoverThumb, &a.Bio, &a.SortOrder, &a.RecordCount)
	if err != nil {
		return nil, fmt.Errorf("artist not found: %w", err)
	}
	a.Aliases = unmarshalStrings(aliases)
	return &a, nil
}

// ReorderArtists sets the manual sort order of artists from an explicit ordered
// id list (first = top). Artists not in the list keep their previous order.
func (db *DB) ReorderArtists(orderedIDs []string) error {
	for i, id := range orderedIDs {
		if _, err := db.conn.Exec("UPDATE artists SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return nil
}

// ListArtistTree returns lightweight id+name pairs for pickers.
func (db *DB) ListArtistTree() ([]models.ArtistTree, error) {
	rows, err := db.conn.Query("SELECT id, name FROM artists ORDER BY sort_order ASC, name COLLATE NOCASE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ArtistTree
	for rows.Next() {
		var a models.ArtistTree
		if err := rows.Scan(&a.ID, &a.Name); err != nil {
			continue
		}
		out = append(out, a)
	}
	if out == nil {
		out = []models.ArtistTree{}
	}
	return out, nil
}

// GetArtistDetail returns the actor plus the performances that feature them.
func (db *DB) GetArtistDetail(id string) (*models.ArtistDetail, error) {
	a, err := db.GetArtist(id)
	if err != nil {
		return nil, err
	}
	records, err := db.ListRecords(RecordFilter{ArtistID: id})
	if err != nil {
		return nil, err
	}
	return &models.ArtistDetail{Artist: *a, Records: records}, nil
}

// SaveArtist inserts or updates an actor. New actors append after any manually
// ordered ones.
func (db *DB) SaveArtist(a models.Artist) (*models.Artist, error) {
	create := a.ID == ""
	if create {
		a.ID = newID()
		db.conn.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM artists").Scan(&a.SortOrder)
	}
	if a.Aliases == nil {
		a.Aliases = []string{}
	}
	_, err := db.conn.Exec(`
		INSERT INTO artists (id, name, aliases, remark, cover, cover_file, cover_thumb, bio, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, aliases=excluded.aliases, remark=excluded.remark,
			cover=excluded.cover, cover_file=excluded.cover_file, cover_thumb=excluded.cover_thumb,
			bio=excluded.bio`,
		a.ID, a.Name, marshalJSON(a.Aliases), a.Remark, a.Cover, a.CoverFile, a.CoverThumb, a.Bio, a.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("save artist: %w", err)
	}
	return db.GetArtist(a.ID)
}

// DeleteArtist removes an actor and cascades the record_artists links so the
// relation table has no orphan rows.
func (db *DB) DeleteArtist(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM record_artists WHERE artist_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM artists WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) CreateZhezi(z models.Zhezi) (*models.Zhezi, error) {
	z.ID = newID()
	if z.Aliases == nil {
		z.Aliases = []string{}
	}
	next := 0
	db.conn.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM zhezis WHERE drama_id = ?", z.DramaID).Scan(&next)
	z.SortOrder = next
	_, err := db.conn.Exec(`
		INSERT INTO zhezis (id, drama_id, name, aliases, sort_order, remark) VALUES (?, ?, ?, ?, ?, ?)`,
		z.ID, z.DramaID, z.Name, marshalJSON(z.Aliases), z.SortOrder, z.Remark)
	if err != nil {
		return nil, fmt.Errorf("create zhezi: %w", err)
	}
	return db.zheziByID(z.ID)
}

func (db *DB) zheziByID(id string) (*models.Zhezi, error) {
	var z models.Zhezi
	var aliases string
	err := db.conn.QueryRow(
		`SELECT id, drama_id, name, aliases, sort_order, remark FROM zhezis WHERE id = ?`, id).
		Scan(&z.ID, &z.DramaID, &z.Name, &aliases, &z.SortOrder, &z.Remark)
	if err != nil {
		return nil, fmt.Errorf("zhezi not found: %w", err)
	}
	z.Aliases = unmarshalStrings(aliases)
	return &z, nil
}

// GetZhezi returns a single zhezi by id (exported for MCP tooling).
func (db *DB) GetZhezi(id string) (*models.Zhezi, error) {
	return db.zheziByID(id)
}

// GetZheziNames resolves a batch of 折子 ids to their display names in a single
// query. Missing ids are simply absent from the returned map. Used by the ICS
// exporter to render 折子 names without per-record lookups.
func (db *DB) GetZheziNames(ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := db.conn.Query(
		"SELECT id, name FROM zhezis WHERE id IN ("+strings.Join(ph, ",")+")", args...)
	if err != nil {
		return nil, fmt.Errorf("get zhezi names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan zhezi name: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (db *DB) UpdateZhezi(z models.Zhezi) (*models.Zhezi, error) {
	if z.Aliases == nil {
		z.Aliases = []string{}
	}
	_, err := db.conn.Exec(`
		UPDATE zhezis SET name = ?, aliases = ?, remark = ? WHERE id = ?`,
		z.Name, marshalJSON(z.Aliases), z.Remark, z.ID)
	if err != nil {
		return nil, fmt.Errorf("update zhezi: %w", err)
	}
	return db.zheziByID(z.ID)
}

func (db *DB) DeleteZhezi(id string) error {
	// Cascade: drop any record<->zhezi links so the relation table stays clean.
	if _, err := db.conn.Exec("DELETE FROM record_zhezis WHERE zhezi_id = ?", id); err != nil {
		return err
	}
	_, err := db.conn.Exec("DELETE FROM zhezis WHERE id = ?", id)
	return err
}

// ReorderZhezis sets the manual order of a drama's zhezis from an explicit
// ordered id list (first = top).
func (db *DB) ReorderZhezis(dramaID string, orderedIDs []string) error {
	for i, id := range orderedIDs {
		if _, err := db.conn.Exec("UPDATE zhezis SET sort_order = ? WHERE id = ? AND drama_id = ?", i, id, dramaID); err != nil {
			return err
		}
	}
	return nil
}

// dramaIDByName returns the id of a drama with the exact name, or "" if none.
func (db *DB) dramaIDByName(name string) string {
	var id string
	if err := db.conn.QueryRow("SELECT id FROM dramas WHERE name = ? LIMIT 1", name).Scan(&id); err != nil {
		return ""
	}
	return id
}

// recordDramaIDs returns the drama ids linked to a record, read from the
// record_dramas relation table (the source of truth). Falls back to the legacy
// records.drama_ids column when the relation table has no rows yet (e.g. mid
// migration of an old database).
func (db *DB) recordDramaIDs(id string) ([]string, error) {
	rows, err := db.conn.Query("SELECT drama_id FROM record_dramas WHERE record_id = ? ORDER BY sort_order", id)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err == nil && d != "" {
			ids = append(ids, d)
		}
	}
	rows.Close()
	if len(ids) > 0 {
		return ids, nil
	}
	// Legacy fallback: try the old JSON text column.
	var s string
	if err := db.conn.QueryRow("SELECT drama_ids FROM records WHERE id = ?", id).Scan(&s); err != nil {
		return []string{}, nil
	}
	return unmarshalStrings(s), nil
}

// BackfillDramasFromRecords derives 剧目 archives from the 剧名 (play) already
// held in records — the two are equivalent — and links each record to the
// matching drama. Idempotent: only creates dramas for names not yet present
// and never drops existing links, so it is safe to run on every startup.
func (db *DB) BackfillDramasFromRecords() error {
	rows, err := db.conn.Query("SELECT id, play FROM records")
	if err != nil {
		return fmt.Errorf("backfill: query records: %w", err)
	}
	type rowRec struct {
		id    string
		plays []string
	}
	var recs []rowRec
	seen := map[string]bool{}
	for rows.Next() {
		var id, play string
		if err := rows.Scan(&id, &play); err != nil {
			rows.Close()
			return err
		}
		var plays []string
		for _, p := range unmarshalStrings(play) {
			p = strings.TrimSpace(p)
			if p != "" {
				plays = append(plays, p)
				seen[p] = true
			}
		}
		recs = append(recs, rowRec{id: id, plays: plays})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// Ensure a drama exists for every distinct 剧名 (name -> id).
	dramaIDByPlay := map[string]string{}
	for name := range seen {
		if id := db.dramaIDByName(name); id != "" {
			dramaIDByPlay[name] = id
			continue
		}
		id := newID()
		if _, err := db.conn.Exec("INSERT INTO dramas (id, name) VALUES (?, ?)", id, name); err != nil {
			return err
		}
		dramaIDByPlay[name] = id
	}

	// Link each record's plays to the corresponding drama ids, written into
	// the record_dramas relation table (the source of truth).
	for _, r := range recs {
		if len(r.plays) == 0 {
			continue
		}
		current, err := db.recordDramaIDs(r.id)
		if err != nil {
			return err
		}
		set := map[string]bool{}
		var merged []string
		push := func(id string) {
			if id == "" || set[id] {
				return
			}
			set[id] = true
			merged = append(merged, id)
		}
		for _, id := range current {
			push(id)
		}
		for _, p := range r.plays {
			push(dramaIDByPlay[p])
		}
		if err := db.setRecordDramas(db.conn, r.id, merged); err != nil {
			return err
		}
	}
	return nil
}

// ---------- Meta ----------

func (db *DB) GetMeta() (*models.Meta, error) {
	m := &models.Meta{}
	get := func(key string) string {
		var v string
		db.conn.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&v)
		if v == "" {
			return "[]"
		}
		return v
	}
	m.Song = json.RawMessage(get("song"))
	m.Tags = json.RawMessage(get("tags"))
	m.WebdavConfig = json.RawMessage(get("webdav_config"))
	return m, nil
}

func (db *DB) SetMeta(m *models.Meta) error {
	return setMetaExec(db.conn, m)
}

// SetMetaTx runs inside the supplied transaction (used by ImportData).
func (db *DB) SetMetaTx(tx *sql.Tx, m *models.Meta) error {
	return setMetaExec(tx, m)
}

func setMetaExec(exec sqlExecutor, m *models.Meta) error {
	upsert := func(key string, raw json.RawMessage) error {
		val := "[]"
		if len(raw) > 0 {
			val = string(raw)
		}
		_, err := exec.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, val)
		return err
	}
	if err := upsert("song", m.Song); err != nil {
		return err
	}
	if err := upsert("tags", m.Tags); err != nil {
		return err
	}
	return upsert("webdav_config", m.WebdavConfig)
}

// ---------- Stats / Dashboard ----------

func (db *DB) GetStats() (*models.Stats, error) {
	s := &models.Stats{}
	db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at = 0").Scan(&s.TotalRecords)
	db.conn.QueryRow("SELECT COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0) FROM records WHERE deleted_at = 0").Scan(&s.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE deleted_at = 0 AND rating IS NOT NULL AND rating != 0").Scan(&s.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT city) FROM records WHERE deleted_at = 0 AND city != ''").Scan(&s.TotalCities)
	return s, nil
}

func (db *DB) GetDashboardStats() (*models.DashboardStats, error) {
	s := &models.DashboardStats{}
	// Initialize slices so empty results marshal as [] instead of null.
	s.ByMonth = []models.MonthStat{}
	s.ByCategory = []models.CategoryStat{}
	s.ByCity = []models.CityStat{}
	s.CostByMonth = []models.CostStat{}
	s.TopRated = []models.Record{}
	s.RecentRecords = []models.Record{}

	db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at = 0").Scan(&s.TotalRecords)
	db.conn.QueryRow("SELECT COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0) FROM records WHERE deleted_at = 0").Scan(&s.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE deleted_at = 0 AND rating IS NOT NULL AND rating != 0").Scan(&s.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT city) FROM records WHERE deleted_at = 0 AND city != ''").Scan(&s.TotalCities)

	rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(date, 'unixepoch')) as month, COUNT(*) as cnt
		FROM records
		WHERE deleted_at = 0 AND date >= strftime('%s', 'now', '-12 months')
		GROUP BY month ORDER BY month`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m models.MonthStat
			rows.Scan(&m.Month, &m.Count)
			s.ByMonth = append(s.ByMonth, m)
		}
	}

	// Multi-category: expand the category_names array so a record counts
	// once for every category it involves.
	rows2, err := db.conn.Query(`
		SELECT je.value AS category_name, COUNT(*) as cnt FROM records,
			json_each(records.category_names) je
		WHERE records.deleted_at = 0 AND je.value != '' GROUP BY je.value ORDER BY cnt DESC`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var c models.CategoryStat
			rows2.Scan(&c.Name, &c.Count)
			s.ByCategory = append(s.ByCategory, c)
		}
	}

	rows3, err := db.conn.Query(`
		SELECT city, COUNT(*) as cnt FROM records
		WHERE deleted_at = 0 AND city != '' GROUP BY city ORDER BY cnt DESC LIMIT 10`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var c models.CityStat
			rows3.Scan(&c.Name, &c.Count)
			s.ByCity = append(s.ByCity, c)
		}
	}

	rows4, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(date, 'unixepoch')) as month,
		       SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)) as cost
		FROM records
		WHERE deleted_at = 0 AND date >= strftime('%s', 'now', '-12 months')
		GROUP BY month ORDER BY month`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var c models.CostStat
			rows4.Scan(&c.Month, &c.Cost)
			s.CostByMonth = append(s.CostByMonth, c)
		}
	}

	topRows, err := db.conn.Query(`SELECT ` + recordColumns + ` FROM records WHERE deleted_at = 0 AND rating IS NOT NULL AND rating != 0 ORDER BY rating DESC, date DESC LIMIT 6`)
	if err == nil {
		defer topRows.Close()
		for topRows.Next() {
			if r, err := scanRecord(topRows); err == nil {
				s.TopRated = append(s.TopRated, *r)
			}
		}
	}

	recentRows, err := db.conn.Query(`SELECT ` + recordColumns + ` FROM records WHERE deleted_at = 0 ORDER BY date DESC LIMIT 6`)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			if r, err := scanRecord(recentRows); err == nil {
				s.RecentRecords = append(s.RecentRecords, *r)
			}
		}
	}
	if err := db.backfillDramaIDs(context.Background(), s.TopRated); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(context.Background(), s.TopRated); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	if err := db.backfillArtistIDs(context.Background(), s.TopRated); err != nil {
		slog.Warn("backfill artist ids", "err", err)
	}
	if err := db.backfillDramaIDs(context.Background(), s.RecentRecords); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(context.Background(), s.RecentRecords); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	if err := db.backfillArtistIDs(context.Background(), s.RecentRecords); err != nil {
		slog.Warn("backfill artist ids", "err", err)
	}

	return s, nil
}

// ListMapPoints returns the slim projection for the map page: only records
// with coordinates and only the fields the map renders. Returns an empty
// (non-nil) slice when there are no geocoded records.
func (db *DB) ListMapPoints() ([]models.MapPoint, error) {
	rows, err := db.conn.Query(`
		SELECT id, name, city, address, coordinate, cover_file, cover_thumb,
		       date_text, rating, active_status, category_name
		FROM records
		WHERE deleted_at = 0 AND coordinate != '' AND coordinate != 'null'
		ORDER BY date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.MapPoint{}
	for rows.Next() {
		var p models.MapPoint
		var coord string
		if err := rows.Scan(&p.ID, &p.Name, &p.City, &p.Address, &coord, &p.CoverFile,
			&p.CoverThumb, &p.DateText, &p.Rating, &p.ActiveStatus, &p.CategoryName); err != nil {
			slog.Warn("scan map point", "err", err)
			continue
		}
		p.Coordinate = unmarshalCoordinate(coord)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- Calendar ----------

func (db *DB) GetCalendarEvents(year, month int) ([]models.CalendarEvent, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, db.loc)
	end := start.AddDate(0, 1, 0)
	rows, err := db.conn.Query(`
		SELECT id, name, date, city, address, cover_file, cover_thumb, rating, active_status, category_name
		FROM records WHERE deleted_at = 0 AND date >= ? AND date < ? ORDER BY date ASC
	`, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CalendarEvent
	for rows.Next() {
		var e models.CalendarEvent
		if err := rows.Scan(&e.ID, &e.Name, &e.Date, &e.City, &e.Address, &e.CoverFile, &e.CoverThumb, &e.Rating, &e.ActiveStatus, &e.CategoryName); err != nil {
			slog.Warn("scan calendar event", "err", err)
			continue
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []models.CalendarEvent{}
	}
	return out, nil
}

// ---------- Autocomplete / field filter ----------

var textFields = map[string]bool{
	"name": true, "city": true, "address": true, "company": true,
	"channel": true, "friends": true, "category_name": true, "seat": true,
}

func (db *DB) GetAutocomplete(field string) ([]string, error) {
	if !textFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}
	rows, err := db.conn.Query("SELECT DISTINCT " + field + " FROM records WHERE deleted_at = 0 AND " + field + " != '' ORDER BY " + field)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			continue
		}
		parts := []string{v}
		if field == "company" || field == "friends" {
			parts = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '，' })
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	if field == "company" || field == "friends" {
		slices.Sort(out)
	}
	return out, nil
}

func (db *DB) GetByField(field, value string) ([]models.Record, error) {
	if !textFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}
	rows, err := db.conn.Query(`SELECT `+recordColumns+` FROM records WHERE deleted_at = 0 AND `+field+` LIKE ? ORDER BY date DESC`, "%"+value+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Record
	for rows.Next() {
		if r, err := scanRecord(rows); err == nil {
			out = append(out, *r)
		}
	}
	if out == nil {
		out = []models.Record{}
	}
	if err := db.backfillDramaIDs(context.Background(), out); err != nil {
		slog.Warn("backfill by-field drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(context.Background(), out); err != nil {
		slog.Warn("backfill by-field zhezi ids", "err", err)
	}
	if err := db.backfillArtistIDs(context.Background(), out); err != nil {
		slog.Warn("backfill by-field artist ids", "err", err)
	}
	return out, nil
}
