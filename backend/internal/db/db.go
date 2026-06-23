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

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
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
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
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
	date, err := time.Parse("2006-01-02T15:04", req.Date)
	if err != nil {
		date, err = time.Parse("2006-01-02", req.Date)
		if err != nil {
			return nil, fmt.Errorf("invalid date format: %w", err)
		}
	}

	status := req.Status
	if status == "" {
		status = "planned"
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
	date, err := time.Parse("2006-01-02T15:04", req.Date)
	if err != nil {
		date, err = time.Parse("2006-01-02", req.Date)
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

func (db *DB) GetCalendarEvents(year, month int) ([]models.CalendarEvent, error) {
	shows, err := db.ListShows(year, month)
	if err != nil {
		return nil, err
	}

	colors := map[string]string{
		"planned":   "#4A90D9",
		"watched":   "#27AE60",
		"cancelled": "#E74C3C",
	}

	events := make([]models.CalendarEvent, len(shows))
	for i, s := range shows {
		color := colors[string(s.Status)]
		events[i] = models.CalendarEvent{
			ID:       s.ID,
			Name:     s.Name,
			Venue:    s.Venue,
			Date:     s.Date.Format("2006-01-02"),
			Duration: s.Duration,
			Status:   string(s.Status),
			Color:    color,
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
		WHERE s.date >= datetime('now') AND s.status = 'planned'
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
		WHERE s.status = 'watched'
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
	q := "%" + strings.ToLower(query) + "%"
	rows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE LOWER(s.name) LIKE ? OR LOWER(s.venue) LIKE ? OR LOWER(s.company) LIKE ?
		   OR LOWER(s.cast) LIKE ? OR LOWER(s.friends) LIKE ?
		ORDER BY s.date DESC
	`, q, q, q, q, q)
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
