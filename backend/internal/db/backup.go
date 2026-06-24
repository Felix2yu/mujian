package db

import (
	"encoding/json"
	"fmt"
	"mujian/internal/models"
	"os"
	"time"
)

type BackupData struct {
	Version    string            `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Categories []models.Category `json:"categories"`
	Shows      []models.Show     `json:"shows"`
}

func (db *DB) Export(userID int64) (*BackupData, error) {
	cats, err := db.ListCategories(userID)
	if err != nil {
		return nil, fmt.Errorf("export categories: %w", err)
	}

	shows, err := db.ListAllShows(userID)
	if err != nil {
		return nil, fmt.Errorf("export shows: %w", err)
	}

	return &BackupData{
		Version:    "1.0",
		ExportedAt: time.Now(),
		Categories: cats,
		Shows:      shows,
	}, nil
}

func (db *DB) ExportToFile(userID int64, path string) error {
	data, err := db.Export(userID)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0644)
}

type ImportResult struct {
	Categories int
	Shows      int
	Skipped    int
}

func (db *DB) Import(userID int64, data *BackupData) (*ImportResult, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result := &ImportResult{}

	// build category id mapping (old id -> new id)
	catMap := make(map[int64]int64)

	for _, cat := range data.Categories {
		var existingID int64
		err := tx.QueryRow("SELECT id FROM categories WHERE user_id = ? AND name = ?", userID, cat.Name).Scan(&existingID)
		if err == nil {
			catMap[cat.ID] = existingID
			tx.Exec("UPDATE categories SET color = ?, sort_order = ? WHERE user_id = ? AND id = ?", cat.Color, cat.SortOrder, userID, existingID)
			continue
		}

		execResult, err := tx.Exec(
			"INSERT INTO categories (user_id, name, color, sort_order) VALUES (?, ?, ?, ?)",
			userID, cat.Name, cat.Color, cat.SortOrder,
		)
		if err != nil {
			return nil, fmt.Errorf("import category %s: %w", cat.Name, err)
		}
		newID, _ := execResult.LastInsertId()
		catMap[cat.ID] = newID
		result.Categories++
	}

	for _, show := range data.Shows {
		// deduplicate by name + date (use date-only format for matching)
		dateOnly := show.Date.Format("2006-01-02")
		var existingID int64
		err := tx.QueryRow("SELECT id FROM shows WHERE user_id = ? AND name = ? AND date LIKE ?", userID, show.Name, dateOnly+"%").Scan(&existingID)
		if err == nil {
			result.Skipped++
			continue
		}

		var newCatID *int64
		if show.CategoryID != nil {
			if newID, ok := catMap[*show.CategoryID]; ok {
				newCatID = &newID
			}
		}

		_, err = tx.Exec(`
			INSERT INTO shows (user_id, name, venue, date, duration, status, category_id, poster_url,
				setlist, cast, company, friends, rating, seat, notes, review, ticket_cost, other_cost,
				created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			userID, show.Name, show.Venue, show.Date, show.Duration, show.Status, newCatID,
			show.PosterURL, show.Setlist, show.Cast, show.Company, show.Friends,
			show.Rating, show.Seat, show.Notes, show.Review, show.TicketCost, show.OtherCost,
			show.CreatedAt, show.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("import show %s: %w", show.Name, err)
		}
		result.Shows++
	}

	return result, tx.Commit()
}

func (db *DB) ImportFromFile(userID int64, path string) (*ImportResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var data BackupData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("invalid backup file: %w", err)
	}

	return db.Import(userID, &data)
}
