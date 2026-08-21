package db

import (
	"encoding/json"
	"mujian/internal/models"
	"mujian/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func sampleRecord(id string, date int64) models.Record {
	return models.Record{
		ID: id, Name: "牡丹亭", Channel: "大麦", City: "上海",
		Address: "上海大剧院", Coordinate: &models.Coordinate{Latitude: 31.2, Longitude: 121.4},
		CoverFile: "covers/" + id + ".avif", CoverThumb: "covers/" + id + ".thumb.avif",
		CategoryName: "昆曲", ArtistNames: []string{"张军"}, Guest: []string{"小王"},
		Play: []string{"惊梦"}, DramaIDs: []string{"d-1"}, ZheziIDs: []string{"z-1"},
		TagIDs: []string{"tag-1"}, Date: date, DateText: "2026-08-22 19:30",
		Rating: 5, Seat: "A1", Friends: "老王", Company: "上昆", Remark: "很好",
		ActiveStatus: 1, Price: 280, PriceCurrency: "CNY",
		PayPrice: 180, PayPriceCurrency: "CNY", OtherCost: 20, OtherCostCurrency: "CNY",
	}
}

func TestNewMigratePingClose(t *testing.T) {
	db := newTestDB(t)
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	db.SetLocation(time.FixedZone("CST", 8*3600))
	if db.loc.String() != "CST" {
		t.Errorf("SetLocation failed: %v", db.loc)
	}
}

func TestRecordCRUD(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix()
	r := sampleRecord("rec-main", base)
	if err := db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	// Upsert with empty ID assigns one internally.
	if err := db.UpsertRecord(models.Record{Name: "无ID记录", Date: base}); err != nil {
		t.Fatalf("UpsertRecord no id: %v", err)
	}
	if all, _ := db.ListRecords(RecordFilter{}); len(all) != 2 {
		t.Errorf("expected 2 records after empty-id upsert, got %d", len(all))
	}

	got, err := db.GetRecord(r.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.Name != "牡丹亭" || got.Coordinate == nil || got.Coordinate.Latitude != 31.2 {
		t.Errorf("GetRecord fields wrong: %+v", got)
	}
	if len(got.ArtistNames) != 1 || got.ArtistNames[0] != "张军" {
		t.Errorf("arrays wrong: %+v", got.ArtistNames)
	}
	if _, err := db.GetRecord("nope"); err == nil {
		t.Error("GetRecord missing should error")
	}

	// Update preserves cover fields and syncs.
	req := models.RecordRequest{Name: "牡丹亭·改", City: "北京", Rating: 4}
	upd, err := db.UpdateRecord(r.ID, req)
	if err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}
	if upd.Name != "牡丹亭·改" || upd.CoverFile != r.CoverFile || upd.Cover != r.Cover {
		t.Errorf("update wrong: %+v", upd)
	}
	if _, err := db.UpdateRecord("missing", req); err == nil {
		t.Error("update missing should error")
	}

	// Filters.
	_ = db.UpsertRecord(sampleRecord("r-future", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC).Unix()))
	list, err := db.ListRecords(RecordFilter{})
	if err != nil || len(list) < 3 {
		t.Fatalf("ListRecords: %v len=%d", err, len(list))
	}
	if got, _ := db.ListRecords(RecordFilter{Query: "牡丹亭"}); len(got) < 2 {
		t.Error("query filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{Category: "昆曲"}); len(got) < 1 {
		t.Error("category filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{City: "北京"}); len(got) != 1 {
		t.Error("city filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{DramaID: "d-1"}); len(got) == 0 {
		t.Error("drama filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{ZheziID: "z-1"}); len(got) == 0 {
		t.Error("zhezi filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{Year: 2026, Month: 8}); len(got) == 0 {
		t.Error("year-month filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{Start: "2026-08-01", End: "2026-08-31"}); len(got) == 0 {
		t.Error("start/end filter failed")
	}
	if got, _ := db.ListRecords(RecordFilter{Start: "1789044479"}); got == nil {
		t.Error("unix start filter failed")
	}

	if err := db.DeleteRecord(r.ID); err != nil {
		t.Fatalf("DeleteRecord: %v", err)
	}
	if _, err := db.GetRecord(r.ID); err == nil {
		t.Error("deleted record should be gone")
	}
}

func TestCreateRecordDramaNames(t *testing.T) {
	db := newTestDB(t)
	d, err := db.SaveDrama(models.Drama{Name: "牡丹亭", CategoryName: "昆曲"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateRecord(models.RecordRequest{Name: "演出A", DramaIDs: []string{d.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Play) != 1 || created.Play[0] != "牡丹亭" {
		t.Errorf("CreateRecord should derive play from drama name: %+v", created.Play)
	}
}

func TestSyncAndAlignVenues(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	c1 := &models.Coordinate{Latitude: 1, Longitude: 2}
	_ = db.UpsertRecord(models.Record{ID: "a1", Name: "A1", Address: "某剧场", Coordinate: c1, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "a2", Name: "A2", Address: "某剧场", Date: base})
	_ = db.UpsertRecord(models.Record{ID: "b1", Name: "B1", Address: "另一地", Date: base})

	// SyncVenueCoordinates with empty addr/coord is a no-op.
	if n, _ := db.SyncVenueCoordinates("", c1, "a2"); n != 0 {
		t.Error("empty addr should be no-op")
	}
	if n, _ := db.SyncVenueCoordinates("某剧场", nil, "a2"); n != 0 {
		t.Error("nil coord should be no-op")
	}
	// Sync a2 from a1's coordinate.
	if n, err := db.SyncVenueCoordinates("某剧场", c1, "a1"); err != nil || n != 1 {
		t.Errorf("sync: n=%d err=%v", n, err)
	}

	res, err := db.AlignVenueCoordinates()
	if err != nil {
		t.Fatal(err)
	}
	if res.GroupsTotal != 2 || res.GroupsAligned != 1 {
		t.Errorf("align result: %+v", res)
	}
	// b1 group has no coords -> skipped; a-group already aligned.
	a2, _ := db.GetRecord("a2")
	if a2.Coordinate == nil || a2.Coordinate.Latitude != 1 {
		t.Errorf("a2 should have synced coordinate: %+v", a2.Coordinate)
	}
}

func TestBatchUpdateAndDelete(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	for _, id := range []string{"b1", "b2"} {
		_ = db.UpsertRecord(models.Record{ID: id, Name: "演出", Date: base, Play: []string{"原剧"}})
	}

	city := "杭州"
	rating := 3
	appendOp := &models.BatchArrayOp{Op: "append", Value: []string{"新剧"}}
	n, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs: []string{"b1", "b2"}, City: &city, Rating: &rating, Play: appendOp,
	})
	if err != nil || n != 2 {
		t.Fatalf("batch update: n=%d err=%v", n, err)
	}
	r1, _ := db.GetRecord("b1")
	if r1.City != "杭州" || r1.Rating != 3 {
		t.Errorf("scalar update wrong: %+v", r1)
	}
	if len(r1.Play) != 2 {
		t.Errorf("append op wrong: %+v", r1.Play)
	}

	// set / remove ops.
	setOp := &models.BatchArrayOp{Op: "set", Value: []string{"x"}}
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{IDs: []string{"b1"}, Play: setOp}); err != nil {
		t.Fatal(err)
	}
	rmOp := &models.BatchArrayOp{Op: "remove", Value: []string{"x"}}
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{IDs: []string{"b1"}, Play: rmOp}); err != nil {
		t.Fatal(err)
	}
	r1, _ = db.GetRecord("b1")
	if len(r1.Play) != 0 {
		t.Errorf("remove op wrong: %+v", r1.Play)
	}
	// Unknown op keeps existing.
	badOp := &models.BatchArrayOp{Op: "nope", Value: []string{"y"}}
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{IDs: []string{"b1"}, Play: badOp}); err != nil {
		t.Fatal(err)
	}
	// Empty ids no-op.
	if n, _ := db.BatchUpdateRecords(models.BatchUpdateParams{}); n != 0 {
		t.Error("empty ids should be no-op")
	}
	// Array op on missing record.
	_ = db.UpsertRecord(models.Record{ID: "b3", Name: "B3", Date: base})
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{IDs: []string{"b3", "missing"}, Guest: appendOp}); err != nil {
		t.Fatal(err)
	}

	if n, err := db.BatchDeleteRecords([]string{"b1", "b2"}); err != nil || n != 2 {
		t.Fatalf("batch delete: n=%d err=%v", n, err)
	}
	if n, _ := db.BatchDeleteRecords(nil); n != 0 {
		t.Error("empty delete should be no-op")
	}
}

func TestReorderDramasAndCategories(t *testing.T) {
	db := newTestDB(t)

	// Fresh DB: dramas keep creation order (sort_order = MAX+1 on create;
	// migrated databases keep sort_order 0, so they fall back to alphabetical).
	d1, _ := db.SaveDrama(models.Drama{Name: "霸王别姬"})
	d2, _ := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	d3, _ := db.SaveDrama(models.Drama{Name: "白蛇传"})
	list, _ := db.ListDramas()
	if len(list) != 3 || list[0].Name != "霸王别姬" || list[2].Name != "白蛇传" {
		t.Fatalf("fresh drama order should be creation order: %+v", list)
	}

	// Manual reorder (first = top).
	if err := db.ReorderDramas([]string{d2.ID, d1.ID, d3.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ = db.ListDramas()
	got := []string{list[0].ID, list[1].ID, list[2].ID}
	want := []string{d2.ID, d1.ID, d3.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drama reorder: got %v want %v", got, want)
		}
	}
	// GetDrama reflects sort order.
	d, _ := db.GetDrama(d2.ID)
	if d.SortOrder != 0 {
		t.Errorf("GetDrama sortOrder: %d", d.SortOrder)
	}

	// New drama appends after manually ordered ones.
	d4, _ := db.SaveDrama(models.Drama{Name: "长生殿"})
	if d4.SortOrder != 3 {
		t.Errorf("new drama should append with sort_order 3, got %d", d4.SortOrder)
	}
	list, _ = db.ListDramas()
	if list[len(list)-1].ID != d4.ID {
		t.Errorf("new drama should be last: %+v", list)
	}

	// Categories: fresh DB keeps creation order, then manual reorder applies.
	c1 := models.Category{Name: "昆曲"}
	c2 := models.Category{Name: "越剧"}
	c3 := models.Category{Name: "京剧"}
	if err := db.UpsertCategory(&c1); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCategory(&c2); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertCategory(&c3); err != nil {
		t.Fatal(err)
	}
	cats, _ := db.ListCategories()
	if len(cats) != 3 || cats[0].Name != "昆曲" || cats[2].Name != "京剧" {
		t.Fatalf("fresh category order should be creation order: %+v", cats)
	}
	if err := db.ReorderCategories([]string{c2.ID, c3.ID, c1.ID}); err != nil {
		t.Fatal(err)
	}
	cats, _ = db.ListCategories()
	if cats[0].Name != "越剧" || cats[1].Name != "京剧" || cats[2].Name != "昆曲" {
		t.Fatalf("category reorder failed: %+v", cats)
	}
	// New category appends at the end.
	c4 := models.Category{Name: "话剧"}
	if err := db.UpsertCategory(&c4); err != nil {
		t.Fatal(err)
	}
	if c4.SortOrder != 3 {
		t.Errorf("new category should append with sort_order 3, got %d", c4.SortOrder)
	}
	cats, _ = db.ListCategories()
	if cats[len(cats)-1].Name != "话剧" {
		t.Errorf("new category should be last: %+v", cats)
	}
}

func TestCategories(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertCategory(&models.Category{Name: "昆曲", ActiveIDs: []string{"a1"}}); err != nil {
		t.Fatal(err)
	}
	cats, err := db.ListCategories()
	if err != nil || len(cats) != 1 {
		t.Fatalf("ListCategories: %v %v", cats, err)
	}
	if cats[0].Name != "昆曲" || len(cats[0].ActiveIDs) != 1 {
		t.Errorf("category wrong: %+v", cats[0])
	}
	// Upsert with fixed id updates.
	if err := db.UpsertCategory(&models.Category{ID: cats[0].ID, Name: "越剧"}); err != nil {
		t.Fatal(err)
	}
	cats, _ = db.ListCategories()
	if cats[0].Name != "越剧" {
		t.Errorf("update category failed: %+v", cats[0])
	}
	if err := db.DeleteCategory(cats[0].ID); err != nil {
		t.Fatal(err)
	}
	if cats, _ := db.ListCategories(); len(cats) != 0 {
		t.Errorf("category not deleted: %v", cats)
	}
}

func TestDramasAndZhezis(t *testing.T) {
	db := newTestDB(t)
	d, err := db.SaveDrama(models.Drama{Name: "牡丹亭", CategoryName: "昆曲", Remark: "r"})
	if err != nil {
		t.Fatal(err)
	}
	// SaveDrama with existing id updates.
	if _, err := db.SaveDrama(models.Drama{ID: d.ID, Name: "牡丹亭改"}); err != nil {
		t.Fatal(err)
	}
	d2, err := db.GetDrama(d.ID)
	if err != nil || d2.Name != "牡丹亭改" {
		t.Fatalf("GetDrama: %+v %v", d2, err)
	}
	if _, err := db.GetDrama("missing"); err == nil {
		t.Error("GetDrama missing should error")
	}

	z, err := db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "惊梦", Aliases: []string{"惊梦/游园"}})
	if err != nil {
		t.Fatal(err)
	}
	if z.SortOrder != 1 {
		t.Errorf("first zhezi sort order should be 1: %d", z.SortOrder)
	}
	z2, err := db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "寻梦"})
	if err != nil {
		t.Fatal(err)
	}
	if z2.SortOrder != 2 {
		t.Errorf("second zhezi sort order should be 2: %d", z2.SortOrder)
	}

	zs, err := db.ListZhezisByDrama(d.ID)
	if err != nil || len(zs) != 2 {
		t.Fatalf("ListZhezisByDrama: %v %v", zs, err)
	}
	if err := db.ReorderZhezis(d.ID, []string{z2.ID, z.ID}); err != nil {
		t.Fatal(err)
	}
	zs, _ = db.ListZhezisByDrama(d.ID)
	if zs[0].ID != z2.ID {
		t.Errorf("reorder failed: %v", zs)
	}

	upd, err := db.UpdateZhezi(models.Zhezi{ID: z.ID, Name: "惊梦改", Aliases: []string{"别名"}})
	if err != nil || upd.Name != "惊梦改" {
		t.Fatalf("UpdateZhezi: %+v %v", upd, err)
	}

	list, err := db.ListDramas()
	if err != nil || len(list) != 1 || list[0].ZheziCount != 2 {
		t.Fatalf("ListDramas: %+v %v", list, err)
	}
	tree, err := db.ListDramaTree()
	if err != nil || len(tree) != 1 || len(tree[0].Zhezis) != 2 {
		t.Fatalf("ListDramaTree: %+v %v", tree, err)
	}

	// Link a record to the drama and check detail's Records.
	_ = db.UpsertRecord(models.Record{ID: "rr", Name: "演出", DramaIDs: []string{d.ID}, Date: time.Now().Unix()})
	detail, err := db.GetDramaDetail(d.ID)
	if err != nil || len(detail.Records) != 1 || len(detail.Zhezis) != 2 {
		t.Fatalf("GetDramaDetail: %+v %v", detail, err)
	}

	if err := db.DeleteZhezi(z.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.zheziByID(z.ID); err == nil {
		t.Error("deleted zhezi should be gone")
	}
	if err := db.DeleteDrama(d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetDrama(d.ID); err == nil {
		t.Error("deleted drama should be gone")
	}
	// DeleteDrama also removed its zhezis.
	if zs, _ := db.ListZhezisByDrama(d.ID); len(zs) != 0 {
		t.Errorf("zhezis should be cascaded: %v", zs)
	}
}

func TestBackfillDramasFromRecords(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "bf1", Name: "R1", Play: []string{"白蛇传", "  "}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "bf2", Name: "R2", Play: []string{"白蛇传"}, Date: base})

	if err := db.BackfillDramasFromRecords(); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListDramas()
	if len(list) != 1 || list[0].Name != "白蛇传" || list[0].RecordCount != 2 {
		t.Fatalf("backfill dramas: %+v", list)
	}
	// Idempotent second run.
	if err := db.BackfillDramasFromRecords(); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.ListDramas(); len(list) != 1 {
		t.Errorf("backfill should be idempotent: %v", list)
	}
	r1, _ := db.GetRecord("bf1")
	if len(r1.DramaIDs) != 1 {
		t.Errorf("record should link drama: %+v", r1.DramaIDs)
	}
}

func TestMeta(t *testing.T) {
	db := newTestDB(t)
	m, err := db.GetMeta()
	if err != nil {
		t.Fatal(err)
	}
	if string(m.Song) != "[]" || string(m.Tags) != "[]" {
		t.Errorf("default meta should be []: %+v", m)
	}
	set := &models.Meta{
		Song: json.RawMessage(`{"name":"x"}`), Tags: json.RawMessage(`["a"]`), WebdavConfig: json.RawMessage(`{"url":"u"}`),
	}
	if err := db.SetMeta(set); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetMeta()
	if !strings.Contains(string(got.Song), "x") || !strings.Contains(string(got.Tags), "a") {
		t.Errorf("meta round trip: %+v", got)
	}
	// Empty raw -> "[]".
	if err := db.SetMeta(&models.Meta{}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetMeta()
	if string(got.WebdavConfig) != "[]" {
		t.Errorf("empty raw should store []: %s", got.WebdavConfig)
	}
}

func TestStatsAndDashboard(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	recent := now.AddDate(0, -1, 0).Unix()
	old := now.AddDate(-3, 0, 0).Unix()
	_ = db.UpsertRecord(models.Record{ID: "s1", Name: "A", City: "上海", CategoryName: "昆曲", Date: recent, Rating: 5, PayPrice: 100})
	_ = db.UpsertRecord(models.Record{ID: "s2", Name: "B", City: "上海", Date: recent, Rating: 4, OtherCost: 50})
	_ = db.UpsertRecord(models.Record{ID: "s3", Name: "C", City: "北京", Date: old, Rating: 0})

	stats, err := db.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRecords != 3 || stats.TotalCities != 2 {
		t.Errorf("stats: %+v", stats)
	}
	if stats.TotalCost != 150 {
		t.Errorf("total cost: %+v", stats)
	}

	dash, err := db.GetDashboardStats()
	if err != nil {
		t.Fatal(err)
	}
	if dash.TotalRecords != 3 || len(dash.ByCategory) == 0 || len(dash.ByCity) != 2 {
		t.Errorf("dashboard: %+v", dash)
	}
	if len(dash.TopRated) != 2 || dash.TopRated[0].Rating != 5 {
		t.Errorf("top rated: %+v", dash.TopRated)
	}
	if len(dash.RecentRecords) != 3 {
		t.Errorf("recent: %d", len(dash.RecentRecords))
	}

	// Empty DB dashboard still returns initialized slices.
	db2 := newTestDB(t)
	dash2, _ := db2.GetDashboardStats()
	if dash2.ByMonth == nil || dash2.TopRated == nil || dash2.TotalRecords != 0 {
		t.Errorf("empty dashboard should have non-nil slices: %+v", dash2)
	}
}

func TestCalendarEvents(t *testing.T) {
	db := newTestDB(t)
	aug := time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix()
	sep := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = db.UpsertRecord(models.Record{ID: "c1", Name: "八月场", Date: aug, City: "上海", CoverFile: "covers/c1.avif", Rating: 5, CategoryName: "昆曲"})
	_ = db.UpsertRecord(models.Record{ID: "c2", Name: "九月场", Date: sep})

	evs, err := db.GetCalendarEvents(2026, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].ID != "c1" {
		t.Errorf("august events: %+v", evs)
	}
	sepEvs, _ := db.GetCalendarEvents(2026, 9)
	if len(sepEvs) != 1 || sepEvs[0].ID != "c2" {
		t.Errorf("september events: %+v", sepEvs)
	}
	empty, _ := db.GetCalendarEvents(2025, 1)
	if len(empty) != 0 {
		t.Errorf("no events expected: %+v", empty)
	}
}

func TestAutocompleteAndByField(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertRecord(models.Record{ID: "f1", Name: "白蛇传", City: "杭州", Date: time.Now().Unix()})
	if _, err := db.GetAutocomplete("bogus_field"); err == nil {
		t.Error("invalid field should error")
	}
	cities, err := db.GetAutocomplete("city")
	if err != nil || len(cities) != 1 || cities[0] != "杭州" {
		t.Fatalf("autocomplete: %v %v", cities, err)
	}
	if _, err := db.GetByField("bogus", "x"); err == nil {
		t.Error("GetByField invalid field should error")
	}
	recs, err := db.GetByField("name", "白蛇")
	if err != nil || len(recs) != 1 {
		t.Fatalf("GetByField: %v %v", recs, err)
	}
}

func TestExportImport(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertRecord(models.Record{ID: "e1", Name: "导出记录", Date: time.Now().Unix()})
	_ = db.UpsertCategory(&models.Category{ID: "cat1", Name: "昆曲"})
	_ = db.SetMeta(&models.Meta{Song: json.RawMessage(`{"s":1}`)})

	data, err := db.Export()
	if err != nil {
		t.Fatal(err)
	}
	if data.RecordCount != 1 || len(data.Records) != 1 || len(data.Categories) != 1 {
		t.Errorf("export: %+v", data)
	}
	if data.Source != "mujian" || data.CoverDir != "covers/" {
		t.Errorf("export meta fields: %+v", data)
	}

	path := filepath.Join(t.TempDir(), "export.json")
	if err := db.ExportToFile(path); err != nil {
		t.Fatal(err)
	}

	// Import into a fresh DB.
	db2 := newTestDB(t)
	res, err := db2.ImportData(data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Records != 1 || res.Categories != 1 {
		t.Errorf("import result: %+v", res)
	}
	r, err := db2.GetRecord("e1")
	if err != nil || r.Name != "导出记录" {
		t.Errorf("imported record: %+v %v", r, err)
	}
	m, _ := db2.GetMeta()
	if !strings.Contains(string(m.Song), "s") {
		t.Errorf("imported meta: %s", m.Song)
	}

	res2, err := db2.ImportFromFile(path)
	if err != nil || res2.Records != 1 {
		t.Fatalf("ImportFromFile: %+v %v", res2, err)
	}
	badPath := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(badPath, []byte("{bad"), 0600)
	if _, err := db2.ImportFromFile(badPath); err == nil {
		t.Error("invalid file should error")
	}
	if _, err := db2.ImportFromFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file should error")
	}
}

func TestCoversMetaAndDuplicates(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertCoverMeta("hash1", "covers/a.avif", ".avif", 1234); err != nil {
		t.Fatal(err)
	}
	if !db.CoverMetaExists("covers/a.avif") {
		t.Error("meta should exist")
	}
	c, err := db.GetCoverByHash("hash1")
	if err != nil || c.FileName != "covers/a.avif" || c.Size != 1234 {
		t.Fatalf("GetCoverByHash: %+v %v", c, err)
	}
	if sz, ok := db.CoverSize("covers/a.avif"); !ok || sz != 1234 {
		t.Errorf("CoverSize: %d %v", sz, ok)
	}
	if _, ok := db.CoverSize("nope"); ok {
		t.Error("CoverSize missing should be !ok")
	}
	// Upsert same file updates.
	if err := db.UpsertCoverMeta("hash2", "covers/a.avif", ".avif", 99); err != nil {
		t.Fatal(err)
	}
	if sz, _ := db.CoverSize("covers/a.avif"); sz != 99 {
		t.Errorf("upsert should update size: %d", sz)
	}
	if err := db.DeleteCoverMeta("covers/a.avif"); err != nil {
		t.Fatal(err)
	}
	if db.CoverMetaExists("covers/a.avif") {
		t.Error("meta should be deleted")
	}
}

func TestSyncCoversAndDuplicateGroups(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	store := storage.NewLocalStorage(dir, nil)
	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00}
	keyA, _, err := store.SaveCoverBytes(jpg, "")
	if err != nil {
		t.Fatal(err)
	}
	// A second, distinct file with identical content, written directly so both
	// exist under the store.
	keyC := "covers/dup2.jpg"
	if err := os.WriteFile(filepath.Join(dir, "covers", "dup2.jpg"), jpg, 0644); err != nil {
		t.Fatal(err)
	}

	base := time.Now().Unix()
	_ = db.UpsertRecord(models.Record{ID: "d1", Name: "R1", CoverFile: keyA, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "d2", Name: "R2", CoverFile: keyC, Date: base})

	added, err := db.SyncCovers(store)
	if err != nil {
		t.Fatal(err)
	}
	if added != 2 {
		t.Errorf("expected 2 new cover metas, got %d", added)
	}
	if !db.CoverMetaExists(keyA) || !db.CoverMetaExists(keyC) {
		t.Error("both cover metas should exist after sync")
	}
	// Second sync adds nothing.
	if added2, _ := db.SyncCovers(store); added2 != 0 {
		t.Errorf("second sync should add 0, got %d", added2)
	}

	// Both files have the same content hash -> one duplicate group.
	groups, err := db.GetDuplicateGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Count != 2 || groups[0].Hash != storage.HashBytes(jpg) {
		t.Errorf("dup groups: %+v", groups)
	}
	recs, err := db.GetRecordsByCoverHash(storage.HashBytes(jpg))
	if err != nil || len(recs) != 2 {
		t.Errorf("records by hash: %+v %v", recs, err)
	}

	// Repoint.
	if n, err := db.UpdateRecordsCoverFile([]string{"d1"}, keyC); err != nil || n != 1 {
		t.Fatalf("UpdateRecordsCoverFile: n=%d err=%v", n, err)
	}
	if n, _ := db.UpdateRecordsCoverFile(nil, keyC); n != 0 {
		t.Error("empty ids no-op")
	}
	if n, err := db.CountCoverRefs(keyC); err != nil || n != 2 {
		t.Errorf("CountCoverRefs: %d %v", n, err)
	}
	if err := db.RepointCoverRefs(keyA, keyC); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.CountCoverRefs(keyA); n != 0 {
		t.Errorf("old key should have 0 refs: %d", n)
	}
	files, err := db.GetRecordsByCoverFile(keyC)
	if err != nil || len(files) != 2 {
		t.Errorf("records by cover file: %+v %v", files, err)
	}
	if err := db.SetRecordThumb("d1", "covers/x.thumb.avif"); err != nil {
		t.Fatal(err)
	}
	r, _ := db.GetRecord("d1")
	if r.CoverThumb != "covers/x.thumb.avif" {
		t.Errorf("thumb not set: %+v", r.CoverThumb)
	}

	// Cover picker.
	refs, total, err := db.ListCoverPicker("", 10, 0)
	if err != nil || total != 1 || len(refs) != 1 || refs[0].RefCount != 2 {
		t.Errorf("picker: %+v total=%d err=%v", refs, total, err)
	}
	refsQ, _, _ := db.ListCoverPicker("R1", 10, 0)
	if len(refsQ) != 1 {
		t.Errorf("picker with q: %+v", refsQ)
	}
	if refs[0].Ext != "jpg" {
		t.Errorf("ext: %+v", refs[0].Ext)
	}
	files2, err := db.ListCoverFiles()
	if err != nil || len(files2) != 2 {
		t.Errorf("ListCoverFiles: %+v %v", files2, err)
	}
}

func TestHelpers(t *testing.T) {
	if got := marshalJSON([]string{"a"}); got != `["a"]` {
		t.Errorf("marshalJSON: %q", got)
	}
	if got := marshalJSON(make(chan int)); got != "[]" {
		t.Errorf("marshalJSON error should return []: %q", got)
	}
	if got := unmarshalStrings(""); len(got) != 0 {
		t.Errorf("unmarshalStrings empty: %v", got)
	}
	if got := unmarshalStrings("bad"); len(got) != 0 {
		t.Errorf("unmarshalStrings bad: %v", got)
	}
	if got := unmarshalStrings(`["a","b"]`); len(got) != 2 {
		t.Errorf("unmarshalStrings: %v", got)
	}
	if got := unmarshalCoordinate(""); got != nil {
		t.Error("unmarshalCoordinate empty should be nil")
	}
	if got := unmarshalCoordinate("null"); got != nil {
		t.Error("unmarshalCoordinate null should be nil")
	}
	if got := unmarshalCoordinate("bad"); got != nil {
		t.Error("unmarshalCoordinate bad should be nil")
	}
	c := unmarshalCoordinate(`{"latitude":1,"longitude":2}`)
	if c == nil || c.Latitude != 1 {
		t.Errorf("unmarshalCoordinate: %+v", c)
	}
	if id := newID(); len(id) < 20 {
		t.Errorf("newID too short: %q", id)
	}
	if got, ok := parseTimeArg("", time.UTC); ok || !got.IsZero() {
		t.Error("parseTimeArg empty should be !ok")
	}
	if got, ok := parseTimeArg("1789044479", time.UTC); !ok || got.Unix() != 1789044479 {
		t.Errorf("parseTimeArg unix: %v %v", got, ok)
	}
	if got, ok := parseTimeArg("2026-08-22", time.UTC); !ok || got.Year() != 2026 {
		t.Errorf("parseTimeArg date: %v %v", got, ok)
	}
	if got, ok := parseTimeArg("2026-08-22 19:30:00", time.UTC); !ok {
		t.Errorf("parseTimeArg datetime: %v %v", got, ok)
	}
	if got, ok := parseTimeArg("junk", time.UTC); ok {
		t.Errorf("parseTimeArg junk should fail: %v", got)
	}
	if n, err := parseInt64("42"); err != nil || n != 42 {
		t.Errorf("parseInt64: %d %v", n, err)
	}
	if _, err := parseInt64("x"); err == nil {
		t.Error("parseInt64 junk should error")
	}
	if got := applyArrayOp([]string{"a"}, nil); len(got) != 1 {
		t.Error("applyArrayOp nil op keeps existing")
	}
	if got := applyArrayOp([]string{"a"}, &models.BatchArrayOp{Op: "set", Value: []string{"b"}}); len(got) != 1 || got[0] != "b" {
		t.Errorf("set op: %v", got)
	}
	if got := applyArrayOp([]string{"a"}, &models.BatchArrayOp{Op: "append", Value: []string{"a", "b"}}); len(got) != 2 {
		t.Errorf("append op: %v", got)
	}
	if got := applyArrayOp([]string{"a", "b"}, &models.BatchArrayOp{Op: "remove", Value: []string{"a"}}); len(got) != 1 || got[0] != "b" {
		t.Errorf("remove op: %v", got)
	}
	if got := applyArrayOp([]string{"a"}, &models.BatchArrayOp{Op: "??", Value: []string{"b"}}); len(got) != 1 {
		t.Errorf("unknown op keeps existing: %v", got)
	}
	if got := extOf("covers/a.avif"); got != "avif" {
		t.Errorf("extOf: %q", got)
	}
	if got := extOf("noext"); got != "" {
		t.Errorf("extOf noext: %q", got)
	}
	// dramaNames with no ids returns nil.
	db := newTestDB(t)
	if got := db.dramaNames(nil); got != nil {
		t.Error("dramaNames nil should return nil")
	}
}
