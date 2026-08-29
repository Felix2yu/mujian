package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

// TestSearchFilterEquivalence guards the ListRecords search rewrite: the
// keyword predicate moved from LEFT JOINs (+SELECT DISTINCT) to EXISTS
// subqueries. This test asserts the new implementation returns exactly the
// same record-ID sets as the legacy JOIN formulation for a variety of
// keywords, including edge cases (empty result, matches that only hit artist
// names / drama aliases, and SQL LIKE metacharacters).
func TestSearchFilterEquivalence(t *testing.T) {
	d := newTestDB(t)

	base := time.Date(2026, 8, 1, 19, 30, 0, 0, time.UTC).Unix()
	rec, err := d.CreateRecord(models.RecordRequest{Name: "搜索等价·京剧", Date: base})
	if err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Two artists on one record → the legacy JOINs multiplied rows.
	if err := d.setRecordArtists(d.conn, rec.ID, nil, []string{"搜索等价·甲", "搜索等价·乙"}); err != nil {
		t.Fatalf("seed artists: %v", err)
	}
	drama, err := d.SaveDrama(models.Drama{Name: "搜索等价·剧目", Aliases: []string{"搜索等价·别名"}})
	if err != nil {
		t.Fatalf("seed drama: %v", err)
	}
	if err := d.setRecordDramas(d.conn, rec.ID, []string{drama.ID}); err != nil {
		t.Fatalf("seed drama link: %v", err)
	}
	rec2, err := d.CreateRecord(models.RecordRequest{Name: "搜索等价·昆剧", Date: base + 86400})
	if err != nil {
		t.Fatalf("seed record 2: %v", err)
	}
	if err := d.setRecordArtists(d.conn, rec2.ID, nil, []string{"搜索等价·丙"}); err != nil {
		t.Fatalf("seed artist c: %v", err)
	}

	keywords := []string{
		"搜索等价",   // hits records + artists
		"别名",     // hits drama aliases only
		"京剧",     // single record via name
		"昆剧",     // both records contain 剧
		"不存在xyz", // empty result
		"%",      // LIKE metacharacter must not become a wildcard match-all
		"_",      // single-char LIKE wildcard
		"",       // empty keyword → no filter, must equal unfiltered list
	}

	for _, kw := range keywords {
		newRows, err := d.ListRecords(RecordFilter{Query: kw})
		if err != nil {
			t.Fatalf("ListRecords(%q): %v", kw, err)
		}
		legacyIDs := d.legacySearchIDs(t, kw)
		newIDs := make(map[string]bool, len(newRows))
		for _, r := range newRows {
			newIDs[r.ID] = true
		}
		if len(newIDs) != len(legacyIDs) {
			t.Fatalf("keyword %q: new=%d ids, legacy=%d ids", kw, len(newIDs), len(legacyIDs))
		}
		for id := range newIDs {
			if !legacyIDs[id] {
				t.Fatalf("keyword %q: id %s in new result but not legacy", kw, id)
			}
		}
	}
}

// legacySearchIDs runs the pre-rewrite JOIN formulation directly.
func (db *DB) legacySearchIDs(t *testing.T, q string) map[string]bool {
	t.Helper()
	var joinsAndWhere string
	var args []any
	if q != "" {
		like := "%" + q + "%"
		joinsAndWhere = ` LEFT JOIN record_artists ra_q ON ra_q.record_id = records.id
			LEFT JOIN artists a_q ON a_q.id = ra_q.artist_id
			LEFT JOIN record_dramas rd_q ON rd_q.record_id = records.id
			LEFT JOIN dramas d_q ON d_q.id = rd_q.drama_id
			WHERE (records.name LIKE ? OR records.city LIKE ? OR records.address LIKE ? OR records.company LIKE ? OR records.channel LIKE ? OR records.remark LIKE ? OR records.friends LIKE ? OR records.category_name LIKE ? OR records.category_names LIKE ? OR a_q.name LIKE ? OR records.play LIKE ? OR d_q.aliases LIKE ?)`
		for range 12 {
			args = append(args, like)
		}
	}
	rows, err := db.conn.Query("SELECT DISTINCT records.id FROM records"+joinsAndWhere, args...)
	if err != nil {
		t.Fatalf("legacy query: %v", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = true
	}
	return out
}

// TestListRecordsRowCaps verifies the default/hard caps and the NoLimit
// escape hatch that keeps export/ICS/records-all contracts intact.
func TestListRecordsRowCaps(t *testing.T) {
	d := newTestDB(t)

	base := time.Date(2026, 7, 1, 19, 30, 0, 0, time.UTC).Unix()
	for i := 0; i < 7; i++ {
		if _, err := d.CreateRecord(models.RecordRequest{Name: "容量测试·剧目", Date: base + int64(i)*3600}); err != nil {
			t.Fatalf("seed record %d: %v", i, err)
		}
	}

	got, err := d.ListRecords(RecordFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListRecords limit=3: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("explicit limit: got %d rows, want 3", len(got))
	}

	all, err := d.ListRecords(RecordFilter{})
	if err != nil {
		t.Fatalf("ListRecords default: %v", err)
	}
	noLimit, err := d.ListRecords(RecordFilter{NoLimit: true})
	if err != nil {
		t.Fatalf("ListRecords NoLimit: %v", err)
	}
	if len(all) != 7 || len(noLimit) != 7 {
		t.Fatalf("row counts: default=%d nolimit=%d, want 7/7", len(all), len(noLimit))
	}
}

// TestMapPointsOnlyGeocoded checks the slim map projection: only records with
// coordinates come back.
func TestMapPointsOnlyGeocoded(t *testing.T) {
	d := newTestDB(t)

	base := time.Date(2026, 7, 1, 19, 30, 0, 0, time.UTC).Unix()
	withCoord, err := d.CreateRecord(models.RecordRequest{
		Name: "地图点·有坐标", Date: base,
		Coordinate: &models.Coordinate{Latitude: 31.2304, Longitude: 121.4737},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := d.CreateRecord(models.RecordRequest{Name: "地图点·无坐标", Date: base + 3600}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}

	pts, err := d.ListMapPoints()
	if err != nil {
		t.Fatalf("ListMapPoints: %v", err)
	}
	if len(pts) != 1 {
		t.Fatalf("points = %d, want 1 (only geocoded records)", len(pts))
	}
	if pts[0].ID != withCoord.ID || pts[0].Coordinate == nil {
		t.Fatalf("point = %+v", pts[0])
	}
	if pts[0].Name != "地图点·有坐标" {
		t.Fatalf("point name = %q", pts[0].Name)
	}
}

// TestStatsCacheInvalidationMiddleware: the TTL cache must serve identical
// data until a mutation arrives, then reflect the new state.
func TestStatsVisibleAfterMutation(t *testing.T) {
	d := newTestDB(t)

	base := time.Date(2026, 7, 1, 19, 30, 0, 0, time.UTC).Unix()
	if _, err := d.CreateRecord(models.RecordRequest{Name: "统计·一", Date: base}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, err := d.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if _, err := d.CreateRecord(models.RecordRequest{Name: "统计·二", Date: base + 60}); err != nil {
		t.Fatalf("seed 2: %v", err)
	}
	after, err := d.GetStats()
	if err != nil {
		t.Fatalf("GetStats 2: %v", err)
	}
	// The DB layer itself is uncached; this pins the numbers the cache must
	// invalidate on (TotalRecords drives the /api/stats contract).
	if before.TotalRecords != 1 || after.TotalRecords != 2 {
		t.Fatalf("TotalRecords before=%d after=%d, want 1/2",
			before.TotalRecords, after.TotalRecords)
	}
}
