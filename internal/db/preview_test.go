package db

import (
	"mujian/internal/models"
	"testing"
	"time"
)

func TestPreviewDeleteCategory(t *testing.T) {
	db := newTestDB(t)
	cat := &models.Category{Name: "昆曲"}
	if err := db.UpsertCategory(cat); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "pd1", Name: "演出1", CategoryName: "昆曲", CategoryNames: []string{"昆曲"}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "pd2", Name: "演出2", CategoryName: "昆曲", CategoryNames: []string{"昆曲"}, Date: base})

	n, err := db.PreviewDeleteCategory(cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("PreviewDeleteCategory = %d, want 2", n)
	}
	// Non-existent category
	n, err = db.PreviewDeleteCategory("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("PreviewDeleteCategory nonexistent = %d, want 0", n)
	}
}

func TestPreviewDeleteDrama(t *testing.T) {
	db := newTestDB(t)
	d, err := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "pdd1", Name: "演出1", DramaIDs: []string{d.ID}, Date: base})

	n, err := db.PreviewDeleteDrama(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("PreviewDeleteDrama = %d, want 1", n)
	}
}

func TestPreviewDeleteArtist(t *testing.T) {
	db := newTestDB(t)
	a, err := db.SaveArtist(models.Artist{Name: "张军"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "pda1", Name: "演出1", ArtistIDs: []string{a.ID}, Date: base})

	n, err := db.PreviewDeleteArtist(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("PreviewDeleteArtist = %d, want 1", n)
	}
}

func TestPreviewDeleteZhezi(t *testing.T) {
	db := newTestDB(t)
	z, err := db.CreateZhezi(models.Zhezi{Name: "惊梦", DramaID: "d-1"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "pdz1", Name: "演出1", ZheziIDs: []string{z.ID}, Date: base})

	n, err := db.PreviewDeleteZhezi(z.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("PreviewDeleteZhezi = %d, want 1", n)
	}
}

func TestCountDeletedRecords(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "cdr1", Name: "演出1", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "cdr2", Name: "演出2", Date: base})

	n, err := db.CountDeletedRecords()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("CountDeletedRecords = %d, want 0", n)
	}

	// Soft delete one record
	_ = db.SoftDeleteRecord("cdr1")
	n, err = db.CountDeletedRecords()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("CountDeletedRecords after delete = %d, want 1", n)
	}
}

func TestRecordExists(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "re1", Name: "存在", Date: base})

	exists, err := db.RecordExists("re1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("RecordExists should return true for existing record")
	}

	exists, err = db.RecordExists("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("RecordExists should return false for missing record")
	}

	// Soft-deleted record still exists (for purging)
	_ = db.SoftDeleteRecord("re1")
	exists, err = db.RecordExists("re1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("soft-deleted record should still exist for purging")
	}
}

func TestCountRecordsByIDs(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "cri1", Name: "A", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "cri2", Name: "B", Date: base})
	_ = db.SoftDeleteRecord("cri2")

	// Empty ids
	n, err := db.CountRecordsByIDs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("CountRecordsByIDs nil = %d, want 0", n)
	}

	// Mix of live and deleted
	n, err = db.CountRecordsByIDs([]string{"cri1", "cri2"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("CountRecordsByIDs = %d, want 1 (only live)", n)
	}
}
