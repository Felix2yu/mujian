package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

func TestPreviewImport(t *testing.T) {
	db := newTestDB(t)

	base := time.Now().Unix()
	data := &models.ExportData{
		Source:     "mujian",
		Records: []models.Record{
			{ID: "pi1", Name: "新记录", Date: base},
			{ID: "pi2", Name: "另一条", Date: base},
		},
		Categories: []models.Category{
			{Name: "昆曲"},
		},
	}

	// PreviewImport with no existing records → all should be "new"
	result, err := db.PreviewImport(data)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}
	if result.NewRecords != 2 {
		t.Errorf("NewRecords = %d, want 2", result.NewRecords)
	}
	if result.UpdatedRecords != 0 {
		t.Errorf("UpdatedRecords = %d, want 0", result.UpdatedRecords)
	}
	if result.Records != 0 {
		t.Error("Records should be 0 in dry run (no writes)")
	}

	// Now actually import one record, then preview again
	_, err = db.ImportData(data)
	if err != nil {
		t.Fatal(err)
	}

	// Preview again — pi1 and pi2 now exist, so all should be "updated"
	result2, err := db.PreviewImport(data)
	if err != nil {
		t.Fatal(err)
	}
	if result2.NewRecords != 0 {
		t.Errorf("NewRecords = %d, want 0 (all exist)", result2.NewRecords)
	}
	if result2.UpdatedRecords != 2 {
		t.Errorf("UpdatedRecords = %d, want 2", result2.UpdatedRecords)
	}

	// Preview with a record that has empty name (should be skipped)
	badData := &models.ExportData{
		Records: []models.Record{
			{ID: "bad1", Name: "", Date: base},
			{ID: "ok1", Name: "有效记录", Date: base},
		},
	}
	result3, err := db.PreviewImport(badData)
	if err != nil {
		t.Fatal(err)
	}
	if result3.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result3.Skipped)
	}
	if result3.Issues == nil || len(result3.Issues) != 1 {
		t.Errorf("Issues should have 1 entry, got %v", result3.Issues)
	}
}

func TestValidateImportRecord(t *testing.T) {
	// Empty name should be rejected
	issue, noDate := validateImportRecord(&models.Record{Name: ""}, 0)
	if issue == nil {
		t.Error("empty name should be rejected")
	}
	if noDate {
		t.Error("noDate should be false when record is rejected")
	}

	// Valid record with date
	issue, noDate = validateImportRecord(&models.Record{Name: "有效", Date: time.Now().Unix()}, 0)
	if issue != nil {
		t.Errorf("valid record should not be rejected: %v", issue)
	}
	if noDate {
		t.Error("record has date, noDate should be false")
	}

	// Valid record without date
	issue, noDate = validateImportRecord(&models.Record{Name: "无日期"}, 0)
	if issue != nil {
		t.Errorf("record without date but with name should not be rejected: %v", issue)
	}
	if !noDate {
		t.Error("record without date: noDate should be true")
	}
}
