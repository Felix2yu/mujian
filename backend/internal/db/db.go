package db

import (
	"database/sql"
	"fmt"
	"log"
	"mujian/internal/models"
	"os"
	"path/filepath"
	"sort"
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
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			color TEXT NOT NULL DEFAULT '#4A90D9',
			sort_order INTEGER NOT NULL DEFAULT 0,
			UNIQUE(user_id, name),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS shows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			venue TEXT NOT NULL DEFAULT '',
			date DATETIME NOT NULL,
			duration INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'normal',
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
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE SET NULL
		)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}

	// Migrate: add user_id to existing tables if missing
	for _, table := range []string{"shows", "categories", "scene_sorts", "actors"} {
		var count int
		db.conn.QueryRow("SELECT COUNT(*) FROM pragma_table_info('" + table + "') WHERE name = 'user_id'").Scan(&count)
		if count == 0 {
			db.conn.Exec("ALTER TABLE " + table + " ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0")
		}
	}

	// Assign orphaned data (user_id=0) to first user
	var firstUserID int64
	err := db.conn.QueryRow("SELECT id FROM users ORDER BY id LIMIT 1").Scan(&firstUserID)
	if err == nil && firstUserID > 0 {
		for _, table := range []string{"shows", "categories", "scene_sorts", "actors"} {
			db.conn.Exec("UPDATE "+table+" SET user_id = ? WHERE user_id = 0", firstUserID)
		}
	}

	// scene sorts table
	db.conn.Exec(`CREATE TABLE IF NOT EXISTS scene_sorts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		play TEXT NOT NULL,
		scenes TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, play),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`)

	// actors table
	db.conn.Exec(`CREATE TABLE IF NOT EXISTS actors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		bio TEXT NOT NULL DEFAULT '',
		photo_url TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, name),
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`)

	return nil
}

func (db *DB) ListShows(userID int64, year, month int) ([]models.Show, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, db.loc)
	end := start.AddDate(0, 1, 0)

	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ? AND s.date >= ? AND s.date < ?
		ORDER BY s.date ASC
	`, userID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanShows(rows)
}

func (db *DB) ListShowsByDateRange(userID int64, startStr, endStr string) ([]models.Show, error) {
	query := `
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ?
	`
	args := []interface{}{userID}

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

func (db *DB) ListAllShows(userID int64) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ?
		ORDER BY s.date DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetAutocomplete(userID int64, field string) ([]string, error) {
	validFields := map[string]bool{
		"company": true, "cast": true, "friends": true, "venue": true, "seat": true,
	}
	if !validFields[field] {
		return nil, fmt.Errorf("invalid field: %s", field)
	}

	quoted := "`" + field + "`"
	rows, err := db.conn.Query(
		"SELECT DISTINCT "+quoted+" FROM shows WHERE user_id = ? AND "+quoted+" != '' ORDER BY "+quoted,
		userID,
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

func (db *DB) GetShowsByField(userID int64, field, value string) ([]models.Show, error) {
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
		WHERE s.user_id = ? AND `+quoted+` LIKE ?
		ORDER BY s.date DESC
	`, userID, "%"+value+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetShow(userID, id int64) (*models.Show, error) {
	var s models.Show
	err := db.conn.QueryRow(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ? AND s.id = ?
	`, userID, id).Scan(
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

func (db *DB) CreateShow(userID int64, req models.ShowRequest) (*models.Show, error) {
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
	err = db.conn.QueryRow("SELECT id FROM shows WHERE user_id = ? AND name = ? AND date LIKE ?", userID, req.Name, dateOnly+"%").Scan(&existingID)
	if err == nil {
		return db.GetShow(userID, existingID)
	}

	status := req.Status
	if status == "" {
		status = "normal"
	}

	result, err := db.conn.Exec(`
		INSERT INTO shows (user_id, name, venue, date, duration, status, category_id, poster_url,
			setlist, cast, company, friends, rating, seat, notes, review, ticket_cost, other_cost)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, req.Name, req.Venue, date, req.Duration, status, req.CategoryID, req.PosterURL,
		req.Setlist, req.Cast, req.Company, req.Friends, req.Rating, req.Seat,
		req.Notes, req.Review, req.TicketCost, req.OtherCost)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return db.GetShow(userID, id)
}

func (db *DB) UpdateShow(userID, id int64, req models.ShowRequest) (*models.Show, error) {
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
		WHERE user_id=? AND id=?
	`, req.Name, req.Venue, date, req.Duration, req.Status, req.CategoryID, req.PosterURL,
		req.Setlist, req.Cast, req.Company, req.Friends, req.Rating, req.Seat,
		req.Notes, req.Review, req.TicketCost, req.OtherCost, userID, id)
	if err != nil {
		return nil, err
	}

	return db.GetShow(userID, id)
}

func (db *DB) DeleteShow(userID, id int64) error {
	_, err := db.conn.Exec("DELETE FROM shows WHERE user_id=? AND id=?", userID, id)
	return err
}

func (db *DB) ListCategories(userID int64) ([]models.Category, error) {
	rows, err := db.conn.Query(`
		SELECT c.id, c.name, c.color, c.sort_order, COUNT(s.id) as show_count
		FROM categories c
		LEFT JOIN shows s ON s.category_id = c.id AND s.user_id = c.user_id
		WHERE c.user_id = ?
		GROUP BY c.id
		ORDER BY c.sort_order, c.name
	`, userID)
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

func (db *DB) CreateCategory(userID int64, name, color string) (*models.Category, error) {
	result, err := db.conn.Exec("INSERT INTO categories (user_id, name, color) VALUES (?, ?, ?)", userID, name, color)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &models.Category{ID: id, Name: name, Color: color}, nil
}

func (db *DB) FindOrCreateCategory(userID int64, name string) (*models.Category, error) {
	var c models.Category
	err := db.conn.QueryRow("SELECT id, name, color FROM categories WHERE user_id = ? AND name = ?", userID, name).Scan(&c.ID, &c.Name, &c.Color)
	if err == nil {
		return &c, nil
	}

	colors := []string{"#E74C3C", "#9B59B6", "#E67E22", "#1ABC9C", "#34495E", "#2ECC71", "#3498DB"}
	color := colors[len(name)%len(colors)]
	return db.CreateCategory(userID, name, color)
}

func (db *DB) UpdateCategory(userID, id int64, name, color string) error {
	_, err := db.conn.Exec("UPDATE categories SET name=?, color=? WHERE user_id=? AND id=?", name, color, userID, id)
	return err
}

func (db *DB) DeleteCategory(userID, id int64) error {
	_, err := db.conn.Exec("DELETE FROM categories WHERE user_id=? AND id=?", userID, id)
	return err
}

func (db *DB) UpdateCategorySort(userID int64, ids []int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE categories SET sort_order = ? WHERE user_id = ? AND id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, id := range ids {
		if _, err := stmt.Exec(i, userID, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) BatchUpdateShows(userID int64, ids []int64, categoryID *int64, rating *int, status *string, ticketCost *float64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
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

	query := "UPDATE shows SET " + strings.Join(sets, ", ") + ", updated_at = CURRENT_TIMESTAMP WHERE user_id = ? AND id IN " + inClause

	result, err := db.conn.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) BatchDeleteShows(userID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := "(" + strings.Join(placeholders, ",") + ")"

	result, err := db.conn.Exec("DELETE FROM shows WHERE user_id = ? AND id IN "+inClause, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) GetCalendarEvents(userID int64, year, month int) ([]models.CalendarEvent, error) {
	shows, err := db.ListShows(userID, year, month)
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

func (db *DB) GetStats(userID int64) (*models.Stats, error) {
	stats := &models.Stats{}

	db.conn.QueryRow("SELECT COUNT(*) FROM shows WHERE user_id = ?", userID).Scan(&stats.TotalShows)
	db.conn.QueryRow("SELECT COALESCE(SUM(COALESCE(ticket_cost,0) + COALESCE(other_cost,0)), 0) FROM shows WHERE user_id = ?", userID).Scan(&stats.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM shows WHERE user_id = ? AND rating IS NOT NULL", userID).Scan(&stats.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT venue) FROM shows WHERE user_id = ? AND venue != ''", userID).Scan(&stats.TotalVenues)
	db.conn.QueryRow("SELECT COALESCE(SUM(duration), 0) / 60.0 FROM shows WHERE user_id = ?", userID).Scan(&stats.TotalHours)

	return stats, nil
}

func (db *DB) GetUpcomingShows(userID int64, limit int) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ? AND s.date >= datetime('now') AND s.status IN ('normal', 'pending_tickets')
		ORDER BY s.date ASC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) GetRecentShows(userID int64, limit int) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ? AND s.status = 'normal'
		ORDER BY s.date DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) SearchShows(userID int64, query string) ([]models.Show, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return []models.Show{}, nil
	}

	where := make([]string, 0, len(words))
	args := make([]interface{}, 0, len(words)*9+1)
	args = append(args, userID)
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

	q := "WHERE s.user_id = ? AND " + strings.Join(where, " AND ")

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

func (db *DB) GetSceneSorts(userID int64) ([]models.SceneSort, error) {
	rows, err := db.conn.Query("SELECT id, play, scenes, updated_at FROM scene_sorts WHERE user_id = ? ORDER BY play", userID)
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

func (db *DB) UpdateSceneSort(userID int64, play, scenes string) error {
	_, err := db.conn.Exec(
		`INSERT INTO scene_sorts (user_id, play, scenes, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id, play) DO UPDATE SET scenes = excluded.scenes, updated_at = excluded.updated_at`,
		userID, play, scenes,
	)
	return err
}

func (db *DB) DeleteSceneSort(userID int64, play string) error {
	_, err := db.conn.Exec("DELETE FROM scene_sorts WHERE user_id = ? AND play = ?", userID, play)
	return err
}

func (db *DB) ListActors(userID int64) ([]models.Actor, error) {
	shows, err := db.ListAllShows(userID)
	if err != nil {
		return nil, err
	}

	type actorInfo struct {
		name      string
		showCount int
	}
	actorMap := make(map[string]*actorInfo)
	for _, s := range shows {
		if s.Cast == "" {
			continue
		}
		parts := strings.Split(s.Cast, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			p = strings.TrimLeft(p, "，")
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, exists := actorMap[p]; !exists {
				actorMap[p] = &actorInfo{name: p}
			}
			actorMap[p].showCount++
		}
	}

	var actors []models.Actor
	for _, info := range actorMap {
		a := models.Actor{Name: info.name, ShowCount: info.showCount}
		row := db.conn.QueryRow("SELECT id, bio, photo_url FROM actors WHERE user_id = ? AND name = ?", userID, info.name)
		row.Scan(&a.ID, &a.Bio, &a.PhotoURL)
		actors = append(actors, a)
	}
	sort.Slice(actors, func(i, j int) bool {
		return actors[i].Name < actors[j].Name
	})
	return actors, nil
}

func (db *DB) GetActor(userID int64, name string) (*models.Actor, error) {
	var a models.Actor
	err := db.conn.QueryRow(`
		SELECT a.id, a.name, a.bio, a.photo_url,
			COUNT(DISTINCT s.id) as show_count
		FROM actors a
		LEFT JOIN shows s ON s.cast LIKE '%' || a.name || '%' AND s.user_id = a.user_id
		WHERE a.user_id = ? AND a.name = ?
		GROUP BY a.id
	`, userID, name).Scan(&a.ID, &a.Name, &a.Bio, &a.PhotoURL, &a.ShowCount)
	if err != nil {
		return nil, fmt.Errorf("actor not found: %w", err)
	}
	return &a, nil
}

func (db *DB) UpsertActor(userID int64, req models.ActorRequest) (*models.Actor, error) {
	_, err := db.conn.Exec(`
		INSERT INTO actors (user_id, name, bio, photo_url, updated_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, name) DO UPDATE SET bio = excluded.bio, photo_url = excluded.photo_url, updated_at = CURRENT_TIMESTAMP
	`, userID, req.Name, req.Bio, req.PhotoURL)
	if err != nil {
		return nil, err
	}
	return db.GetActor(userID, req.Name)
}

func (db *DB) GetShowsByActor(userID int64, name string) ([]models.Show, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.user_id = ? AND s.cast LIKE ?
		ORDER BY s.date DESC
	`, userID, "%"+name+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShows(rows)
}

func (db *DB) CreateUser(username, passwordHash string) (*models.User, error) {
	result, err := db.conn.Exec(
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()

	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '话剧', '#E74C3C', 0)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '音乐剧', '#9B59B6', 1)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '演唱会', '#E67E22', 2)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '舞蹈', '#1ABC9C', 3)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '相声', '#34495E', 4)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '脱口秀', '#2ECC71', 5)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '展览', '#3498DB', 6)`, id)
	db.conn.Exec(`INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, '其他', '#95A5A6', 7)`, id)

	return db.GetUserByID(id)
}

func (db *DB) GetUserByID(id int64) (*models.User, error) {
	var u models.User
	err := db.conn.QueryRow(
		"SELECT id, username, created_at FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Username, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) GetUserByUsername(username string) (*models.User, error) {
	var u models.User
	err := db.conn.QueryRow(
		"SELECT id, username, password_hash, created_at FROM users WHERE username = ?", username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) GetPasswordHash(userID int64) (string, error) {
	var hash string
	err := db.conn.QueryRow("SELECT password_hash FROM users WHERE id = ?", userID).Scan(&hash)
	return hash, err
}

func (db *DB) UpdatePassword(userID int64, hash string) error {
	_, err := db.conn.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	return err
}

func (db *DB) DeleteUser_data(userID int64) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tx.Exec("DELETE FROM shows WHERE user_id = ?", userID)
	tx.Exec("DELETE FROM categories WHERE user_id = ?", userID)
	tx.Exec("DELETE FROM scene_sorts WHERE user_id = ?", userID)
	tx.Exec("DELETE FROM actors WHERE user_id = ?", userID)
	tx.Exec("DELETE FROM users WHERE id = ?", userID)

	return tx.Commit()
}
