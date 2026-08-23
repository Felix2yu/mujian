package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

func seedAnalytics(t *testing.T) *DB {
	t.Helper()
	db := newTestDB(t)
	day := func(d int) int64 { return time.Date(2026, 8, d, 19, 30, 0, 0, time.UTC).Unix() }
	recs := []models.Record{
		{ID: "v1", Name: "A", Address: "上海大剧院", City: "上海", Company: "上昆", Channel: "大麦", CategoryName: "昆剧", Coordinate: &models.Coordinate{Latitude: 31.2, Longitude: 121.4}, Date: day(1)},
		{ID: "v2", Name: "B", Address: "上海大剧院", City: "上海", Company: "上昆", Date: day(2)},
		// Same venue as v1/v2 but a different city -> two distinct cities in the group.
		{ID: "v3", Name: "C", Address: "上海大剧院", City: "南京", Company: "沪团", Date: day(3)},
		{ID: "v4", Name: "D", Address: "江苏紫金大剧院", City: "南京", Company: "省昆", Date: day(4)},
		// Empty address/company rows must be skipped by both queries.
		{ID: "v5", Name: "E", Address: "", City: "杭州", Date: day(5)},
	}
	for _, r := range recs {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestListVenueGroups(t *testing.T) {
	db := seedAnalytics(t)

	groups, err := db.ListVenueGroups("")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 address groups, got %d: %+v", len(groups), groups)
	}
	// Ordered by record count desc.
	if groups[0].Address != "上海大剧院" || groups[0].RecordCount != 3 {
		t.Errorf("top group: %+v", groups[0])
	}
	// has_coord only true when some record in the group carries coordinates.
	if !groups[0].HasCoord {
		t.Errorf("group with coordinate should report has_coord: %+v", groups[0])
	}
	if groups[1].HasCoord {
		t.Errorf("group without coordinates: %+v", groups[1])
	}
	// Distinct cities attached, ordered ascending.
	if len(groups[0].Cities) != 2 || groups[0].Cities[0] != "上海" || groups[0].Cities[1] != "南京" {
		t.Errorf("cities not attached correctly: %+v", groups[0].Cities)
	}

	filtered, err := db.ListVenueGroups("紫金")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Address != "江苏紫金大剧院" {
		t.Errorf("substring filter: %+v", filtered)
	}

	empty, err := db.ListVenueGroups("不存在的场馆")
	if err != nil || len(empty) != 0 {
		t.Errorf("no-match query: %+v %v", empty, err)
	}
}

func TestGetValueCounts(t *testing.T) {
	db := seedAnalytics(t)

	counts, err := db.GetValueCounts("company")
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 3 { // v5 has no company; blank values are skipped
		t.Fatalf("company counts: %+v", counts)
	}
	if counts[0].Value != "上昆" || counts[0].Count != 2 {
		t.Errorf("ordered by count desc: %+v", counts[0])
	}

	cities, err := db.GetValueCounts("city")
	if err != nil || len(cities) != 3 {
		t.Fatalf("city counts: %+v %v", cities, err)
	}

	if _, err := db.GetValueCounts("name"); err == nil {
		t.Error("non-whitelisted field should error")
	}
	if _, err := db.GetValueCounts(""); err == nil {
		t.Error("empty field should error")
	}
}
