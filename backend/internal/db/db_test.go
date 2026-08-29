package db

import (
	"context"
	"encoding/json"
	"fmt"
	"mujian/internal/models"
	"mujian/internal/storage"
	"os"
	"path/filepath"
	"slices"
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

	// Exercise every scalar field + every array op so BatchUpdateRecords'
	// optional branches are fully covered.
	db.UpsertRecord(models.Record{ID: "bf", Name: "全字段", Date: base, Play: []string{"p0"}, Guest: []string{"g0"}})
	cat, rate, act, addr, chanS, comp, fr, remark, seat := "昆曲", 5, 1, "剧场", "大麦", "院团", "友人", "注", "A1"
	price, pp, oc := 200.0, 180.0, 20.0
	pcur, ppcur, ocur := "CNY", "CNY", "CNY"
	setD := &models.BatchArrayOp{Op: "set", Value: []string{"d1"}}
	zset := &models.BatchArrayOp{Op: "set", Value: []string{"z1"}}
	appOp := &models.BatchArrayOp{Op: "append", Value: []string{"p1"}}
	appG := &models.BatchArrayOp{Op: "append", Value: []string{"g1"}}
	appA := &models.BatchArrayOp{Op: "append", Value: []string{"张军"}}
	rmT := &models.BatchArrayOp{Op: "remove", Value: []string{"nope"}}
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs: []string{"bf"}, CategoryName: &cat, Rating: &rate, ActiveStatus: &act,
		City: &addr, Address: &addr, Channel: &chanS, Company: &comp, Friends: &fr,
		Remark: &remark, Seat: &seat, Price: &price, PriceCurrency: &pcur,
		PayPrice: &pp, PayPriceCurrency: &ppcur, OtherCost: &oc, OtherCostCurrency: &ocur,
		DramaIDs: setD, ZheziIDs: zset, Play: appOp, Guest: appG, ArtistNames: appA, TagIDs: rmT,
	}); err != nil {
		t.Fatal(err)
	}
	// Second pass: remove/append on arrays + set artist.
	rmP := &models.BatchArrayOp{Op: "remove", Value: []string{"p0"}}
	rmG := &models.BatchArrayOp{Op: "remove", Value: []string{"g0"}}
	setA := &models.BatchArrayOp{Op: "set", Value: []string{"单雯"}}
	appD := &models.BatchArrayOp{Op: "append", Value: []string{"d2"}}
	appZ := &models.BatchArrayOp{Op: "append", Value: []string{"z2"}}
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs: []string{"bf"}, Play: rmP, Guest: rmG, ArtistNames: setA, DramaIDs: appD, ZheziIDs: appZ,
	}); err != nil {
		t.Fatal(err)
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
	// active_ids is now a derived field (computed from records), so it is no
	// longer persisted; ListCategories returns an empty slice for it.
	if cats[0].Name != "昆曲" || len(cats[0].ActiveIDs) != 0 {
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
	_ = db.UpsertRecord(models.Record{ID: "f2", Name: "雷峰塔", City: "杭州", Company: "上海昆剧团, 苏州昆剧院", Date: time.Now().Unix()})
	_ = db.UpsertRecord(models.Record{ID: "f3", Name: "占花魁", City: "杭州", Company: "苏州昆剧院，上海评弹团", Date: time.Now().Unix()})
	companies, err := db.GetAutocomplete("company")
	if err != nil {
		t.Fatalf("autocomplete company: %v", err)
	}
	want := []string{"上海昆剧团", "上海评弹团", "苏州昆剧院"}
	if !slices.Equal(companies, want) {
		t.Fatalf("company autocomplete = %v, want %v", companies, want)
	}
	if _, err := db.GetByField("bogus", "x"); err == nil {
		t.Error("GetByField invalid field should error")
	}
	recs, err := db.GetByField("name", "白蛇")
	if err != nil || len(recs) != 1 {
		t.Fatalf("GetByField: %v %v", recs, err)
	}
}

func TestSetRecordArtistsNewNameInsideTx(t *testing.T) {
	// 回归：ImportData 事务内解析未知演员名时，旧实现经由连接池
	// INSERT artists —— 写锁被本事务持有，池连接等满 busy_timeout 后
	// 以 SQLITE_BUSY 失败（导入报 "database is locked"）。现在解析与
	// 链接写入共用同一个 exec 上下文。
	db := newTestDB(t)

	tx, err := db.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// 先在事务连接上制造一次写，确保 RESERVED 写锁已被本事务持有。
	if _, err := tx.Exec("INSERT INTO records (id) VALUES ('tx-rec')"); err != nil {
		t.Fatal(err)
	}
	if err := db.setRecordArtists(tx, "tx-rec", nil, []string{"事务新演员"}); err != nil {
		t.Fatalf("setRecordArtists inside tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var aid string
	if err := db.conn.QueryRow(
		"SELECT artist_id FROM record_artists WHERE record_id = 'tx-rec'",
	).Scan(&aid); err != nil {
		t.Fatalf("link missing: %v", err)
	}
	var name string
	if err := db.conn.QueryRow("SELECT name FROM artists WHERE id = ?", aid).Scan(&name); err != nil || name != "事务新演员" {
		t.Fatalf("artist created: %q %v", name, err)
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

// countDramaLinks returns the number of rows in the record_dramas relation
// table for a given record.
func (db *DB) countDramaLinks(t *testing.T, recordID string) int {
	t.Helper()
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM record_dramas WHERE record_id = ?", recordID).Scan(&n); err != nil {
		t.Fatalf("count links: %v", err)
	}
	return n
}

func TestRecordDramasRelation(t *testing.T) {
	db := newTestDB(t)
	d, err := db.SaveDrama(models.Drama{Name: "长生殿"})
	if err != nil {
		t.Fatal(err)
	}
	z := time.Now().Unix()

	// Upsert writes links into the relation table.
	_ = db.UpsertRecord(models.Record{ID: "r1", Name: "演出1", DramaIDs: []string{d.ID, "d-x"}, Date: z})
	if n := db.countDramaLinks(t, "r1"); n != 2 {
		t.Fatalf("expected 2 drama links, got %d", n)
	}
	// Read path backfills DramaIDs from the relation table.
	r1, err := db.GetRecord("r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.DramaIDs) != 2 {
		t.Errorf("GetRecord should backfill DramaIDs: %+v", r1.DramaIDs)
	}
	// Re-upsert with fewer links should replace, not append.
	_ = db.UpsertRecord(models.Record{ID: "r1", Name: "演出1", DramaIDs: []string{d.ID}, Date: z})
	if n := db.countDramaLinks(t, "r1"); n != 1 {
		t.Fatalf("re-upsert should replace links, got %d", n)
	}

	// ListRecords(DramaID) uses the indexed relation table.
	matched, err := db.ListRecords(RecordFilter{DramaID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].ID != "r1" {
		t.Errorf("ListRecords(DramaID) mismatch: %+v", matched)
	}

	// GetDrama.record_count counts via the relation table.
	got, err := db.GetDrama(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RecordCount != 1 {
		t.Errorf("record_count should be 1, got %d", got.RecordCount)
	}

	// DeleteDrama cascades to the relation table.
	if err := db.DeleteDrama(d.ID); err != nil {
		t.Fatal(err)
	}
	if n := db.countDramaLinks(t, "r1"); n != 0 {
		t.Errorf("DeleteDrama should cascade links, got %d", n)
	}
}

func TestMigrateDramaRelationsIdempotent(t *testing.T) {
	db := newTestDB(t)
	d, _ := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	// Simulate an old database: write the legacy drama_ids JSON column only
	// (bypassing the relation table).
	if _, err := db.conn.Exec("INSERT INTO records (id, name, drama_ids, date) VALUES (?, ?, ?, ?)",
		"old1", "老演出", marshalJSON([]string{d.ID}), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	// Running the migration should expand the JSON into the relation table.
	if err := db.migrateDramaRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countDramaLinks(t, "old1"); n != 1 {
		t.Fatalf("migration should expand legacy column, got %d", n)
	}
	// Re-running must be idempotent (no duplicate rows thanks to PK).
	if err := db.migrateDramaRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countDramaLinks(t, "old1"); n != 1 {
		t.Fatalf("migration must be idempotent, got %d", n)
	}
	// And the read path sees the link after migration.
	r, err := db.GetRecord("old1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.DramaIDs) != 1 {
		t.Errorf("post-migration read should backfill: %+v", r.DramaIDs)
	}
}

func TestExportImportRoundTripsDramaLinks(t *testing.T) {
	db := newTestDB(t)
	d, _ := db.SaveDrama(models.Drama{Name: "桃花扇"})
	_ = db.UpsertRecord(models.Record{ID: "ex1", Name: "演出", DramaIDs: []string{d.ID}, Date: time.Now().Unix()})

	data, err := db.Export()
	if err != nil {
		t.Fatal(err)
	}
	// The exported JSON must still carry drama_ids for backward compatibility.
	var found bool
	for _, r := range data.Records {
		if r.ID == "ex1" {
			if len(r.DramaIDs) != 1 {
				t.Fatalf("export should include DramaIDs: %+v", r.DramaIDs)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("export missing record")
	}

	// Import into a fresh db: links should be rebuilt from DramaIDs.
	db2 := newTestDB(t)
	if _, err := db2.ImportData(data); err != nil {
		t.Fatal(err)
	}
	if n := db2.countDramaLinks(t, "ex1"); n != 1 {
		t.Fatalf("import should rebuild relation table, got %d", n)
	}
	r, err := db2.GetRecord("ex1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.DramaIDs) != 1 {
		t.Errorf("imported record should link drama: %+v", r.DramaIDs)
	}
}

// countZheziLinks returns the number of rows in the record_zhezis relation
// table for a given record.
func (db *DB) countZheziLinks(t *testing.T, recordID string) int {
	t.Helper()
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM record_zhezis WHERE record_id = ?", recordID).Scan(&n); err != nil {
		t.Fatalf("count zhezi links: %v", err)
	}
	return n
}

func (db *DB) countArtistLinks(t *testing.T, recordID string) int {
	t.Helper()
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM record_artists WHERE record_id = ?", recordID).Scan(&n); err != nil {
		t.Fatalf("count artist links: %v", err)
	}
	return n
}

func TestRecordZhezisRelation(t *testing.T) {
	db := newTestDB(t)
	z, err := db.CreateZhezi(models.Zhezi{Name: "游园", DramaID: "d-1"})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Unix()

	// Upsert writes links into the relation table.
	_ = db.UpsertRecord(models.Record{ID: "r1", Name: "演出1", ZheziIDs: []string{z.ID, "z-x"}, Date: ts})
	if n := db.countZheziLinks(t, "r1"); n != 2 {
		t.Fatalf("expected 2 zhezi links, got %d", n)
	}
	// Read path backfills ZheziIDs from the relation table.
	r1, err := db.GetRecord("r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.ZheziIDs) != 2 {
		t.Errorf("GetRecord should backfill ZheziIDs: %+v", r1.ZheziIDs)
	}
	// Re-upsert with fewer links should replace, not append.
	_ = db.UpsertRecord(models.Record{ID: "r1", Name: "演出1", ZheziIDs: []string{z.ID}, Date: ts})
	if n := db.countZheziLinks(t, "r1"); n != 1 {
		t.Fatalf("re-upsert should replace zhezi links, got %d", n)
	}

	// ListRecords(ZheziID) uses the indexed relation table.
	matched, err := db.ListRecords(RecordFilter{ZheziID: z.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].ID != "r1" {
		t.Errorf("ListRecords(ZheziID) mismatch: %+v", matched)
	}

	// DeleteZhezi cascades to the relation table.
	if err := db.DeleteZhezi(z.ID); err != nil {
		t.Fatal(err)
	}
	if n := db.countZheziLinks(t, "r1"); n != 0 {
		t.Errorf("DeleteZhezi should cascade links, got %d", n)
	}
}

// TestRecordDramaRelation mirrors TestRecordZhezisRelation for the drama side:
// it guards the record_dramas relation table keeps the same write/backfill/
// replace/cascade semantics as zhezi and artist relations.
func TestRecordDramaRelation(t *testing.T) {
	db := newTestDB(t)
	d, err := db.SaveDrama(models.Drama{Name: "长生殿", CategoryName: "昆曲"})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Unix()

	// Upsert writes links into the relation table.
	_ = db.UpsertRecord(models.Record{ID: "rd1", Name: "演出D", DramaIDs: []string{d.ID, "d-x"}, Date: ts})
	if n := db.countDramaLinks(t, "rd1"); n != 2 {
		t.Fatalf("expected 2 drama links, got %d", n)
	}
	// Read path backfills DramaIDs from the relation table.
	r1, err := db.GetRecord("rd1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.DramaIDs) != 2 {
		t.Errorf("GetRecord should backfill DramaIDs: %+v", r1.DramaIDs)
	}
	// Re-upsert with fewer links should replace, not append.
	_ = db.UpsertRecord(models.Record{ID: "rd1", Name: "演出D", DramaIDs: []string{d.ID}, Date: ts})
	if n := db.countDramaLinks(t, "rd1"); n != 1 {
		t.Fatalf("re-upsert should replace drama links, got %d", n)
	}

	// ListRecords(DramaID) uses the indexed relation table.
	matched, err := db.ListRecords(RecordFilter{DramaID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].ID != "rd1" {
		t.Errorf("ListRecords(DramaID) mismatch: %+v", matched)
	}

	// DeleteRecord cascades to the relation table (FK constraints disabled).
	if err := db.DeleteRecord("rd1"); err != nil {
		t.Fatal(err)
	}
	if n := db.countDramaLinks(t, "rd1"); n != 0 {
		t.Errorf("DeleteRecord should cascade drama links, got %d", n)
	}
}

func TestMigrateZheziRelationsIdempotent(t *testing.T) {
	db := newTestDB(t)
	z, _ := db.CreateZhezi(models.Zhezi{Name: "惊梦", DramaID: "d-1"})
	// Simulate an old database: write the legacy zhezi_ids JSON column only
	// (bypassing the relation table).
	if _, err := db.conn.Exec("INSERT INTO records (id, name, zhezi_ids, date) VALUES (?, ?, ?, ?)",
		"old1", "老数据", marshalJSON([]string{z.ID}), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	// Re-run the migration manually (it also runs on New()).
	if err := db.migrateZheziRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countZheziLinks(t, "old1"); n != 1 {
		t.Fatalf("migration should expand legacy zhezi_ids, got %d", n)
	}
	// Idempotent: running again must not duplicate.
	if err := db.migrateZheziRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countZheziLinks(t, "old1"); n != 1 {
		t.Fatalf("migration should be idempotent, got %d", n)
	}
}

func TestArtists(t *testing.T) {
	db := newTestDB(t)

	// Save a new artist; id is assigned internally.
	saved, err := db.SaveArtist(models.Artist{Name: "张军", Aliases: []string{"张三"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" {
		t.Fatal("SaveArtist should assign an id")
	}
	if got, _ := db.GetArtist(saved.ID); got.Name != "张军" || len(got.Aliases) != 1 {
		t.Errorf("GetArtist wrong: %+v", got)
	}
	// Update existing (upsert by id).
	if _, err := db.SaveArtist(models.Artist{ID: saved.ID, Name: "张军(改)", Bio: "昆曲演员"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetArtist(saved.ID); got.Name != "张军(改)" || got.Bio != "昆曲演员" {
		t.Errorf("artist update wrong: %+v", got)
	}

	// Linking a record to the artist via UpsertRecord, then reverse lookup.
	r := sampleRecord("rec-artist", time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix())
	r.ArtistNames = []string{"张军(改)"}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}
	detail, err := db.GetArtistDetail(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Records) != 1 || detail.Records[0].ID != r.ID {
		t.Errorf("artist reverse lookup wrong: %+v", detail.Records)
	}
	if saved2, _ := db.GetArtist(saved.ID); saved2.RecordCount != 1 {
		t.Errorf("record_count should be 1, got %d", saved2.RecordCount)
	}

	// Second artist + reorder.
	other, _ := db.SaveArtist(models.Artist{Name: "单雯"})
	if err := db.ReorderArtists([]string{other.ID, saved.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := db.ListArtists()
	if len(list) != 2 || list[0].ID != other.ID {
		t.Errorf("reorder wrong: %+v", list)
	}

	// Delete cascades the relation row.
	if err := db.DeleteArtist(saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetArtist(saved.ID); err == nil {
		t.Error("deleted artist should be gone")
	}
	if n := db.countArtistLinks(t, r.ID); n != 0 {
		t.Errorf("delete should cascade record_artists, got %d", n)
	}
}

// TestRecordArtistIDsRelation covers the record-form path that links a record
// to artists by *entity id* (artist_ids picked from the tree), not by name.
// Regression test for the bug where RecordRequest had no ArtistIDs field and
// the links were silently dropped on create/update.
func TestRecordArtistIDsRelation(t *testing.T) {
	db := newTestDB(t)
	a1, _ := db.SaveArtist(models.Artist{Name: "单雯"})
	a2, _ := db.SaveArtist(models.Artist{Name: "施夏明"})

	r := sampleRecord("rec-by-id", time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix())
	r.ArtistIDs = []string{a1.ID, a2.ID}
	r.ArtistNames = nil // pure id path, no legacy names
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}
	// Both ids should be linked in the relation table.
	if n := db.countArtistLinks(t, r.ID); n != 2 {
		t.Fatalf("expected 2 artist links via ids, got %d", n)
	}
	// Read path backfills ArtistIDs.
	got, err := db.GetRecord(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ArtistIDs) != 2 {
		t.Errorf("GetRecord should backfill ArtistIDs: %+v", got.ArtistIDs)
	}
	// Reverse lookup: each artist's recordCount is 1 and the record appears.
	for _, aid := range []string{a1.ID, a2.ID} {
		detail, err := db.GetArtistDetail(aid)
		if err != nil {
			t.Fatal(err)
		}
		if detail.RecordCount != 1 {
			t.Errorf("artist %s recordCount should be 1, got %d", aid, detail.RecordCount)
		}
		if len(detail.Records) != 1 || detail.Records[0].ID != r.ID {
			t.Errorf("artist %s reverse lookup wrong: %+v", aid, detail.Records)
		}
	}
	// ListRecords(ArtistID) uses the indexed relation table.
	for _, aid := range []string{a1.ID, a2.ID} {
		matched, err := db.ListRecords(RecordFilter{ArtistID: aid})
		if err != nil {
			t.Fatal(err)
		}
		if len(matched) != 1 || matched[0].ID != r.ID {
			t.Errorf("ListRecords(ArtistID=%s) mismatch: %+v", aid, matched)
		}
	}
	// Re-upsert with fewer ids should REPLACE, not append (drop a2).
	r.ArtistIDs = []string{a1.ID}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, r.ID); n != 1 {
		t.Fatalf("re-upsert should replace artist links, got %d", n)
	}
	if detail, _ := db.GetArtistDetail(a2.ID); detail.RecordCount != 0 {
		t.Errorf("dropped artist recordCount should be 0, got %d", detail.RecordCount)
	}
}

// TestDeleteRecordCascadesRelations covers DeleteRecord clearing the
// record_artists / record_dramas / record_zhezis relation rows so that
// artist/drama/zhezi record counts stay accurate after a record is removed
// (the schema has FK constraints disabled, so this is manual cascade).
func TestDeleteRecordCascadesRelations(t *testing.T) {
	db := newTestDB(t)
	artist, _ := db.SaveArtist(models.Artist{Name: "黎安"})
	drama, _ := db.SaveDrama(models.Drama{Name: "长生殿", CategoryName: "昆曲"})
	zhezi, _ := db.CreateZhezi(models.Zhezi{Name: "小宴", DramaID: drama.ID})

	r := sampleRecord("rec-cascade", time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix())
	r.ArtistIDs = []string{artist.ID}
	r.ArtistNames = nil // use the id path only; sampleRecord seeds a legacy name
	r.DramaIDs = []string{drama.ID}
	r.ZheziIDs = []string{zhezi.ID}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, r.ID); n != 1 {
		t.Fatalf("artist link not written: %d", n)
	}

	// Sanity: artist count is 1 before delete.
	if detail, _ := db.GetArtistDetail(artist.ID); detail.RecordCount != 1 {
		t.Fatalf("pre-delete recordCount should be 1, got %d", detail.RecordCount)
	}

	// Delete the record; all three relation tables must be cleared.
	if err := db.DeleteRecord(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetRecord(r.ID); err == nil {
		t.Error("deleted record should be gone")
	}
	if n := db.countArtistLinks(t, r.ID); n != 0 {
		t.Errorf("DeleteRecord should clear record_artists, got %d", n)
	}
	if n := db.countDramaLinks(t, r.ID); n != 0 {
		t.Errorf("DeleteRecord should clear record_dramas, got %d", n)
	}
	if n := db.countZheziLinks(t, r.ID); n != 0 {
		t.Errorf("DeleteRecord should clear record_zhezis, got %d", n)
	}
	// And the artist recordCount must drop back to 0.
	if detail, _ := db.GetArtistDetail(artist.ID); detail.RecordCount != 0 {
		t.Errorf("post-delete artist recordCount should be 0, got %d", detail.RecordCount)
	}
}

func TestArtistResolutionAndMigration(t *testing.T) {
	db := newTestDB(t)

	// resolveArtistByName creates a new artist when the name is unknown.
	id1, err := db.resolveArtistByName(db.conn, " 新演员 ")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == "" {
		t.Fatal("resolve should create artist")
	}
	// Exact match on second call.
	id1b, _ := db.resolveArtistByName(db.conn, "新演员")
	if id1b != id1 {
		t.Fatalf("exact match should return same id: %s vs %s", id1, id1b)
	}
	// Empty name errors.
	if _, err := db.resolveArtistByName(db.conn, "  "); err == nil {
		t.Error("empty name should error")
	}
	// Alias match: create artist with alias, then resolve by alias.
	if _, err := db.SaveArtist(models.Artist{Name: "老演员", Aliases: []string{"小李"}}); err != nil {
		t.Fatal(err)
	}
	if aid, err := db.resolveArtistByName(db.conn, "小李"); err != nil || aid == "" {
		t.Fatalf("alias match failed: %v %s", err, aid)
	}

	// migrateArtistRelations: seed a legacy record with artist_names JSON, run
	// the migration, then verify the relation table is populated + idempotent.
	db.conn.Exec("INSERT INTO records (id, name, artist_names, date) VALUES ('leg1','老数据',?,?)",
		marshalJSON([]string{"新演员", "小李"}), time.Now().Unix())
	if err := db.migrateArtistRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, "leg1"); n != 2 {
		t.Fatalf("migration should link 2 artists, got %d", n)
	}
	if err := db.migrateArtistRelations(); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, "leg1"); n != 2 {
		t.Fatalf("migration should be idempotent, got %d", n)
	}

	// ListArtistTree returns the lightweight picker list.
	tree, err := db.ListArtistTree()
	if err != nil || len(tree) == 0 {
		t.Fatalf("ListArtistTree: %v %v", tree, err)
	}

	// BulkUpsertRecords: empty slice is a no-op; non-empty inserts + links.
	if err := db.BulkUpsertRecords(nil); err != nil {
		t.Fatal(err)
	}
	if err := db.BulkUpsertRecords([]models.Record{
		{ID: "b1", Name: "批量A", Date: time.Now().Unix(), ArtistNames: []string{"新演员"}},
		{ID: "b2", Name: "批量B", Date: time.Now().Unix()},
	}); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, "b1"); n != 1 {
		t.Fatalf("bulk upsert should link artist, got %d", n)
	}

	// setRecordArtists with empty ids+names clears existing links.
	if err := db.setRecordArtists(db.conn, "b1", nil, nil); err != nil {
		t.Fatal(err)
	}
	if n := db.countArtistLinks(t, "b1"); n != 0 {
		t.Fatalf("empty names should clear links, got %d", n)
	}
}

func TestMultiCategory(t *testing.T) {
	db := newTestDB(t)

	// Upsert with multiple categories: scalar primary stays in sync.
	r := sampleRecord("m1", 1000)
	r.CategoryNames = []string{"昆剧", "苏剧"}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	got, err := db.GetRecord("m1")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.CategoryName != "昆剧" || len(got.CategoryNames) != 2 || got.CategoryNames[1] != "苏剧" {
		t.Fatalf("multi-category readback: %+v / %+v", got.CategoryName, got.CategoryNames)
	}

	// Scalar-only write promotes into a single-element array.
	r2 := sampleRecord("m2", 2000)
	r2.CategoryNames = nil
	r2.CategoryName = "越剧"
	if err := db.UpsertRecord(r2); err != nil {
		t.Fatalf("UpsertRecord m2: %v", err)
	}
	got2, _ := db.GetRecord("m2")
	if len(got2.CategoryNames) != 1 || got2.CategoryNames[0] != "越剧" {
		t.Fatalf("scalar fallback: %+v", got2.CategoryNames)
	}

	// Category filter matches any element.
	for _, cat := range []string{"昆剧", "苏剧"} {
		rs, err := db.ListRecords(RecordFilter{Category: cat})
		if err != nil {
			t.Fatalf("ListRecords(%s): %v", cat, err)
		}
		if len(rs) != 1 || rs[0].ID != "m1" {
			t.Fatalf("filter by %s: got %d records", cat, len(rs))
		}
	}

	// Batch append keeps the primary category pinned to element 0.
	n, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:           []string{"m1"},
		CategoryNames: &models.BatchArrayOp{Op: "append", Value: []string{"评弹"}},
	})
	if err != nil || n == 0 {
		t.Fatalf("batch append: n=%d err=%v", n, err)
	}
	got3, _ := db.GetRecord("m1")
	if got3.CategoryName != "昆剧" || len(got3.CategoryNames) != 3 {
		t.Fatalf("after batch append: %+v / %+v", got3.CategoryName, got3.CategoryNames)
	}

	// 未手动设置且未关联演出时为空（SaveDrama 传入的分类即手动值，此处不传）。
	d, err := db.SaveDrama(models.Drama{Name: "白蛇传"})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	if d.CategoryName != "" || len(d.CategoryNames) != 0 {
		t.Fatalf("drama categories should be empty before any performance: %+v / %+v", d.CategoryName, d.CategoryNames)
	}

	// 关联两条不同剧种的演出后，GetDrama 聚合出剧种（按使用次数降序）。
	mustUpsertRec(t, db, "w1", "白蛇传", []string{"昆曲"})
	mustUpsertRec(t, db, "w2", "白蛇传", []string{"京剧"})
	mustUpsertRec(t, db, "w3", "白蛇传", []string{"京剧"})
	gotD, err := db.GetDrama(d.ID)
	if err != nil {
		t.Fatalf("GetDrama: %v", err)
	}
	if gotD.CategoryName != "京剧" || len(gotD.CategoryNames) != 2 || gotD.CategoryNames[0] != "京剧" || gotD.CategoryNames[1] != "昆曲" {
		t.Fatalf("aggregated categories: %+v / %+v", gotD.CategoryName, gotD.CategoryNames)
	}
	list, err := db.ListDramas()
	if err != nil {
		t.Fatalf("ListDramas: %v", err)
	}
	var listed *models.Drama
	for i := range list {
		if list[i].ID == d.ID {
			listed = &list[i]
		}
	}
	if listed == nil || listed.CategoryName != "京剧" || len(listed.CategoryNames) != 2 {
		t.Fatalf("list aggregated categories: %+v", listed)
	}

	// 手动覆盖优先于聚合（修正拼盘演出污染）；清空后回退聚合。
	if _, err := db.SaveDrama(models.Drama{ID: d.ID, Name: "白蛇传", CategoryNames: []string{"滑稽戏"}}); err != nil {
		t.Fatalf("manual override: %v", err)
	}
	gotD, _ = db.GetDrama(d.ID)
	if gotD.CategoryName != "滑稽戏" || len(gotD.CategoryNames) != 1 {
		t.Fatalf("manual override should win: %+v / %+v", gotD.CategoryName, gotD.CategoryNames)
	}
	if _, err := db.SaveDrama(models.Drama{ID: d.ID, Name: "白蛇传", CategoryNames: []string{}}); err != nil {
		t.Fatalf("clear override: %v", err)
	}
	gotD, _ = db.GetDrama(d.ID)
	if gotD.CategoryName != "京剧" || len(gotD.CategoryNames) != 2 {
		t.Fatalf("cleared override should fall back to aggregation: %+v / %+v", gotD.CategoryName, gotD.CategoryNames)
	}

	// 拼盘噪声过滤：一条演出同时关联多个剧目时，其剧种不应污染任一剧目的聚合，
	// 剧目只应反映「被单独演出」时的真实剧种。
	a, err := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	if err != nil {
		t.Fatalf("SaveDrama A: %v", err)
	}
	b, err := db.SaveDrama(models.Drama{Name: "长生殿"})
	if err != nil {
		t.Fatalf("SaveDrama B: %v", err)
	}
	// 拼盘演出：同场演《牡丹亭》《长生殿》，剧种分别为 昆曲 / 京昆
	if err := db.UpsertRecord(models.Record{
		ID: "p1", Name: "昆曲专场", DateText: "2026-02-01 19:30",
		CategoryName: "昆曲", CategoryNames: []string{"昆曲", "京昆"},
		DramaIDs: []string{a.ID, b.ID},
	}); err != nil {
		t.Fatalf("UpsertRecord 拼盘: %v", err)
	}
	// 《牡丹亭》另有一场单独演出（昆曲），验证单独演出的剧种仍被统计
	if err := db.UpsertRecord(models.Record{
		ID: "s1", Name: "牡丹亭独演", DateText: "2026-02-02 19:30",
		CategoryName: "昆曲", CategoryNames: []string{"昆曲"},
		DramaIDs: []string{a.ID},
	}); err != nil {
		t.Fatalf("UpsertRecord 独演: %v", err)
	}
	gotA, err := db.GetDrama(a.ID)
	if err != nil {
		t.Fatalf("GetDrama A: %v", err)
	}
	for _, c := range gotA.CategoryNames {
		if c == "京昆" {
			t.Fatalf("拼盘噪声未过滤，牡丹亭被污染: %+v", gotA.CategoryNames)
		}
	}
	if len(gotA.CategoryNames) != 1 || gotA.CategoryNames[0] != "昆曲" {
		t.Fatalf("牡丹亭应仅聚合单独演出的昆曲: %+v", gotA.CategoryNames)
	}
	gotB, err := db.GetDrama(b.ID)
	if err != nil {
		t.Fatalf("GetDrama B: %v", err)
	}
	if len(gotB.CategoryNames) != 0 {
		t.Fatalf("长生殿仅出现在拼盘中，聚合应为空: %+v", gotB.CategoryNames)
	}
}

// mustUpsertRec writes a minimal performance record with the given categories
// linked to a drama of the same name (created on demand).
func mustUpsertRec(t *testing.T, db *DB, id, dramaName string, cats []string) {
	t.Helper()
	ds, _ := db.ListDramas()
	var did string
	for _, x := range ds {
		if x.Name == dramaName {
			did = x.ID
			break
		}
	}
	if did == "" {
		nd, err := db.SaveDrama(models.Drama{Name: dramaName})
		if err != nil {
			t.Fatalf("SaveDrama(%s): %v", dramaName, err)
		}
		did = nd.ID
	}
	r := models.Record{ID: id, Name: dramaName, DateText: "2026-01-01 19:30", CategoryName: cats[0], CategoryNames: cats, DramaIDs: []string{did}}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord %s: %v", id, err)
	}
}

func TestBatchUpdateNameDateTimeCoordinateMoney(t *testing.T) {
	db := newTestDB(t)
	r := sampleRecord("n1", 1000)
	r.CategoryNames = []string{"昆剧", "苏剧"}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}

	n, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:        []string{"n1"},
		Name:       strPtr("改名后的演出"),
		DateText:   strPtr("2026-08-23 19:30"),
		Coordinate: &models.Coordinate{Latitude: 31.3, Longitude: 120.6},
		Price:      fltPtr(180),
	})
	if err != nil || n == 0 {
		t.Fatalf("batch update: n=%d err=%v", n, err)
	}
	got, _ := db.GetRecord("n1")
	if got.Name != "改名后的演出" {
		t.Fatalf("name: %q", got.Name)
	}
	wantT, ok := parseDateText("2026-08-23 19:30", db.loc)
	if !ok {
		t.Fatal("parseDateText should accept the canonical format")
	}
	if got.DateText != "2026-08-23 19:30" || got.Date != wantT.Unix() {
		t.Fatalf("date: %q %d", got.DateText, got.Date)
	}
	if got.Coordinate == nil || got.Coordinate.Latitude != 31.3 {
		t.Fatalf("coordinate: %+v", got.Coordinate)
	}
	if got.Price != 180 {
		t.Fatalf("price: %v", got.Price)
	}

	// 无效日期文本不改动任何字段（no-op，仅计入请求数）
	if _, err = db.BatchUpdateRecords(models.BatchUpdateParams{IDs: []string{"n1"}, DateText: strPtr("not-a-date")}); err != nil {
		t.Fatalf("invalid date should not error: %v", err)
	}
	got2, _ := db.GetRecord("n1")
	if got2.DateText != "2026-08-23 19:30" {
		t.Fatalf("date changed by invalid input: %q", got2.DateText)
	}

	// tagIds 数组操作
	n, err = db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:    []string{"n1"},
		TagIDs: &models.BatchArrayOp{Op: "append", Value: []string{"小剧场"}},
	})
	if err != nil || n == 0 {
		t.Fatalf("tag append: n=%d err=%v", n, err)
	}
	got3, _ := db.GetRecord("n1")
	// sampleRecord 自带 tag-1；append 后应为两个元素（去重集合，顺序不保证）
	if len(got3.TagIDs) != 2 {
		t.Fatalf("tag_ids: %+v", got3.TagIDs)
	}
}

func strPtr(s string) *string   { return &s }
func fltPtr(f float64) *float64 { return &f }

// 批量数组操作必须同步关联表（record_dramas / record_artists），
// 否则读取回填看不到变更——水浒记合并时曾因此丢失关联。
func TestBatchArrayOpsSyncRelations(t *testing.T) {
	db := newTestDB(t)
	d1, _ := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	d2, _ := db.SaveDrama(models.Drama{Name: "长生殿"})
	a1, _ := db.SaveArtist(models.Artist{Name: "张军"})

	r := sampleRecord("rel1", 1000)
	r.DramaIDs = []string{d1.ID}
	r.ArtistNames = []string{"张军"}
	r.TagIDs = []string{}
	if err := db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}

	// append 另一剧目：读取（关联表回填）应立即可见。
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:      []string{"rel1"},
		DramaIDs: &models.BatchArrayOp{Op: "append", Value: []string{d2.ID}},
	}); err != nil {
		t.Fatalf("batch append drama: %v", err)
	}
	got, err := db.GetRecord("rel1")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	has := func(ids []string, want string) bool {
		for _, x := range ids {
			if x == want {
				return true
			}
		}
		return false
	}
	if !has(got.DramaIDs, d1.ID) || !has(got.DramaIDs, d2.ID) {
		t.Fatalf("drama_ids after append: %+v", got.DramaIDs)
	}

	// remove 后关联表同步删除。
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:      []string{"rel1"},
		DramaIDs: &models.BatchArrayOp{Op: "remove", Value: []string{d1.ID}},
	}); err != nil {
		t.Fatalf("batch remove drama: %v", err)
	}
	got, _ = db.GetRecord("rel1")
	if has(got.DramaIDs, d1.ID) || !has(got.DramaIDs, d2.ID) {
		t.Fatalf("drama_ids after remove: %+v", got.DramaIDs)
	}

	// artist_names 变化同步 record_artists（演员详情页反向查询依赖它）。
	if _, err := db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:         []string{"rel1"},
		ArtistNames: &models.BatchArrayOp{Op: "append", Value: []string{"龚隐雷"}},
	}); err != nil {
		t.Fatalf("batch append artist: %v", err)
	}
	detail, err := db.GetArtistDetail(a1.ID)
	if err != nil {
		t.Fatalf("GetArtistDetail: %v", err)
	}
	found := false
	for _, rr := range detail.Records {
		if rr.ID == "rel1" {
			found = true
		}
	}
	if !found {
		t.Fatal("artist detail should still include rel1 after append")
	}
}

func TestListRecordsPagination(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC)

	// 插入 25 条不同日期的记录，按 date DESC 排序后应是 r25..r0
	for i := 0; i < 25; i++ {
		date := base.AddDate(0, 0, i).Unix()
		r := sampleRecord(fmt.Sprintf("page-rec-%02d", i), date)
		r.Name = fmt.Sprintf("第 %d 场演出", i)
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	// CountRecords 无筛选 = 25
	total, err := db.CountRecords(RecordFilter{})
	if err != nil {
		t.Fatalf("CountRecords: %v", err)
	}
	if total != 25 {
		t.Fatalf("CountRecords 无筛选 = %d, want 25", total)
	}

	// CountRecords 带筛选
	totalSh, _ := db.CountRecords(RecordFilter{City: "上海"})
	if totalSh != 25 {
		t.Fatalf("CountRecords city=上海 = %d, want 25", totalSh)
	}

	// Limit=0、Offset=0 等价于全量（不截断）
	all, err := db.ListRecords(RecordFilter{})
	if err != nil {
		t.Fatalf("ListRecords all: %v", err)
	}
	if len(all) != 25 {
		t.Fatalf("ListRecords 无 limit/offset = %d, want 25", len(all))
	}

	// Limit=10 → 返回 10 条
	page10, err := db.ListRecords(RecordFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListRecords limit=10: %v", err)
	}
	if len(page10) != 10 {
		t.Fatalf("len(limit=10) = %d, want 10", len(page10))
	}
	// 按日期倒序，第一条应是最后插入的（日期最新）
	if page10[0].ID != "page-rec-24" {
		t.Errorf("limit=10 首条 = %s, want page-rec-24", page10[0].ID)
	}
	if page10[9].ID != "page-rec-15" {
		t.Errorf("limit=10 末条 = %s, want page-rec-15", page10[9].ID)
	}

	// Offset=10, Limit=10 → 翻到第二页
	page2, err := db.ListRecords(RecordFilter{Limit: 10, Offset: 10})
	if err != nil {
		t.Fatalf("ListRecords limit=10 offset=10: %v", err)
	}
	if len(page2) != 10 {
		t.Fatalf("len(page2) = %d, want 10", len(page2))
	}
	if page2[0].ID != "page-rec-14" {
		t.Errorf("page2 首条 = %s, want page-rec-14", page2[0].ID)
	}
	if page2[9].ID != "page-rec-05" {
		t.Errorf("page2 末条 = %s, want page-rec-05", page2[9].ID)
	}

	// Offset=20, Limit=10 → 最后只剩 5 条
	page3, err := db.ListRecords(RecordFilter{Limit: 10, Offset: 20})
	if err != nil {
		t.Fatalf("ListRecords limit=10 offset=20: %v", err)
	}
	if len(page3) != 5 {
		t.Fatalf("len(page3) = %d, want 5", len(page3))
	}
	if page3[0].ID != "page-rec-04" {
		t.Errorf("page3 首条 = %s, want page-rec-04", page3[0].ID)
	}
	if page3[4].ID != "page-rec-00" {
		t.Errorf("page3 末条 = %s, want page-rec-00", page3[4].ID)
	}

	// Offset 超过 total → 空列表
	empty, err := db.ListRecords(RecordFilter{Limit: 10, Offset: 100})
	if err != nil {
		t.Fatalf("ListRecords offset=100: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("offset 超出应返回空列表, got %d", len(empty))
	}

	// CountRecords 带筛选 + ListRecords 带筛选 + limit/offset
	// 改几个城市做过滤测试
	_, _ = db.UpdateRecord("page-rec-00", models.RecordRequest{City: "北京"})
	_, _ = db.UpdateRecord("page-rec-01", models.RecordRequest{City: "北京"})
	totalBj, _ := db.CountRecords(RecordFilter{City: "北京"})
	if totalBj != 2 {
		t.Fatalf("CountRecords city=北京 = %d, want 2", totalBj)
	}
	bjPage, err := db.ListRecords(RecordFilter{City: "北京", Limit: 1})
	if err != nil {
		t.Fatalf("ListRecords 筛选+limit: %v", err)
	}
	if len(bjPage) != 1 {
		t.Fatalf("筛选+limit len = %d, want 1", len(bjPage))
	}
}

func TestGetAnalytics(t *testing.T) {
	db := newTestDB(t)

	// 空库也能返回合法结构体（不是 nil，各 slice 已初始化）
	a, err := db.GetAnalytics()
	if err != nil {
		t.Fatalf("empty GetAnalytics: %v", err)
	}
	if a == nil {
		t.Fatal("empty GetAnalytics 返回 nil")
	}
	if a.Overview.TotalRecords != 0 {
		t.Errorf("empty TotalRecords = %d, want 0", a.Overview.TotalRecords)
	}
	if len(a.Trends) != 24 {
		t.Errorf("empty Trends len = %d, want 24", len(a.Trends))
	}
	if len(a.RatingDist) != 0 {
		t.Errorf("empty RatingDist should be empty slice when no rated records, got %d", len(a.RatingDist))
	}
	if a.Rewatch == nil {
		t.Error("empty Rewatch should not be nil")
	}

	// 先插入 dramas / zhezis / artists 实体（让 UpsertRecord 能关联到它们）
	dramas := []string{"牡丹亭", "长生殿", "西厢记", "玉簪记", "赵氏孤儿", "雷峰塔"}
	for _, name := range dramas {
		id := fmt.Sprintf("d-%s", name)
		db.conn.Exec("INSERT OR IGNORE INTO dramas (id, name, aliases) VALUES (?, ?, '[]')", id, name)
	}
	zhezis := []string{"惊梦", "寻梦", "叫画", "琴挑", "断桥", "哭塔"}
	for _, name := range zhezis {
		id := fmt.Sprintf("z-%s", name)
		db.conn.Exec("INSERT OR IGNORE INTO zhezis (id, name, drama_id) VALUES (?, ?, 'd-牡丹亭')", id, name)
	}
	artists := []string{"张军", "王芳", "沈昳丽", "黎安", "余彬", "倪徐浩", "罗晨雪", "袁国良"}
	for _, name := range artists {
		id := fmt.Sprintf("a-%s", name)
		db.conn.Exec("INSERT OR IGNORE INTO artists (id, name) VALUES (?, ?)", id, name)
	}

	// 插入多样化演出（跨城市、评分、价格、时间、演员、剧目）
	base := time.Now()
	cities := []string{"上海", "北京", "广州", "杭州"}
	// 用真实存在的 drama/zhezi/artist id
	dramaIDs := []string{"d-牡丹亭", "d-长生殿", "d-西厢记", "d-玉簪记", "d-赵氏孤儿", "d-雷峰塔"}
	zheziIDs := []string{"z-惊梦", "z-寻梦", "z-叫画", "z-琴挑", "z-断桥", "z-哭塔"}
	artistIDs := []string{"a-张军", "a-王芳", "a-沈昳丽", "a-黎安", "a-余彬", "a-倪徐浩", "a-罗晨雪", "a-袁国良"}
	for i := 0; i < 60; i++ {
		// 分布在过去 24 个月内
		date := base.AddDate(0, -int(i%24), -i%28)
		r := models.Record{
			ID:            fmt.Sprintf("ana-%03d", i),
			Name:          dramas[i%len(dramas)],
			Channel:       []string{"大麦", "永乐票务", "微店", "其它"}[i%4],
			City:          cities[i%len(cities)],
			Address:       fmt.Sprintf("剧院 %02d", i%15),
			Coordinate:    &models.Coordinate{Latitude: 30.0 + float64(i%10), Longitude: 120.0 + float64(i%10)},
			Date:          date.Unix(),
			DateText:      date.Format("2006-01-02 15:04"),
			Rating:        []int{1, 2, 3, 4, 5, 0}[i%6], // 含未评分
			Price:         float64(50 + i*5),
			PayPrice:      float64(30 + i*4),
			OtherCost:     float64(i * 2),
			PriceCurrency: "CNY",
			Company:       []string{"上昆", "苏昆", "浙昆", "京昆"}[i%4],
			ActiveStatus:  1,
			ArtistNames:   []string{artists[i%8], artists[(i+3)%8]},
			Play:          []string{zhezis[i%6]},
			DramaIDs:      []string{dramaIDs[i%len(dramas)]},
			ZheziIDs:      []string{zheziIDs[i%6]},
			ArtistIDs:     []string{artistIDs[i%8], artistIDs[(i+3)%8]},
			CategoryName:  []string{"昆曲", "京剧", "越剧", "其他"}[i%4],
		}
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	// 再来几个没有评分/没有价格的边缘记录
	edges := []models.Record{
		{ID: "edge-no-rating", Name: "无评分", City: "上海", Channel: "大麦", Date: base.AddDate(0, -2, 0).Unix(), DateText: base.AddDate(0, -2, 0).Format("2006-01-02 15:04"), Price: 200, PayPrice: 200, CategoryName: "昆曲", ActiveStatus: 1, ArtistNames: []string{"张军"}},
		{ID: "edge-zero-price", Name: "零票价", City: "南京", Channel: "现场", Date: base.AddDate(0, -1, 0).Unix(), DateText: base.AddDate(0, -1, 0).Format("2006-01-02 15:04"), Price: 0, PayPrice: 0, CategoryName: "昆曲", ActiveStatus: 1},
		{ID: "edge-old", Name: "老记录", City: "苏州", Channel: "朋友送", Date: time.Now().AddDate(-3, 0, 0).Unix(), DateText: time.Now().AddDate(-3, 0, 0).Format("2006-01-02 15:04"), Rating: 4, Price: 800, PayPrice: 500, CategoryName: "昆曲", ActiveStatus: 1},
	}
	for _, r := range edges {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("Upsert edge: %v", err)
		}
	}

	a, err = db.GetAnalytics()
	if err != nil {
		t.Fatalf("GetAnalytics: %v", err)
	}

	// ---- Overview ----
	if a.Overview.TotalRecords != 60+len(edges) {
		t.Errorf("TotalRecords = %d, want %d", a.Overview.TotalRecords, 60+len(edges))
	}
	if a.Overview.TotalCost <= 0 {
		t.Error("TotalCost should be > 0 with data")
	}
	if a.Overview.AvgRating <= 0 || a.Overview.AvgRating > 5 {
		t.Errorf("AvgRating = %v, should be in (0,5]", a.Overview.AvgRating)
	}
	if a.Overview.TotalCities < 4 {
		t.Errorf("TotalCities = %d, want >= 4", a.Overview.TotalCities)
	}

	// ---- Trends: 24 个月，每个 TrendPoint.Period 格式 YYYY-MM ----
	if len(a.Trends) != 24 {
		t.Errorf("Trends len = %d, want 24", len(a.Trends))
	}
	for _, tp := range a.Trends {
		if len(tp.Period) != 7 || tp.Period[4] != '-' {
			t.Errorf("Trends Period = %q, want YYYY-MM", tp.Period)
		}
	}

	// ---- 分布 ----
	if len(a.CategoryDist) == 0 {
		t.Error("CategoryDist 不应为空")
	}
	if len(a.ChannelDist) == 0 {
		t.Error("ChannelDist 不应为空")
	}
	if len(a.CityDist) == 0 {
		t.Error("CityDist 不应为空")
	}
	if len(a.YearDist) == 0 {
		t.Error("YearDist 不应为空（含 edge-old 的跨年数据）")
	}
	// RatingDist: 5 颗星都应该有对应条目（哪怕 0）
	if len(a.RatingDist) != 5 {
		t.Errorf("RatingDist len = %d, want 5 (1..5★)", len(a.RatingDist))
	}

	// ---- 比较 / 异常 / 排名 / 其他 ----
	// CompareMonthly 基于 24 个月，取后 12 个 → 12 条
	if len(a.CompareMonthly) != 12 {
		t.Errorf("CompareMonthly len = %d, want 12", len(a.CompareMonthly))
	}
	// TopArtists / TopDramas / TopVenues 都应有数据
	if len(a.TopArtists) == 0 {
		t.Error("TopArtists 不应为空")
	}
	if len(a.TopDramas) == 0 {
		t.Error("TopDramas 不应为空")
	}
	if len(a.TopVenues) == 0 {
		t.Error("TopVenues 不应为空")
	}

	// CorrPairs 固定 4 条
	if len(a.CorrPairs) != 4 {
		t.Errorf("CorrPairs len = %d, want 4", len(a.CorrPairs))
	}

	// Scatter 应有数据（有 rating > 0 且 price > 0 的记录）
	if len(a.Scatter) == 0 {
		t.Error("Scatter 不应为空")
	}

	// 价格/其他花费 buckets、rewatch、discovery、diversity、interval、weekday
	// 应至少有非 nil 结果
	if a.Rewatch == nil {
		t.Error("Rewatch 不应为 nil")
	}
	if a.Diversity == nil {
		t.Error("Diversity 不应为 nil")
	}
	if a.Intervals == nil {
		t.Error("Intervals 不应为 nil")
	}

	// ---- TopZhezis / Discovery / WeekdayDist 等数组 ----
	if len(a.TopZhezis) == 0 {
		t.Error("TopZhezis 不应为空")
	}
	if len(a.Discovery) == 0 {
		t.Error("Discovery 不应为空")
	}
	if len(a.WeekdayDist) == 0 {
		t.Error("WeekdayDist 不应为空")
	}

	// ---- PriceBuckets / OtherCostBuckets ----
	if len(a.PriceBuckets) == 0 {
		t.Error("PriceBuckets 不应为空")
	}
	if len(a.OtherCostBuckets) == 0 {
		t.Error("OtherCostBuckets 不应为空")
	}

	// ---- Anomalies（z-score 检测）：数据跨 24 个月，有波动就应能检测到 ----
	// 不是必须有 anomaly，但如果活跃窗口有足够 variance，应该产生
	// 我们只验证：这个字段存在且不会 panic
	_ = a.Anomalies
}

// TestCountRecordsBranches 专门覆盖 CountRecords 的所有筛选分支
func TestCountRecordsBranches(t *testing.T) {
	db := newTestDB(t)

	// 先插入 dramas / zhezis / artists 实体
	db.conn.Exec("INSERT OR IGNORE INTO dramas (id, name, aliases) VALUES ('d-1', '牡丹亭', '[]'), ('d-2', '长生殿', '[]')")
	db.conn.Exec("INSERT OR IGNORE INTO zhezis (id, name, drama_id) VALUES ('z-1', '惊梦', 'd-1'), ('z-2', '寻梦', 'd-1')")
	db.conn.Exec("INSERT OR IGNORE INTO artists (id, name) VALUES ('a-1', '张军'), ('a-2', '王芳')")

	// 6 条记录，覆盖不同组合
	now := time.Now()
	recs := []models.Record{
		{ID: "c1", Name: "牡丹亭", City: "上海", Channel: "大麦", Company: "上昆", CategoryName: "昆曲", ActiveStatus: 1, DramaIDs: []string{"d-1"}, ZheziIDs: []string{"z-1"}, ArtistIDs: []string{"a-1"}, ArtistNames: []string{"张军"}, Date: now.Unix(), DateText: now.Format("2006-01-02 15:04")},
		{ID: "c2", Name: "牡丹亭", City: "北京", Channel: "现场", Company: "上昆", CategoryName: "昆曲", ActiveStatus: 1, DramaIDs: []string{"d-1"}, ZheziIDs: []string{"z-2"}, ArtistIDs: []string{"a-2"}, ArtistNames: []string{"王芳"}, Date: now.AddDate(0, -1, 0).Unix(), DateText: now.AddDate(0, -1, 0).Format("2006-01-02 15:04")},
		{ID: "c3", Name: "长生殿", City: "上海", Channel: "大麦", Company: "苏昆", CategoryName: "昆曲", ActiveStatus: 1, DramaIDs: []string{"d-2"}, ArtistIDs: []string{"a-1"}, ArtistNames: []string{"张军"}, Date: now.AddDate(0, -2, 0).Unix(), DateText: now.AddDate(0, -2, 0).Format("2006-01-02 15:04")},
		{ID: "c4", Name: "长生殿", City: "杭州", Channel: "微店", Company: "苏昆", CategoryName: "昆曲", ActiveStatus: 1, DramaIDs: []string{"d-2"}, ArtistIDs: []string{"a-2"}, ArtistNames: []string{"王芳"}, Date: now.AddDate(0, -3, 0).Unix(), DateText: now.AddDate(0, -3, 0).Format("2006-01-02 15:04")},
		{ID: "c5", Name: "京剧白蛇传", City: "北京", Channel: "大麦", Company: "京昆", CategoryName: "京剧", ActiveStatus: 1, Date: now.AddDate(0, -4, 0).Unix(), DateText: now.AddDate(0, -4, 0).Format("2006-01-02 15:04")},
		{ID: "c6", Name: "越剧红楼梦", City: "上海", Channel: "现场", Company: "越剧院", CategoryName: "越剧", ActiveStatus: 1, Date: now.AddDate(0, -5, 0).Unix(), DateText: now.AddDate(0, -5, 0).Format("2006-01-02 15:04")},
	}
	for _, r := range recs {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("Upsert %s: %v", r.ID, err)
		}
	}

	// 表驱动测试：每个 Case 设一个或多个筛选字段，验证总数
	cases := []struct {
		name string
		f    RecordFilter
		want int
	}{
		{"全量", RecordFilter{}, 6},
		{"Query: 牡丹亭", RecordFilter{Query: "牡丹亭"}, 2},
		{"Query: 张军 (演员)", RecordFilter{Query: "张军"}, 2},
		{"Query: 大麦 (channel)", RecordFilter{Query: "大麦"}, 3},
		{"City=上海", RecordFilter{City: "上海"}, 3},
		{"City=北京", RecordFilter{City: "北京"}, 2},
		{"DramaID=d-1 (牡丹亭)", RecordFilter{DramaID: "d-1"}, 2},
		{"DramaID=d-2 (长生殿)", RecordFilter{DramaID: "d-2"}, 2},
		{"ZheziID=z-1", RecordFilter{ZheziID: "z-1"}, 1},
		{"ArtistID=a-1 (张军)", RecordFilter{ArtistID: "a-1"}, 2},
		{"ArtistID=a-2 (王芳)", RecordFilter{ArtistID: "a-2"}, 2},
		{"Year+Month 当月", RecordFilter{Year: now.Year(), Month: int(now.Month())}, 1}, // c1
		{"City+Drama 联合", RecordFilter{City: "上海", DramaID: "d-1"}, 1},                // c1
		{"Artist+City 联合", RecordFilter{ArtistID: "a-1", City: "上海"}, 2},              // c1, c3
		{"Start 从今天起", RecordFilter{Start: now.Format("2006-01-02")}, 1},              // c1
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := db.CountRecords(tc.f)
			if err != nil {
				t.Fatalf("CountRecords(%+v): %v", tc.f, err)
			}
			if got != tc.want {
				t.Errorf("CountRecords(%+v) = %d, want %d", tc.f, got, tc.want)
			}
		})
	}
}

// ListRecordsContext/CountRecordsContext must behave like their non-ctx
// wrappers, and a cancelled context must stop the query instead of burning
// SQLite time for an abandoned request.
func TestListRecordsContext(t *testing.T) {
	d := newTestDB(t)
	if err := d.UpsertRecord(sampleRecord("r-ctx", 1755000000)); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertRecord(sampleRecord("r-ctx-2", 1754900000)); err != nil {
		t.Fatal(err)
	}

	recs, err := d.ListRecordsContext(context.Background(), RecordFilter{Query: "牡丹亭"})
	if err != nil {
		t.Fatalf("ListRecordsContext: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[0].DramaIDs == nil {
		t.Error("backfills should run in the ctx path too")
	}

	total, err := d.CountRecordsContext(context.Background(), RecordFilter{Query: "牡丹亭"})
	if err != nil {
		t.Fatalf("CountRecordsContext: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}

	// Wrapper equivalence.
	wrapped, err := d.ListRecords(RecordFilter{Query: "牡丹亭"})
	if err != nil || len(wrapped) != 2 {
		t.Fatalf("ListRecords wrapper: %v, %d records", err, len(wrapped))
	}

	// A pre-cancelled context fails the query instead of scanning.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.ListRecordsContext(ctx, RecordFilter{}); err == nil {
		t.Error("cancelled context should abort ListRecordsContext")
	}
	if _, err := d.CountRecordsContext(ctx, RecordFilter{}); err == nil {
		t.Error("cancelled context should abort CountRecordsContext")
	}
}
