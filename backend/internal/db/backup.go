package db

import (
	"encoding/json"
	"fmt"
	"mujian/internal/models"
	"os"
	"time"
)

// Export builds an ExportData that is byte-for-byte compatible with the
// recordlive_export/data.json source format.
func (db *DB) Export() (*models.ExportData, error) {
	records, err := db.ListRecords(RecordFilter{})
	if err != nil {
		return nil, fmt.Errorf("export records: %w", err)
	}
	categories, err := db.ListCategories()
	if err != nil {
		return nil, fmt.Errorf("export categories: %w", err)
	}
	meta, err := db.GetMeta()
	if err != nil {
		return nil, fmt.Errorf("export meta: %w", err)
	}

	return &models.ExportData{
		Source:       "mujian",
		ExportedAt:   time.Now().Format("2006-01-02T15:04:05"),
		RecordCount:  len(records),
		CoverMissing: 0,
		CoverDir:     "covers/",
		CoverNote:    "每条记录的 coverFile 字段为封面图相对路径（covers/<uuid>.<ext>），ext 为 jpg/png/webp。",
		Meta:         *meta,
		Records:      records,
		Categories:   categories,
	}, nil
}

func (db *DB) ExportToFile(path string) error {
	data, err := db.Export()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// ImportData loads an ExportData (the recordlive_export format) into the
// database, replacing records/categories by their ids. Meta is also restored.
func (db *DB) ImportData(data *models.ExportData) (*ImportResult, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result := &ImportResult{}

	for i := range data.Records {
		if err := db.UpsertRecordTx(tx, data.Records[i]); err != nil {
			return nil, fmt.Errorf("import record %s: %w", data.Records[i].Name, err)
		}
		result.Records++
	}

	for i := range data.Categories {
		if err := db.UpsertCategoryTx(tx, &data.Categories[i]); err != nil {
			return nil, fmt.Errorf("import category %s: %w", data.Categories[i].Name, err)
		}
		result.Categories++
	}

	if err := db.SetMetaTx(tx, &data.Meta); err != nil {
		return nil, fmt.Errorf("import meta: %w", err)
	}

	return result, tx.Commit()
}

func (db *DB) ImportFromFile(path string) (*ImportResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data models.ExportData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("invalid export file: %w", err)
	}
	return db.ImportData(&data)
}

type ImportResult struct {
	Records    int `json:"records"`
	Categories int `json:"categories"`
}
