package db

import (
	"mujian/internal/models"
	"testing"
	"time"
)

func TestListDeletedRecords(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()

	// Create and soft-delete records
	_ = db.UpsertRecord(models.Record{ID: "ldr1", Name: "演出1", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "ldr2", Name: "演出2", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "ldr3", Name: "演出3", Date: base})
	_ = db.SoftDeleteRecord("ldr1")
	_ = db.SoftDeleteRecord("ldr2")

	// List with default limit
	deleted, err := db.ListDeletedRecords(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Errorf("ListDeletedRecords = %d, want 2", len(deleted))
	}

	// List with limit
	deleted2, err := db.ListDeletedRecords(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted2) != 1 {
		t.Errorf("ListDeletedRecords limit=1 = %d, want 1", len(deleted2))
	}

	// List with offset
	deleted3, err := db.ListDeletedRecords(10, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted3) != 1 {
		t.Errorf("ListDeletedRecords offset=1 = %d, want 1", len(deleted3))
	}

	// DeletedRecord should have the original record data
	if len(deleted) > 0 {
		if deleted[0].Name == "" {
			t.Error("DeletedRecord should have record data")
		}
		if deleted[0].DeletedAt == 0 {
			t.Error("DeletedRecord should have non-zero DeletedAt")
		}
	}

	// Empty trash
	db2 := newTestDB(t)
	deleted4, err := db2.ListDeletedRecords(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted4) != 0 {
		t.Errorf("empty trash should return empty slice, got %d", len(deleted4))
	}
}

func TestListDeletedRecordsOrdering(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertRecord(models.Record{ID: "ldo1", Name: "A", Date: time.Now().Unix()})
	_ = db.UpsertRecord(models.Record{ID: "ldo2", Name: "B", Date: time.Now().Unix()})
	// Delete both at the same time — ordering by deleted_at DESC should be
	// deterministic (secondary sort by rowid / insertion order is undefined,
	// so we just verify both are present).
	_ = db.SoftDeleteRecord("ldo1")
	_ = db.SoftDeleteRecord("ldo2")

	deleted, err := db.ListDeletedRecords(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Fatalf("expected 2, got %d", len(deleted))
	}
	ids := map[string]bool{deleted[0].ID: true, deleted[1].ID: true}
	if !ids["ldo1"] || !ids["ldo2"] {
		t.Errorf("both records should be present, got %v", ids)
	}
}
