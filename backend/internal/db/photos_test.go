package db

import (
	"testing"
)

func TestRecordPhotosCRUD(t *testing.T) {
	db := newTestDB(t)
	r := sampleRecord("ph-rec", 1755000000)
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}

	p1, err := db.AddRecordPhoto(r.ID, "covers/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := db.AddRecordPhoto(r.ID, "covers/b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if p1.Sort != 1 || p2.Sort != 2 {
		t.Fatalf("sort should auto-increment: %d %d", p1.Sort, p2.Sort)
	}

	photos, err := db.ListRecordPhotos(r.ID)
	if err != nil || len(photos) != 2 {
		t.Fatalf("list: %d photos err=%v", len(photos), err)
	}

	// 重排：b 在前
	if err := db.ReorderRecordPhotos(r.ID, []string{p2.ID, p1.ID}); err != nil {
		t.Fatal(err)
	}
	photos, _ = db.ListRecordPhotos(r.ID)
	if photos[0].FileName != "covers/b.jpg" || photos[1].FileName != "covers/a.jpg" {
		t.Fatalf("reorder failed: %+v", photos)
	}

	// 删除一张
	if err := db.DeleteRecordPhoto(r.ID, p1.ID); err != nil {
		t.Fatal(err)
	}
	photos, _ = db.ListRecordPhotos(r.ID)
	if len(photos) != 1 {
		t.Fatalf("after delete: %d", len(photos))
	}

	// PurgeRecord 级联清理照片关联
	if err := db.DeleteRecord(r.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.PurgeRecord(r.ID); err != nil {
		t.Fatal(err)
	}
	photos, _ = db.ListRecordPhotos(r.ID)
	if len(photos) != 0 {
		t.Fatalf("purge should cascade photos, got %d", len(photos))
	}
}

func TestExportImportRecordPhotos(t *testing.T) {
	db := newTestDB(t)
	r := sampleRecord("exp-ph", 1755000000)
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddRecordPhoto(r.ID, "covers/p1.jpg"); err != nil {
		t.Fatal(err)
	}

	data, err := db.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.RecordPhotos) != 1 {
		t.Fatalf("export should include record photos, got %d", len(data.RecordPhotos))
	}

	// 导入到另一个库：关联按 record_id 重建
	db2 := newTestDB(t)
	if _, err := db2.ImportData(data); err != nil {
		t.Fatal(err)
	}
	photos, err := db2.ListRecordPhotos(r.ID)
	if err != nil || len(photos) != 1 || photos[0].FileName != "covers/p1.jpg" {
		t.Fatalf("import photos: %d err=%v", len(photos), err)
	}
}
