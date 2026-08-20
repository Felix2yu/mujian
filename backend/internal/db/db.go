package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"mujian/internal/models"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	loc  *time.Location
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(0)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	db := &DB{conn: conn, loc: time.UTC}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
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
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
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

const recordColumns = `id, name, channel, city, address, coordinate, cover, cover_file,
	cover_thumb, custom_category_id, category_name, artist_names, guest, play, tag_ids,
	date, date_text, rating, seat, friends, company, remark, active_status,
	price, price_currency, pay_price, pay_price_currency, other_cost, other_cost_currency`

func scanRecord(rows *sql.Rows) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, artistNames, guest, play, tagIDs string
	)
	err := rows.Scan(
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &artistNames, &guest, &play, &tagIDs,
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
	r.TagIDs = unmarshalStrings(tagIDs)
	// List contexts don't carry the base64 thumbnail (payload bloat); the
	// detail view (scanRecordRow) keeps it.
	r.CoverThumb = ""
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
}

func (db *DB) ListRecords(f RecordFilter) ([]models.Record, error) {
	query := `SELECT ` + recordColumns + ` FROM records`
	where := []string{}
	args := []interface{}{}

	if f.Query != "" {
		like := "%" + f.Query + "%"
		where = append(where, `(name LIKE ? OR city LIKE ? OR address LIKE ? OR company LIKE ? OR channel LIKE ? OR remark LIKE ? OR friends LIKE ? OR category_name LIKE ? OR artist_names LIKE ? OR play LIKE ?)`)
		for i := 0; i < 10; i++ {
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
			log.Printf("scan record: %v", err)
			continue
		}
		out = append(out, *r)
	}
	if out == nil {
		out = []models.Record{}
	}
	return out, nil
}

func parseTimeArg(s string, loc *time.Location) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if n, err := parseInt64(s); err == nil {
		return time.Unix(n, 0).In(loc), true
	}
	for _, f := range []string{"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(f, s, loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
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
	return r, nil
}

func scanRecordRow(row *sql.Row) (*models.Record, error) {
	var r models.Record
	var (
		coordinate, artistNames, guest, play, tagIDs string
	)
	err := row.Scan(
		&r.ID, &r.Name, &r.Channel, &r.City, &r.Address, &coordinate, &r.Cover, &r.CoverFile,
		&r.CoverThumb, &r.CustomCategoryID, &r.CategoryName, &artistNames, &guest, &play, &tagIDs,
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

func (db *DB) UpsertRecord(r models.Record) error {
	if r.ID == "" {
		r.ID = newID()
	}
	_, err := db.conn.Exec(`
		INSERT INTO records (
			id, name, channel, city, address, coordinate, cover, cover_file, cover_thumb,
			custom_category_id, category_name, artist_names, guest, play, tag_ids,
			date, date_text, rating, seat, friends, company, remark, active_status,
			price, price_currency, pay_price, pay_price_currency, other_cost, other_cost_currency
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, channel=excluded.channel, city=excluded.city, address=excluded.address,
			coordinate=excluded.coordinate, cover=excluded.cover, cover_file=excluded.cover_file, cover_thumb=excluded.cover_thumb,
			custom_category_id=excluded.custom_category_id, category_name=excluded.category_name,
			artist_names=excluded.artist_names, guest=excluded.guest, play=excluded.play, tag_ids=excluded.tag_ids,
			date=excluded.date, date_text=excluded.date_text, rating=excluded.rating, seat=excluded.seat,
			friends=excluded.friends, company=excluded.company, remark=excluded.remark, active_status=excluded.active_status,
			price=excluded.price, price_currency=excluded.price_currency, pay_price=excluded.pay_price,
			pay_price_currency=excluded.pay_price_currency, other_cost=excluded.other_cost, other_cost_currency=excluded.other_cost_currency
	`,
		r.ID, r.Name, r.Channel, r.City, r.Address, marshalJSON(r.Coordinate), r.Cover, r.CoverFile, r.CoverThumb,
		r.CustomCategoryID, r.CategoryName, marshalJSON(r.ArtistNames), marshalJSON(r.Guest), marshalJSON(r.Play), marshalJSON(r.TagIDs),
		r.Date, r.DateText, r.Rating, r.Seat, r.Friends, r.Company, r.Remark, r.ActiveStatus,
		r.Price, r.PriceCurrency, r.PayPrice, r.PayPriceCurrency, r.OtherCost, r.OtherCostCurrency,
	)
	return err
}

func (db *DB) CreateRecord(r models.RecordRequest) (*models.Record, error) {
	rec := requestToRecord(r)
	rec.ID = newID()
	if err := db.UpsertRecord(rec); err != nil {
		return nil, err
	}
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
	if err := db.UpsertRecord(rec); err != nil {
		return nil, err
	}
	return db.GetRecord(id)
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
	t := r.TagIDs
	if t == nil {
		t = []string{}
	}
	return models.Record{
		Name: r.Name, Channel: r.Channel, City: r.City, Address: r.Address,
		Coordinate: r.Coordinate, Cover: r.Cover, CoverFile: r.CoverFile, CoverThumb: r.CoverThumb,
		CustomCategoryID: r.CustomCategoryID, CategoryName: r.CategoryName,
		ArtistNames: a, Guest: g, Play: p, TagIDs: t,
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

func (db *DB) BatchUpdateRecords(ids []string, categoryName *string, rating *int, activeStatus *int) (int64, error) {
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

	sets := []string{}
	if categoryName != nil {
		sets = append(sets, "category_name = ?")
		args = append(args, *categoryName)
	}
	if rating != nil {
		sets = append(sets, "rating = ?")
		args = append(args, *rating)
	}
	if activeStatus != nil {
		sets = append(sets, "active_status = ?")
		args = append(args, *activeStatus)
	}
	if len(sets) == 0 {
		return 0, nil
	}
	res, err := db.conn.Exec("UPDATE records SET "+strings.Join(sets, ", ")+" WHERE id IN "+inClause, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	return res.RowsAffected()
}

// ---------- Categories ----------

func (db *DB) ListCategories() ([]models.Category, error) {
	rows, err := db.conn.Query(`SELECT id, name, active_ids, record_count FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		var c models.Category
		var activeIDs string
		if err := rows.Scan(&c.ID, &c.Name, &activeIDs, &c.RecordCount); err != nil {
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

func (db *DB) UpsertCategory(c models.Category) error {
	if c.ID == "" {
		c.ID = newID()
	}
	_, err := db.conn.Exec(`
		INSERT INTO categories (id, name, active_ids, record_count) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, active_ids=excluded.active_ids, record_count=excluded.record_count
	`, c.ID, c.Name, marshalJSON(c.ActiveIDs), c.RecordCount)
	return err
}

func (db *DB) DeleteCategory(id string) error {
	_, err := db.conn.Exec("DELETE FROM categories WHERE id = ?", id)
	return err
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
	upsert := func(key string, raw json.RawMessage) error {
		val := "[]"
		if len(raw) > 0 {
			val = string(raw)
		}
		_, err := db.conn.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, val)
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

	topRows, err := db.conn.Query(`SELECT `+recordColumns+` FROM records WHERE rating IS NOT NULL AND rating != 0 ORDER BY rating DESC, date DESC LIMIT 6`)
	if err == nil {
		defer topRows.Close()
		for topRows.Next() {
			if r, err := scanRecord(topRows); err == nil {
				s.TopRated = append(s.TopRated, *r)
			}
		}
	}

	recentRows, err := db.conn.Query(`SELECT `+recordColumns+` FROM records ORDER BY date DESC LIMIT 6`)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			if r, err := scanRecord(recentRows); err == nil {
				s.RecentRecords = append(s.RecentRecords, *r)
			}
		}
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
	rows, err := db.conn.Query("SELECT DISTINCT "+field+" FROM records WHERE "+field+" != '' ORDER BY "+field)
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
	return out, nil
}

