package db

import (
	"database/sql"
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

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
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

func (db *DB) Close() {
	db.conn.Close()
}

func (db *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			color TEXT NOT NULL DEFAULT '#4A90D9',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS shows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			venue TEXT NOT NULL DEFAULT '',
			date DATETIME NOT NULL,
			duration INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'planned',
			category_id INTEGER,
			poster_url TEXT NOT NULL DEFAULT '',
			setlist TEXT NOT NULL DEFAULT '',
			cast TEXT NOT NULL DEFAULT '',
			company TEXT NOT NULL DEFAULT '',
			friends TEXT NOT NULL DEFAULT '',
			rating INTEGER,
			seat TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			review TEXT NOT NULL DEFAULT '',
			ticket_cost REAL,
			other_cost REAL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL
		)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// add sort_order column if missing (upgrade from older version)
	db.conn.Exec("ALTER TABLE categories ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0")

	// migrate old status values to new ones
	db.conn.Exec("UPDATE shows SET status = 'normal' WHERE status = 'planned'")
	db.conn.Exec("UPDATE shows SET status = 'normal' WHERE status = 'watched'")

	// scene sorts table
	db.conn.Exec(`CREATE TABLE IF NOT EXISTS scene_sorts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		play TEXT NOT NULL UNIQUE,
		scenes TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)

	// seed default categories
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count)
	if count == 0 {
		defaults := []struct {
			name  string
			color string
		}{
			{"话剧", "#E74C3C"},
			{"音乐剧", "#9B59B6"},
			{"演唱会", "#E67E22"},
			{"舞蹈", "#1ABC9C"},
			{"相声", "#34495E"},
			{"脱口秀", "#2ECC71"},
			{"展览", "#3498DB"},
			{"其他", "#95A5A6"},
		}
		for _, d := range defaults {
			db.conn.Exec("INSERT INTO categories (name, color) VALUES (?, ?)", d.name, d.color)
		}
	}

	return nil
}

func (db *DB) ListShows(year, month int) ([]models.Show, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, db.loc)
	end := start.AddDate(0, 1, 0)

	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.date >= ? AND s.date < ?
		ORDER BY s.date ASC
	`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanShows(rows)
}

func (db *DB) ListShowsByDateRange(startStr, endStr string) ([]models.Show, error) {
	query := `
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE 1=1
	`
	args := []interface{}{}

	if startStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", startStr, db.loc); err == nil {
			query += " AND s.date >= ?"
			args = append(args, t)
		}
	}
	if endStr != "" {
		if t, err := time.ParseInLocation("2006-01-02", endStr, db.loc); err == nil {
			query += " AND s.date < ?"
			args = append(args, t.AddDate(0, 0, 1))
		}
	}

	query += " ORDER BY s.date ASC"

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanShows(rows)
}

func (db *DB) ListAllShows() ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		ORDER BY s.date DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetAutocomplete(field string) ([]string, error) {
	validFields := map[string]bool{
		"company": true, "cast": true, "friends": true, "venue": true, "seat": true,
	}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}

	quoted := "`" + field + "`"
	rows, err := db.conn.Query(
		"SELECT DISTINCT " + quoted + " FROM shows WHERE " + quoted + " != '' ORDER BY " + quoted,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

func (db *DB) GetShowsByField(field, value string) ([]models.Show, error) {
	validFields := map[string]bool{
		"company": true, "cast": true, "friends": true,
		"venue": true, "setlist": true,
	}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}

	quoted := "`" + field + "`"
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE `+quoted+` LIKE ?
		ORDER BY s.date DESC
	`, "%"+value+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetShow(id int64) (*models.Show, error) {
	var s models.Show
	err := db.conn.QueryRow(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.id = ?
	`, id).Scan(
		&s.ID, &s.Name, &s.Venue, &s.Date, &s.Duration, &s.Status, &s.CategoryID,
		&s.PosterURL, &s.Setlist, &s.Cast, &s.Company, &s.Friends, &s.Rating,
		&s.Seat, &s.Notes, &s.Review, &s.TicketCost, &s.OtherCost,
		&s.CreatedAt, &s.UpdatedAt, &s.CategoryName,
	)
	if err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}
	return &s, nil
}

func (db *DB) CreateShow(req models.ShowRequest) (*models.Show, error) {
	date, err := time.ParseInLocation("2006-01-02T15:04", req.Date, db.loc)
	if err != nil {
		date, err = time.ParseInLocation("2006-01-02", req.Date, db.loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %w", err)
		}
	}

	// check for duplicate by name + date (use date-only format for matching)
	dateOnly := date.Format("2006-01-02")
	var existingID int64
	err = db.conn.QueryRow("SELECT id FROM shows WHERE name = ? AND date LIKE ?", req.Name, dateOnly+"%").Scan(&existingID)
	if err == nil {
		return db.GetShow(existingID)
	}

	status := req.Status
	if status == "" {
		status = "normal"
	}

	result, err := db.conn.Exec(`
		INSERT INTO shows (name, venue, date, duration, status, category_id, poster_url,
			setlist, cast, company, friends, rating, seat, notes, review, ticket_cost, other_cost)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.Venue, date, req.Duration, status, req.CategoryID, req.PosterURL,
		req.Setlist, req.Cast, req.Company, req.Friends, req.Rating, req.Seat,
		req.Notes, req.Review, req.TicketCost, req.OtherCost)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return db.GetShow(id)
}

func (db *DB) UpdateShow(id int64, req models.ShowRequest) (*models.Show, error) {
	date, err := time.ParseInLocation("2006-01-02T15:04", req.Date, db.loc)
	if err != nil {
		date, err = time.ParseInLocation("2006-01-02", req.Date, db.loc)
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %w", err)
		}
	}

	_, err = db.conn.Exec(`
		UPDATE shows SET name=?, venue=?, date=?, duration=?, status=?, category_id=?,
			poster_url=?, setlist=?, cast=?, company=?, friends=?, rating=?, seat=?,
			notes=?, review=?, ticket_cost=?, other_cost=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, req.Name, req.Venue, date, req.Duration, req.Status, req.CategoryID, req.PosterURL,
		req.Setlist, req.Cast, req.Company, req.Friends, req.Rating, req.Seat,
		req.Notes, req.Review, req.TicketCost, req.OtherCost, id)
	if err != nil {
		return nil, err
	}

	return db.GetShow(id)
}

func (db *DB) DeleteShow(id int64) error {
	_, err := db.conn.Exec("DELETE FROM shows WHERE id=?", id)
	return err
}

func (db *DB) ListCategories() ([]models.Category, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.name, c.color, c.sort_order, COUNT(s.id) as show_count
		FROM categories c
		LEFT JOIN shows s ON s.category_id = c.id
		GROUP BY c.id
		ORDER BY c.sort_order, c.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.SortOrder, &c.ShowCount); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (db *DB) CreateCategory(name, color string) (*models.Category, error) {
	result, err := db.conn.Exec("INSERT INTO categories (name, color) VALUES (?, ?)", name, color)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &models.Category{ID: id, Name: name, Color: color}, nil
}

func (db *DB) FindOrCreateCategory(name string) (*models.Category, error) {
	var c models.Category
	err := db.conn.QueryRow("SELECT id, name, color FROM categories WHERE name = ?", name).Scan(&c.ID, &c.Name, &c.Color)
	if err == nil {
		return &c, nil
	}

	colors := []string{"#E74C3C", "#9B59B6", "#E67E22", "#1ABC9C", "#34495E", "#2ECC71", "#3498DB"}
	color := colors[len(name)%len(colors)]
	return db.CreateCategory(name, color)
}

func (db *DB) UpdateCategory(id int64, name, color string) error {
	_, err := db.conn.Exec("UPDATE categories SET name=?, color=? WHERE id=?", name, color, id)
	return err
}

func (db *DB) DeleteCategory(id int64) error {
	_, err := db.conn.Exec("DELETE FROM categories WHERE id=?", id)
	return err
}

func (db *DB) UpdateCategorySort(ids []int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE categories SET sort_order = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range ids {
		if _, err := stmt.Exec(i, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) BatchUpdateShows(ids []int64, categoryID *int64, rating *int, status *string, ticketCost *float64) (int64, error) {
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
	if categoryID != nil {
		sets = append(sets, "category_id = ?")
		args = append(args, *categoryID)
	}
	if rating != nil {
		sets = append(sets, "rating = ?")
		args = append(args, *rating)
	}
	if status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *status)
	}
	if ticketCost != nil {
		sets = append(sets, "ticket_cost = ?")
		args = append(args, *ticketCost)
	}

	if len(sets) == 0 {
		return 0, nil
	}

	query := "UPDATE shows SET " + strings.Join(sets, ", ") + ", updated_at = CURRENT_TIMESTAMP WHERE id IN " + inClause

	result, err := db.conn.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) BatchDeleteShows(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := "(" + strings.Join(placeholders, ",") + ")"

	result, err := db.conn.Exec("DELETE FROM shows WHERE id IN "+inClause, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) GetCalendarEvents(year, month int) ([]models.CalendarEvent, error) {
	shows, err := db.ListShows(year, month)
	if err != nil {
		return nil, err
	}

	colors := map[string]string{
		"normal":         "#27AE60",
		"cancelled":      "#E74C3C",
		"pending_tickets": "#F39C12",
		"no_show":        "#95A5A6",
	}

	events := make([]models.CalendarEvent, len(shows))
	for i, s := range shows {
		color := colors[string(s.Status)]
		events[i] = models.CalendarEvent{
			ID:        s.ID,
			Name:      s.Name,
			Venue:     s.Venue,
			Date:      s.Date.Format("2006-01-02"),
			Duration:  s.Duration,
			Status:    string(s.Status),
			Color:     color,
			PosterURL: s.PosterURL,
		}
	}
	return events, nil
}

func (db *DB) GetStats() (*models.Stats, error) {
	stats := &models.Stats{}

	db.conn.QueryRow("SELECT COUNT(*) FROM shows").Scan(&stats.TotalShows)
	db.conn.QueryRow("SELECT COALESCE(SUM(COALESCE(ticket_cost,0) + COALESCE(other_cost,0)), 0) FROM shows").Scan(&stats.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM shows WHERE rating IS NOT NULL").Scan(&stats.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT venue) FROM shows WHERE venue != ''").Scan(&stats.TotalVenues)
	db.conn.QueryRow("SELECT COALESCE(SUM(duration), 0) / 60.0 FROM shows").Scan(&stats.TotalHours)

	return stats, nil
}

func (db *DB) GetUpcomingShows(limit int) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.date >= datetime('now') AND s.status IN ('normal', 'pending_tickets')
		ORDER BY s.date ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetRecentShows(limit int) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.status = 'normal'
		ORDER BY s.date DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) SearchShows(query string) ([]models.Show, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return []models.Show{}, nil
	}

	where := make([]string, 0, len(words))
	args := make([]interface{}, 0, len(words)*9)
	for _, w := range words {
		like := "%" + w + "%"
		where = append(where, `(LOWER(s.name) LIKE ? OR LOWER(s.venue) LIKE ? OR LOWER(s.company) LIKE ?
			OR LOWER(s.cast) LIKE ? OR LOWER(s.friends) LIKE ? OR LOWER(s.review) LIKE ?
			OR LOWER(s.notes) LIKE ? OR LOWER(s.setlist) LIKE ? OR LOWER(s.seat) LIKE ?
			OR LOWER(c.name) LIKE ?)`)
		for i := 0; i < 10; i++ {
			args = append(args, like)
		}
	}

	q := "WHERE " + strings.Join(where, " AND ")

	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		`+q+`
		ORDER BY s.date DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func scanShows(rows *sql.Rows) ([]models.Show, error) {
	var shows []models.Show
	for rows.Next() {
		var s models.Show
		err := rows.Scan(
			&s.ID, &s.Name, &s.Venue, &s.Date, &s.Duration, &s.Status, &s.CategoryID,
			&s.PosterURL, &s.Setlist, &s.Cast, &s.Company, &s.Friends, &s.Rating,
			&s.Seat, &s.Notes, &s.Review, &s.TicketCost, &s.OtherCost,
			&s.CreatedAt, &s.UpdatedAt, &s.CategoryName,
		)
		if err != nil {
			log.Printf("scan show: %v", err)
			continue
		}
		shows = append(shows, s)
	}
	return shows, nil
}

func (db *DB) GetSceneSorts() ([]models.SceneSort, error) {
	rows, err := db.conn.Query("SELECT id, play, scenes, updated_at FROM scene_sorts ORDER BY play")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sorts []models.SceneSort
	for rows.Next() {
		var s models.SceneSort
		if err := rows.Scan(&s.ID, &s.Play, &s.Scenes, &s.UpdatedAt); err != nil {
			continue
		}
		sorts = append(sorts, s)
	}
	return sorts, nil
}

func (db *DB) UpdateSceneSort(play, scenes string) error {
	_, err := db.conn.Exec(
		`INSERT INTO scene_sorts (play, scenes, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(play) DO UPDATE SET scenes = excluded.scenes, updated_at = excluded.updated_at`,
		play, scenes,
	)
	return err
}

func (db *DB) DeleteSceneSort(play string) error {
	_, err := db.conn.Exec("DELETE FROM scene_sorts WHERE play = ?", play)
	return err
}
