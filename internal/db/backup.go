package db

import (
	"encoding/json"
	"fmt"
	"mujian/internal/models"
	"os"
	"strings"
	"time"
)

// Export builds an ExportData that is byte-for-byte compatible with the
// recordlive_export/data.json source format.
func (db *DB) Export() (*models.ExportData, error) {
	// Export must contain every record: bypass the list row caps.
	records, err := db.ListRecords(RecordFilter{NoLimit: true})
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
	photos, err := db.ListAllRecordPhotos()
	if err != nil {
		return nil, fmt.Errorf("export record photos: %w", err)
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
		RecordPhotos: photos,
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
//
// Records that fail the minimum validation rules (see validateImportRecord) are
// skipped and reported in ImportResult.Issues rather than written.
func (db *DB) ImportData(data *models.ExportData) (*ImportResult, error) {
	return db.importData(data, false)
}

// PreviewImport runs the exact same validation and counting as ImportData but
// writes nothing, so callers can show 新增 / 覆盖 / 跳过 before committing.
func (db *DB) PreviewImport(data *models.ExportData) (*ImportResult, error) {
	return db.importData(data, true)
}

func (db *DB) importData(data *models.ExportData, dryRun bool) (*ImportResult, error) {
	result := &ImportResult{DryRun: dryRun}

	// 先整体校验再写库：拒收记录不进入事务，避免"导到一半才失败"留下半截数据。
	kept := make([]models.Record, 0, len(data.Records))
	noDateCount := 0
	for i := range data.Records {
		rec := data.Records[i]
		issue, missingDate := validateImportRecord(&rec, i)
		if issue != nil {
			result.Skipped++
			if len(result.Issues) < maxImportIssues {
				result.Issues = append(result.Issues, *issue)
			}
			continue
		}
		if missingDate {
			noDateCount++
		}
		kept = append(kept, rec)
	}
	if noDateCount > 0 {
		result.Warnings = append(result.Warnings, ImportWarning{Reason: "缺少日期", Count: noDateCount})
	}

	existing := db.existingRecordIDs(kept)
	for _, r := range kept {
		if existing[r.ID] {
			result.UpdatedRecords++
		} else {
			result.NewRecords++
		}
	}
	if dryRun {
		return result, nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for i := range kept {
		if err := db.UpsertRecordTx(tx, kept[i]); err != nil {
			return nil, fmt.Errorf("import record %s: %w", kept[i].Name, err)
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

	// 票根关联：按 record_id 整体替换（旧版导出无该字段则跳过）。
	for _, p := range data.RecordPhotos {
		if err := db.replaceRecordPhotosTx(tx, p.RecordID, p); err != nil {
			return nil, fmt.Errorf("import record photo %s: %w", p.FileName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// existingRecordIDs reports which of the given ids already exist, in chunks so
// the IN(...) list stays under SQLite's variable limit.
func (db *DB) existingRecordIDs(recs []models.Record) map[string]bool {
	out := map[string]bool{}
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		if r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	const chunk = 400
	for start := 0; start < len(ids); start += chunk {
		end := start + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[start:end]
		ph := strings.TrimSuffix(strings.Repeat("?,", len(part)), ",")
		args := make([]any, len(part))
		for i, s := range part {
			args[i] = s
		}
		rows, err := db.conn.Query("SELECT id FROM records WHERE id IN ("+ph+")", args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				out[id] = true
			}
		}
		rows.Close()
	}
	return out
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
	// 校验与预览（N5）。DryRun=true 时不做任何写入，只回报下面的计数。
	DryRun bool `json:"dry_run,omitempty"`
	// NewRecords / UpdatedRecords 按 id 是否已存在区分（预览用）。
	NewRecords     int `json:"new_records"`
	UpdatedRecords int `json:"updated_records"`
	// Skipped 是被拒收的记录数（当前规则：名称为空）。
	Skipped int `json:"skipped"`
	// Issues 列出拒收明细，最多保留 50 条，避免把整个文件回吐给前端。
	Issues []ImportIssue `json:"issues,omitempty"`
	// Warnings 是不阻断导入但值得提示的问题（当前规则：无日期）。
	Warnings []ImportWarning `json:"warnings,omitempty"`
}

// ImportIssue is one rejected record in an import payload.
type ImportIssue struct {
	Index  int    `json:"index"`
	ID     string `json:"id,omitempty"`
	Reason string `json:"reason"`
}

// ImportWarning is a non-blocking problem in an import payload.
type ImportWarning struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

const maxImportIssues = 50

// validateImportRecord applies the minimum data-hygiene rules an import must
// satisfy. The HTTP import/restore path previously accepted anything, which is
// how records with an empty name/date/city/address ended up in the database and
// then polluted stats, /records/all and the ICS feed.
func validateImportRecord(r *models.Record, index int) (reject *ImportIssue, noDate bool) {
	if strings.TrimSpace(r.Name) == "" {
		return &ImportIssue{Index: index, ID: r.ID, Reason: "名称为空"}, false
	}
	return nil, r.Date == 0 && strings.TrimSpace(r.DateText) == ""
}

