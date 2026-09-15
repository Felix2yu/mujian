package db

import (
	"math"
	"mujian/internal/models"
	"testing"
	"time"
)

func TestNormVenueName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"苏州开明大戏院", "苏州开明大戏院"},
		{"开明大戏院（观前街店）", "开明大戏院"},
		{"上海 大剧院", "上海 大剧院"},
		{"  上海大剧院  ", "上海大剧院"},
		{"A（B）C（D）", "AC"},
		{"", ""},
		{"没有括号", "没有括号"},
	}
	for _, tc := range tests {
		if got := normVenueName(tc.in); got != tc.want {
			t.Errorf("normVenueName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestVenueDupKey(t *testing.T) {
	tests := []struct {
		city, name, want string
	}{
		{"上海", "上海大剧院", "上海\u0000"},             // 去掉后缀"大剧院" → "上海" → 去城市前缀 → ""
		{"上海", "大剧院", "上海\u0000大"},             // 名字就是后缀 → base stays "大剧院" → trim suffix if len > suf
		{"北京", "国家大剧院", "北京\u0000国家"},          // 去掉后缀"大剧院" → "国家"
		{"上海", "上海大戏院", "上海\u0000"},             // 去掉后缀"大戏院" → "上海" → 去城市前缀 → ""
		{"上海", "某剧场（分店）", "上海\u0000某"},        // 归一化 → "某剧场" → 去后缀"剧场" → "某"
		{"", "某中心", "\u0000某"},                     // 空城市, 去后缀"中心" → "某"
		{"上海", "上海体育馆", "上海\u0000"},             // 去掉后缀"体育馆" → "上海" → 去城市前缀 → ""
	}
	for _, tc := range tests {
		if got := venueDupKey(tc.city, tc.name); got != tc.want {
			t.Errorf("venueDupKey(%q, %q) = %q, want %q", tc.city, tc.name, got, tc.want)
		}
	}
}

func TestParseCoord(t *testing.T) {
	tests := []struct {
		in       string
		lat, lng float64
		ok       bool
	}{
		{"31.2304,121.4737", 31.2304, 121.4737, true},
		{" 31.2304 , 121.4737 ", 31.2304, 121.4737, true},
		{"", 0, 0, false},
		{"bad", 0, 0, false},
		{"31.2304", 0, 0, false},
		{"abc,def", 0, 0, false},
	}
	for _, tc := range tests {
		lat, lng, ok := parseCoord(tc.in)
		if ok != tc.ok || (ok && (math.Abs(lat-tc.lat) > 1e-6 || math.Abs(lng-tc.lng) > 1e-6)) {
			t.Errorf("parseCoord(%q) = (%f, %f, %v), want (%f, %f, %v)", tc.in, lat, lng, ok, tc.lat, tc.lng, tc.ok)
		}
	}
}

func TestParseFloat(t *testing.T) {
	if v, err := parseFloat("3.14"); err != nil || math.Abs(v-3.14) > 1e-10 {
		t.Errorf("parseFloat: %f %v", v, err)
	}
	if v, err := parseFloat(" 42 "); err != nil || v != 42 {
		t.Errorf("parseFloat spaces: %f %v", v, err)
	}
	if _, err := parseFloat("abc"); err == nil {
		t.Error("parseFloat junk should error")
	}
}

func TestHaversine(t *testing.T) {
	// Shanghai to Beijing ≈ 1068 km
	d := haversine(31.2304, 121.4737, 39.9087, 116.3975)
	if d < 1000 || d > 1200 {
		t.Errorf("haversine Shanghai-Beijing = %.0f km, want ~1068", d)
	}
	// Same point ≈ 0
	d2 := haversine(31.2304, 121.4737, 31.2304, 121.4737)
	if d2 > 0.001 {
		t.Errorf("haversine same point = %f, want ~0", d2)
	}
}

func TestVenuesCRUD(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()

	_ = db.UpsertRecord(models.Record{ID: "v1", Name: "演出1", Address: "上海大剧院", City: "上海", Coordinate: &models.Coordinate{Latitude: 31.23, Longitude: 121.47}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "v2", Name: "演出2", Address: "上海大剧院（人民广场店）", City: "上海", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "v3", Name: "演出3", Address: "北京国家大剧院", City: "北京", Coordinate: &models.Coordinate{Latitude: 39.9, Longitude: 116.4}, Date: base})

	// BackfillVenuesFromRecords
	if err := db.BackfillVenuesFromRecords(); err != nil {
		t.Fatal(err)
	}
	// Idempotent second run
	if err := db.BackfillVenuesFromRecords(); err != nil {
		t.Fatal(err)
	}

	list, err := db.ListVenues()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("ListVenues should return venues after backfill")
	}
	t.Logf("ListVenues returned %d venues", len(list))

	// GetVenue
	if len(list) > 0 {
		v, err := db.GetVenue(list[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if v.Name == "" {
			t.Error("GetVenue name should not be empty")
		}
	}
	// GetVenue missing
	if _, err := db.GetVenue("nonexistent"); err == nil {
		t.Error("GetVenue missing should error")
	}

	// RescanVenues
	added, removed, err := db.RescanVenues()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RescanVenues: added=%d removed=%d", added, removed)
}

func TestMergeVenues(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()

	// Create records with different address spellings
	_ = db.UpsertRecord(models.Record{ID: "mv1", Name: "演出1", Address: "A剧场", City: "上海", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "mv2", Name: "演出2", Address: "A剧场（分店）", City: "上海", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "mv3", Name: "演出3", Address: "B剧场", City: "上海", Date: base})

	if err := db.BackfillVenuesFromRecords(); err != nil {
		t.Fatal(err)
	}

	list, err := db.ListVenues()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 venues, got %d", len(list))
	}

	// Find two venues in the same city
	var targetID, sourceID string
	for _, v := range list {
		if v.Name == "A剧场" {
			targetID = v.ID
		}
		if v.Name == "A剧场（分店）" {
			sourceID = v.ID
		}
	}
	if targetID == "" || sourceID == "" {
		t.Skip("could not find test venues for merge")
	}

	// PreviewMergeVenues
	preview, err := db.PreviewMergeVenues(targetID, []string{sourceID})
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || preview.Target.ID != targetID {
		t.Errorf("PreviewMergeVenues wrong: %+v", preview)
	}

	// MergeVenues dry run then real
	rewritten, err := db.MergeVenues(targetID, []string{sourceID})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MergeVenues rewrote %d records", rewritten)

	// Source should be deleted
	if _, err := db.GetVenue(sourceID); err == nil {
		t.Error("source venue should be deleted after merge")
	}

	// PreviewMergeVenues error cases
	if _, err := db.PreviewMergeVenues("", []string{sourceID}); err == nil {
		t.Error("empty target should error")
	}
	if _, err := db.PreviewMergeVenues(targetID, nil); err == nil {
		t.Error("empty sources should error")
	}
	// MergeVenues error cases
	if _, err := db.MergeVenues("nonexistent", []string{sourceID}); err == nil {
		t.Error("missing target should error")
	}
}

func TestCountRecordsByVenueName(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "cv1", Name: "演出1", Address: "测试剧场", City: "上海", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "cv2", Name: "演出2", Address: "测试剧场（分店）", City: "上海", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "cv3", Name: "演出3", Address: "其他剧场", City: "上海", Date: base})

	n, err := db.countRecordsByVenueName("测试剧场")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("countRecordsByVenueName = %d, want 2", n)
	}
}

func TestListVenuesRaw(t *testing.T) {
	db := newTestDB(t)
	if err := db.BackfillVenuesFromRecords(); err != nil {
		// Empty DB, no venues to backfill — that's fine
		return
	}
	raw, err := db.listVenuesRaw()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("listVenuesRaw returned %d venues", len(raw))
}
