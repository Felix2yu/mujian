package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
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
	//  - busy_timeout(5000): wait instead of immediately erroring on lock
	//  - foreign_keys(0): the schema uses JSON-in-TEXT links, not FK constraints
	//  - synchronous(NORMAL): WAL already protects against corruption; avoids a
	//    fsync per transaction while keeping crash safety (recommended for WAL)
	//  - mmap_size / cache_size: keep more of the db in memory for read speed
	conn, err := sql.Open("sqlite", dbPath+"?"+
		"_pragma=journal_mode(WAL)"+
		"&_pragma=busy_timeout(5000)"+
		"&_pragma=foreign_keys(0)"+
		"&_pragma=synchronous(NORMAL)"+
		"&_pragma=cache_size(-8000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite is a single-writer database. busy_timeout (set in the DSN above)
	// makes concurrent writers wait instead of erroring with "database is
	// locked". We deliberately do NOT pin MaxOpenConns(1): with modernc.org/sqlite
	// that can deadlock when successive calls briefly contend on the single
	// pooled connection. The pool default is safe for this low-concurrency app.

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
			other_cost_currency TEXT NOT NULL DEFAULT 'CNY'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_records_date ON records(date)`,
		`CREATE INDEX IF NOT EXISTS idx_records_category ON records(category_name)`,
		`CREATE INDEX IF NOT EXISTS idx_records_city ON records(city)`,
		`CREATE INDEX IF NOT EXISTS idx_records_cover_file ON records(cover_file)`,
		`CREATE INDEX IF NOT EXISTS idx_records_active_status ON records(active_status)`,
		`CREATE INDEX IF NOT EXISTS idx_records_name ON records(name)`,
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
			category_name TEXT NOT NULL DEFAULT '',
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
	// Manual ordering for dramas/categories (0 = alphabetical).
	if err := db.addColumn("dramas", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumn("categories", "sort_order", "INTEGER NOT NULL DEFAULT 0"); err != nil {
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
// is intentionally NOT included: drama links live in the record_dramas relation
// table and are backfilled into models.Record.DramaIDs via backfillDramaIDs
// after the query. (recordUpsertSQL still writes the legacy drama_ids column for
// backward-compatible backups; the relation table is the source of truth.)
const recordColumns = `id, name, channel, city, address, coordinate, cover, cover_file,
	cover_thumb, custom_category_id, category_name, artist_names, guest, play, zhezi_ids, tag_ids,
	date, date_text, rating, seat, friends, company, remark, active_status,
	price, price_currency, pay_price, pay_price_currency, other_cost, other_cost_currency`

func scanRecord(rows *sql.Rows) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, artistNames, guest, play, zheziIDs, tagIDs string
	)
	err := rows.Scan(
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &artistNames, &guest, &play, &zheziIDs, &tagIDs,
		&r.Date, &r.DateText, &r.Rating, &r.Seat, &r.Friends, &r.Company, &r.Remark, &r.ActiveStatus,
		&r.Price, &r.PriceCurrency, &r.PayPrice, &r.PayPriceCurrency, &r.OtherCost, &r.OtherCostCurrency,
	)
	if err != nil {
		return nil, err
	}
	r.Coordinate = unmarshalCoordinate(coordinate)
	r.ArtistNames = unmarshalStrings(artistNames)
	r.Guest = unmarshalStrings(guest)
	r.Play = unmarshalStrings(play)
	r.ZheziIDs = unmarshalStrings(zheziIDs)
	r.TagIDs = unmarshalStrings(tagIDs)
	return &r, nil
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
}

func (db *DB) ListRecords(f RecordFilter) ([]models.Record, error) {
	query := `SELECT ` + recordColumns + ` FROM records`
	where := []string{}
	args := []interface{}{}

	if f.Query != "" {
		like := "%" + f.Query + "%"
		where = append(where, `(name LIKE ? OR city LIKE ? OR address LIKE ? OR company LIKE ? OR channel LIKE ? OR remark LIKE ? OR friends LIKE ? OR category_name LIKE ? OR artist_names LIKE ? OR play LIKE ?)`)
		for range 10 {
			args = append(args, like)
		}
	}
	if f.Category != "" {
		where = append(where, "category_name = ?")
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

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY date DESC"

	rows, err := db.conn.Query(query, args...)
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
	if err := db.backfillDramaIDs(out); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(out); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	return out, nil
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
	row := db.conn.QueryRow(`SELECT `+recordColumns+` FROM records WHERE id = ?`, id)
	r, err := scanRecordRow(row)
	if err != nil {
		return nil, fmt.Errorf("record not found: %w", err)
	}
	rs := []models.Record{*r}
	if err := db.backfillDramaIDs(rs); err != nil {
		slog.Warn("backfill drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(rs); err != nil {
		slog.Warn("backfill zhezi ids", "err", err)
	}
	return &rs[0], nil
}

func scanRecordRow(row *sql.Row) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, artistNames, guest, play, zheziIDs, tagIDs string
	)
	err := row.Scan(
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &artistNames, &guest, &play, &zheziIDs, &tagIDs,
		&r.Date, &r.DateText, &r.Rating, &r.Seat, &r.Friends, &r.Company, &r.Remark, &r.ActiveStatus,
		&r.Price, &r.PriceCurrency, &r.PayPrice, &r.PayPriceCurrency, &r.OtherCost, &r.OtherCostCurrency,
	)
	if err != nil {
		return nil, err
	}
	r.Coordinate = unmarshalCoordinate(coordinate)
	r.ArtistNames = unmarshalStrings(artistNames)
	r.Guest = unmarshalStrings(guest)
	r.Play = unmarshalStrings(play)
	r.ZheziIDs = unmarshalStrings(zheziIDs)
	r.TagIDs = unmarshalStrings(tagIDs)
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
		custom_category_id, category_name, artist_names, guest, play, drama_ids, zhezi_ids, tag_ids,
		date, date_text, rating, seat, friends, company, remark, active_status,
		price, price_currency, pay_price, pay_price_currency, other_cost, other_cost_currency
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		name=excluded.name, channel=excluded.channel, city=excluded.city, address=excluded.address,
		coordinate=excluded.coordinate, cover=excluded.cover, cover_file=excluded.cover_file, cover_thumb=excluded.cover_thumb,
		custom_category_id=excluded.custom_category_id, category_name=excluded.category_name,
		artist_names=excluded.artist_names, guest=excluded.guest, play=excluded.play,
		drama_ids=excluded.drama_ids, zhezi_ids=excluded.zhezi_ids, tag_ids=excluded.tag_ids,
		date=excluded.date, date_text=excluded.date_text, rating=excluded.rating, seat=excluded.seat,
		friends=excluded.friends, company=excluded.company, remark=excluded.remark, active_status=excluded.active_status,
		price=excluded.price, price_currency=excluded.price_currency, pay_price=excluded.pay_price,
		pay_price_currency=excluded.pay_price_currency, other_cost=excluded.other_cost, other_cost_currency=excluded.other_cost_currency
`

// UpsertRecord inserts or updates a single record. For bulk imports prefer
// BulkUpsertRecords, which wraps all rows in one transaction (far fewer fsyncs).
func (db *DB) UpsertRecord(r models.Record) error {
	if r.ID == "" {
		r.ID = newID()
	}
	if _, err := db.stmtUpsertRecord.Exec(
		r.ID, r.Name, r.Channel, r.City, r.Address, marshalJSON(r.Coordinate), r.Cover, r.CoverFile, r.CoverThumb,
		r.CustomCategoryID, r.CategoryName, marshalJSON(r.ArtistNames), marshalJSON(r.Guest), marshalJSON(r.Play),
		marshalJSON(r.DramaIDs), marshalJSON(r.ZheziIDs), marshalJSON(r.TagIDs),
		r.Date, r.DateText, r.Rating, r.Seat, r.Friends, r.Company, r.Remark, r.ActiveStatus,
		r.Price, r.PriceCurrency, r.PayPrice, r.PayPriceCurrency, r.OtherCost, r.OtherCostCurrency,
	); err != nil {
		return err
	}
	// Keep the drama/zhezi relation tables in sync with the upserted record.
	if err := db.setRecordDramas(db.conn, r.ID, r.DramaIDs); err != nil {
		return err
	}
	return db.setRecordZhezis(db.conn, r.ID, r.ZheziIDs)
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
	if _, err := tx.Exec(recordUpsertSQL,
		r.ID, r.Name, r.Channel, r.City, r.Address, marshalJSON(r.Coordinate), r.Cover, r.CoverFile, r.CoverThumb,
		r.CustomCategoryID, r.CategoryName, marshalJSON(r.ArtistNames), marshalJSON(r.Guest), marshalJSON(r.Play),
		marshalJSON(r.DramaIDs), marshalJSON(r.ZheziIDs), marshalJSON(r.TagIDs),
		r.Date, r.DateText, r.Rating, r.Seat, r.Friends, r.Company, r.Remark, r.ActiveStatus,
		r.Price, r.PriceCurrency, r.PayPrice, r.PayPriceCurrency, r.OtherCost, r.OtherCostCurrency,
	); err != nil {
		return err
	}
	// Keep the drama/zhezi relation tables in sync within the same transaction.
	if err := db.setRecordDramas(tx, r.ID, r.DramaIDs); err != nil {
		return err
	}
	return db.setRecordZhezis(tx, r.ID, r.ZheziIDs)
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

// backfillDramaIDs loads drama ids for the given records from the relation
// table in a single batched query and fills models.Record.DramaIDs.
func (db *DB) backfillDramaIDs(records []models.Record) error {
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
	rows, err := db.conn.Query(
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
func (db *DB) backfillZheziIDs(records []models.Record) error {
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
	rows, err := db.conn.Query(
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
	rec.CoverFile = existing.CoverFile
	rec.CoverThumb = existing.CoverThumb
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
		"UPDATE records SET coordinate = ? WHERE address = ? AND id != ?",
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
	rows, err := db.conn.Query("SELECT DISTINCT address FROM records WHERE address != ''")
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
		gr, err := db.conn.Query("SELECT coordinate FROM records WHERE address = ?", addr)
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
			"UPDATE records SET coordinate = ? WHERE address = ? AND coordinate != ?",
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
		CustomCategoryID: r.CustomCategoryID, CategoryName: r.CategoryName,
		ArtistNames: a, Guest: g, Play: p, DramaIDs: d, ZheziIDs: z, TagIDs: t,
		Date: r.Date, DateText: r.DateText, Rating: r.Rating, Seat: r.Seat,
		Friends: r.Friends, Company: r.Company, Remark: r.Remark, ActiveStatus: r.ActiveStatus,
		Price: r.Price, PriceCurrency: r.PriceCurrency, PayPrice: r.PayPrice,
		PayPriceCurrency: r.PayPriceCurrency, OtherCost: r.OtherCost, OtherCostCurrency: r.OtherCostCurrency,
	}
}

func (db *DB) DeleteRecord(id string) error {
	_, err := db.conn.Exec("DELETE FROM records WHERE id = ?", id)
	return err
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
		params.ArtistNames != nil || params.TagIDs != nil

	// 2. Apply simple scalar updates (one SQL for all)
	if len(simpleSets) > 0 {
		placeholders := make([]string, len(params.IDs))
		inArgs := make([]interface{}, len(params.IDs))
		for i, id := range params.IDs {
			placeholders[i] = "?"
			inArgs[i] = id
		}
		sql := "UPDATE records SET " + strings.Join(simpleSets, ", ") + " WHERE id IN (" + strings.Join(placeholders, ",") + ")"
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
	if params.TagIDs != nil {
		arrayCols["tag_ids"] = params.TagIDs
	}

	var total int64
	for _, id := range params.IDs {
		colUpdates := []string{}
		colArgs := []interface{}{}

		for col, op := range arrayCols {
			var existing []string
			row := db.conn.QueryRow("SELECT "+col+" FROM records WHERE id = ?", id)
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
		}

		if len(colUpdates) > 0 {
			colArgs = append(colArgs, id)
			sql := "UPDATE records SET " + strings.Join(colUpdates, ", ") + " WHERE id = ?"
			if _, err := db.conn.Exec(sql, colArgs...); err != nil {
				return total, err
			}
			total++
		}
	}
	return total, nil
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
		set := make(map[string]struct{}, len(existing)+len(op.Value))
		for _, v := range existing {
			set[v] = struct{}{}
		}
		for _, v := range op.Value {
			set[v] = struct{}{}
		}
		return slices.Collect(maps.Keys(set))
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
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := "(" + strings.Join(placeholders, ",") + ")"
	res, err := db.conn.Exec("DELETE FROM records WHERE id IN "+inClause, args...)
	if err != nil {
		return 0, err
	}
	// Cascade: drop drama/zhezi links for the deleted records.
	if _, err := db.conn.Exec("DELETE FROM record_dramas WHERE record_id IN "+inClause, args...); err != nil {
		slog.Warn("delete record drama links", "err", err)
	}
	if _, err := db.conn.Exec("DELETE FROM record_zhezis WHERE record_id IN "+inClause, args...); err != nil {
		slog.Warn("delete record zhezi links", "err", err)
	}
	return res.RowsAffected()
}

// ---------- Categories ----------

func (db *DB) ListCategories() ([]models.Category, error) {
	rows, err := db.conn.Query(`SELECT id, name, active_ids, record_count, sort_order FROM categories ORDER BY sort_order ASC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		var c models.Category
		var activeIDs string
		if err := rows.Scan(&c.ID, &c.Name, &activeIDs, &c.RecordCount, &c.SortOrder); err != nil {
			continue
		}
		c.ActiveIDs = unmarshalStrings(activeIDs)
		out = append(out, c)
	}
	if out == nil {
		out = []models.Category{}
	}
	return out, nil
}

// upsertCategoryExec inserts/updates a category against the given executor.
func upsertCategoryExec(exec sqlExecutor, c *models.Category) error {
	if c.ID == "" {
		c.ID = newID()
		// New categories append after any manually ordered ones.
		exec.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM categories").Scan(&c.SortOrder)
	}
	_, err := exec.Exec(`
		INSERT INTO categories (id, name, active_ids, record_count, sort_order) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, active_ids=excluded.active_ids, record_count=excluded.record_count
	`, c.ID, c.Name, marshalJSON(c.ActiveIDs), c.RecordCount, c.SortOrder)
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

func (db *DB) ListDramas() ([]models.Drama, error) {
	rows, err := db.conn.Query(`
		SELECT d.id, d.name, d.category_name, d.remark, d.sort_order,
			(SELECT COUNT(*) FROM zhezis z WHERE z.drama_id = d.id) AS zhezi_count,
			(SELECT COUNT(*) FROM record_dramas rd WHERE rd.drama_id = d.id) AS record_count
		FROM dramas d ORDER BY d.sort_order ASC, d.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Drama
	for rows.Next() {
		var d models.Drama
		if err := rows.Scan(&d.ID, &d.Name, &d.CategoryName, &d.Remark, &d.SortOrder, &d.ZheziCount, &d.RecordCount); err != nil {
			continue
		}
		out = append(out, d)
	}
	if out == nil {
		out = []models.Drama{}
	}
	return out, nil
}

func (db *DB) GetDrama(id string) (*models.Drama, error) {
	var d models.Drama
	err := db.conn.QueryRow(`
		SELECT d.id, d.name, d.category_name, d.remark, d.sort_order,
			(SELECT COUNT(*) FROM zhezis z WHERE z.drama_id = d.id),
			(SELECT COUNT(*) FROM record_dramas rd WHERE rd.drama_id = d.id)
		FROM dramas d WHERE d.id = ?`, id).
		Scan(&d.ID, &d.Name, &d.CategoryName, &d.Remark, &d.SortOrder, &d.ZheziCount, &d.RecordCount)
	if err != nil {
		return nil, fmt.Errorf("drama not found: %w", err)
	}
	return &d, nil
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
		out = append(out, models.DramaTree{ID: d.ID, Name: d.Name, CategoryName: d.CategoryName, Zhezis: zs})
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
	if create {
		d.ID = newID()
		// New dramas append after any manually ordered ones.
		db.conn.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM dramas").Scan(&d.SortOrder)
	}
	_, err := db.conn.Exec(`
		INSERT INTO dramas (id, name, category_name, remark, sort_order) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, category_name=excluded.category_name, remark=excluded.remark`,
		d.ID, d.Name, d.CategoryName, d.Remark, d.SortOrder)
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
	db.conn.QueryRow("SELECT COUNT(*) FROM records").Scan(&s.TotalRecords)
	db.conn.QueryRow("SELECT COALESCE(SUM(COALESCE(pay_price,0) + COALESCE(other_cost,0)), 0) FROM records").Scan(&s.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE rating IS NOT NULL AND rating != 0").Scan(&s.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT city) FROM records WHERE city != ''").Scan(&s.TotalCities)
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

	db.conn.QueryRow("SELECT COUNT(*) FROM records").Scan(&s.TotalRecords)
	db.conn.QueryRow("SELECT COALESCE(SUM(COALESCE(pay_price,0) + COALESCE(other_cost,0)), 0) FROM records").Scan(&s.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE rating IS NOT NULL AND rating != 0").Scan(&s.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT city) FROM records WHERE city != ''").Scan(&s.TotalCities)

	rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(date, 'unixepoch')) as month, COUNT(*) as cnt
		FROM records
		WHERE date >= strftime('%s', 'now', '-12 months')
		GROUP BY month ORDER BY month`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m models.MonthStat
			rows.Scan(&m.Month, &m.Count)
			s.ByMonth = append(s.ByMonth, m)
		}
	}

	rows2, err := db.conn.Query(`
		SELECT category_name, COUNT(*) as cnt FROM records
		WHERE category_name != '' GROUP BY category_name ORDER BY cnt DESC`)
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
		WHERE city != '' GROUP BY city ORDER BY cnt DESC LIMIT 10`)
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
		       SUM(COALESCE(pay_price,0) + COALESCE(other_cost,0)) as cost
		FROM records
		WHERE date >= strftime('%s', 'now', '-12 months')
		GROUP BY month ORDER BY month`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var c models.CostStat
			rows4.Scan(&c.Month, &c.Cost)
			s.CostByMonth = append(s.CostByMonth, c)
		}
	}

	topRows, err := db.conn.Query(`SELECT ` + recordColumns + ` FROM records WHERE rating IS NOT NULL AND rating != 0 ORDER BY rating DESC, date DESC LIMIT 6`)
	if err == nil {
		defer topRows.Close()
		for topRows.Next() {
			if r, err := scanRecord(topRows); err == nil {
				s.TopRated = append(s.TopRated, *r)
			}
		}
	}

	recentRows, err := db.conn.Query(`SELECT ` + recordColumns + ` FROM records ORDER BY date DESC LIMIT 6`)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			if r, err := scanRecord(recentRows); err == nil {
				s.RecentRecords = append(s.RecentRecords, *r)
			}
		}
	}
	if err := db.backfillDramaIDs(s.TopRated); err != nil {
		slog.Warn("backfill top rated drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(s.TopRated); err != nil {
		slog.Warn("backfill top rated zhezi ids", "err", err)
	}
	if err := db.backfillDramaIDs(s.RecentRecords); err != nil {
		slog.Warn("backfill recent drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(s.RecentRecords); err != nil {
		slog.Warn("backfill recent zhezi ids", "err", err)
	}

	return s, nil
}

// ---------- Calendar ----------

func (db *DB) GetCalendarEvents(year, month int) ([]models.CalendarEvent, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, db.loc)
	end := start.AddDate(0, 1, 0)
	rows, err := db.conn.Query(`
		SELECT id, name, date, city, address, cover_file, rating, category_name
		FROM records WHERE date >= ? AND date < ? ORDER BY date ASC
	`, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CalendarEvent
	for rows.Next() {
		var e models.CalendarEvent
		if err := rows.Scan(&e.ID, &e.Name, &e.Date, &e.City, &e.Address, &e.CoverFile, &e.Rating, &e.CategoryName); err != nil {
			continue
		}
		out = append(out, e)
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
	rows, err := db.conn.Query("SELECT DISTINCT " + field + " FROM records WHERE " + field + " != '' ORDER BY " + field)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (db *DB) GetByField(field, value string) ([]models.Record, error) {
	if !textFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}
	rows, err := db.conn.Query(`SELECT `+recordColumns+` FROM records WHERE `+field+` LIKE ? ORDER BY date DESC`, "%"+value+"%")
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
	if err := db.backfillDramaIDs(out); err != nil {
		slog.Warn("backfill by-field drama ids", "err", err)
	}
	if err := db.backfillZheziIDs(out); err != nil {
		slog.Warn("backfill by-field zhezi ids", "err", err)
	}
	return out, nil
}
