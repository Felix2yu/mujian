package db

import (
	"mujian/internal/models"
	"testing"
	"time"
)

func TestSetRecordWatched(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "rw1", Name: "演出1", Date: base})

	// Set watched = true
	if err := db.SetRecordWatched("rw1", true); err != nil {
		t.Fatal(err)
	}
	r, _ := db.GetRecord("rw1")
	if !r.Watched {
		t.Error("Watched should be true after SetRecordWatched(true)")
	}

	// Set watched = false
	if err := db.SetRecordWatched("rw1", false); err != nil {
		t.Fatal(err)
	}
	r, _ = db.GetRecord("rw1")
	if r.Watched {
		t.Error("Watched should be false after SetRecordWatched(false)")
	}

	// Non-existent record is a no-op (no error)
	if err := db.SetRecordWatched("nonexistent", true); err != nil {
		t.Error("SetRecordWatched on missing record should not error")
	}
}

func TestNormalizeEntityName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"张三", "张三"},
		{"  张三  ", "张三"},
		{"张 三", "张三"},           // ideographic space
		{"ABC", "abc"},              // lowercase
		{"ＡＢＣ", "abc"},           // full-width → ASCII
		{"abc  123", "abc123"},     // spaces removed
		{"", ""},
		{"  ", ""},
		{"张三 ABC", "张三abc"},     // mixed
		{"\t\n", ""},               // whitespace
	}
	for _, tc := range tests {
		if got := NormalizeEntityName(tc.in); got != tc.want {
			t.Errorf("NormalizeEntityName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFindCategoryByName(t *testing.T) {
	db := newTestDB(t)
	cat := &models.Category{Name: "昆曲"}
	if err := db.UpsertCategory(cat); err != nil {
		t.Fatal(err)
	}

	// Exact match
	id, err := db.FindCategoryByName("昆曲")
	if err != nil {
		t.Fatal(err)
	}
	if id != cat.ID {
		t.Errorf("FindCategoryByName = %q, want %q", id, cat.ID)
	}

	// Normalized match (spaces, full-width)
	id2, err := db.FindCategoryByName("  昆  曲  ")
	if err != nil {
		t.Fatal(err)
	}
	if id2 != cat.ID {
		t.Errorf("FindCategoryByName normalized = %q, want %q", id2, cat.ID)
	}

	// Not found
	id3, err := db.FindCategoryByName("不存在")
	if err != nil {
		t.Fatal(err)
	}
	if id3 != "" {
		t.Errorf("FindCategoryByName not found = %q, want empty", id3)
	}

	// Empty name
	id4, err := db.FindCategoryByName("")
	if err != nil {
		t.Fatal(err)
	}
	if id4 != "" {
		t.Errorf("FindCategoryByName empty = %q, want empty", id4)
	}
}

func TestCountRecordsByCategory(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "crc1", Name: "A", CategoryName: "昆曲", CategoryNames: []string{"昆曲"}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "crc2", Name: "B", CategoryName: "昆曲", CategoryNames: []string{"昆曲", "京剧"}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "crc3", Name: "C", CategoryName: "京剧", CategoryNames: []string{"京剧"}, Date: base})

	n, err := db.CountRecordsByCategory("昆曲")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("CountRecordsByCategory(昆曲) = %d, want 2", n)
	}

	n2, err := db.CountRecordsByCategory("京剧")
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 2 {
		t.Errorf("CountRecordsByCategory(京剧) = %d, want 2", n2)
	}

	n3, err := db.CountRecordsByCategory("不存在")
	if err != nil {
		t.Fatal(err)
	}
	if n3 != 0 {
		t.Errorf("CountRecordsByCategory(不存在) = %d, want 0", n3)
	}
}

func TestRetiredDramaNames(t *testing.T) {
	db := newTestDB(t)

	// No retired names yet
	names, err := db.RetiredDramaNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("RetiredDramaNames should be empty, got %v", names)
	}
}

func TestRecordArtistIDs(t *testing.T) {
	db := newTestDB(t)
	a1, _ := db.SaveArtist(models.Artist{Name: "演员1"})
	a2, _ := db.SaveArtist(models.Artist{Name: "演员2"})
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "raid1", Name: "演出1", ArtistIDs: []string{a1.ID, a2.ID}, Date: base})

	ids, err := db.recordArtistIDs(db.conn, "raid1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("recordArtistIDs = %v, want 2 ids", ids)
	}

	// Empty for record with no artists
	_ = db.UpsertRecord(models.Record{ID: "raid2", Name: "演出2", Date: base})
	ids2, err := db.recordArtistIDs(db.conn, "raid2")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids2) != 0 {
		t.Errorf("recordArtistIDs empty = %v, want empty", ids2)
	}
}
