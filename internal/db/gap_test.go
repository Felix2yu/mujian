package db

import (
	"path/filepath"
	"testing"
	"time"

	"mujian/internal/models"
)

func TestSearchByLocation(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 8, 22, 19, 30, 0, 0, time.Local).Unix()

	// 人民广场附近（上海）两场，北京一场，无坐标一场。
	near := models.Record{ID: "rec-near", Name: "近场", City: "上海", Address: "上海大剧院",
		CategoryName: "昆曲", Date: base,
		Coordinate: &models.Coordinate{Latitude: 31.2304, Longitude: 121.4737}}
	if err := db.UpsertRecord(near); err != nil {
		t.Fatalf("upsert near: %v", err)
	}
	far := models.Record{ID: "rec-far", Name: "远场", City: "上海", Address: "佘山",
		CategoryName: "话剧", Date: base + 86400,
		Coordinate: &models.Coordinate{Latitude: 31.0980, Longitude: 121.1920}}
	if err := db.UpsertRecord(far); err != nil {
		t.Fatalf("upsert far: %v", err)
	}
	if err := db.UpsertRecord(models.Record{ID: "rec-bj", Name: "北京场", City: "北京",
		Date: base, Coordinate: &models.Coordinate{Latitude: 39.9, Longitude: 116.4}}); err != nil {
		t.Fatalf("upsert bj: %v", err)
	}
	if err := db.UpsertRecord(models.Record{ID: "rec-nocoord", Name: "无坐标", City: "上海", Date: base}); err != nil {
		t.Fatalf("upsert nocoord: %v", err)
	}

	// 半径 5km：只有近场。
	res, err := db.SearchByLocation(31.2304, 121.4737, 5000, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("SearchByLocation: %v", err)
	}
	if len(res) != 1 || res[0].ID != "rec-near" {
		t.Fatalf("5km search: %+v", res)
	}
	if res[0].Coordinate == nil || res[0].DistanceM < 0 || res[0].DistanceM > 5000 {
		t.Fatalf("near result: %+v", res[0])
	}

	// 半径放大后按距离排序（近 → 远；北京距上海约 1000km，被排除）。
	res, _ = db.SearchByLocation(31.2304, 121.4737, 100000, 10, nil, nil, nil, nil)
	if len(res) != 2 || res[0].ID != "rec-near" || res[1].ID != "rec-far" {
		t.Fatalf("100km search order: %+v", res)
	}

	// 城市过滤。
	city := "上海"
	res, _ = db.SearchByLocation(31.2304, 121.4737, 1000000, 10, nil, &city, nil, nil)
	if len(res) != 2 {
		t.Fatalf("city filter: %+v", res)
	}

	// 剧种过滤。
	cat := "话剧"
	res, _ = db.SearchByLocation(31.2304, 121.4737, 100000, 10, &cat, nil, nil, nil)
	if len(res) != 1 || res[0].ID != "rec-far" {
		t.Fatalf("category filter: %+v", res)
	}

	// 日期过滤：开始日期只保留 8/22 及之后……实际两者都在范围内，检查起点排除。
	start := "2026-08-23"
	res, _ = db.SearchByLocation(31.2304, 121.4737, 100000, 10, nil, nil, &start, nil)
	if len(res) != 1 || res[0].ID != "rec-far" {
		t.Fatalf("start date filter: %+v", res)
	}
	end := "2026-08-22" // 结束日按「< 当日+1 天」过滤：远场在 8/23 19:30，被排除
	res, _ = db.SearchByLocation(31.2304, 121.4737, 100000, 10, nil, nil, nil, &end)
	if len(res) != 1 || res[0].ID != "rec-near" {
		t.Fatalf("end date filter: %+v", res)
	}

	// limit 生效。
	res, _ = db.SearchByLocation(31.2304, 121.4737, 100000, 1, nil, nil, nil, nil)
	if len(res) != 1 {
		t.Fatalf("limit: %+v", res)
	}

	// 无匹配时返回空切片而非 nil。
	res, _ = db.SearchByLocation(0, 0, 1, 10, nil, nil, nil, nil)
	if res == nil || len(res) != 0 {
		t.Fatalf("no match should be empty slice: %#v", res)
	}
}

func TestZheziGetters(t *testing.T) {
	db := newTestDB(t)
	drama, err := db.SaveDrama(models.Drama{Name: "牡丹亭"})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	z1, err := db.CreateZhezi(models.Zhezi{DramaID: drama.ID, Name: "游园"})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	z2, err := db.CreateZhezi(models.Zhezi{DramaID: drama.ID, Name: "惊梦", Aliases: []string{"惊梦集成"}})
	if err != nil {
		t.Fatalf("CreateZhezi 2: %v", err)
	}

	// GetZhezi / GetZheziNames。
	got, err := db.GetZhezi(z1.ID)
	if err != nil {
		t.Fatalf("GetZhezi: %v", err)
	}
	if got.Name != "游园" || got.DramaID != drama.ID {
		t.Fatalf("GetZhezi: %+v", got)
	}
	if _, err := db.GetZhezi("missing"); err == nil {
		t.Fatal("GetZhezi missing should error")
	}

	names, err := db.GetZheziNames([]string{z1.ID, z2.ID, "missing"})
	if err != nil {
		t.Fatalf("GetZheziNames: %v", err)
	}
	if len(names) != 2 || names[z1.ID] != "游园" || names[z2.ID] != "惊梦" {
		t.Fatalf("GetZheziNames: %v", names)
	}
	// 空 id 列表直接返回空 map。
	empty, err := db.GetZheziNames(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("GetZheziNames empty: %v %v", empty, err)
	}
}

func TestReplaceRecordPhotos(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertRecord(models.Record{ID: "rec-p", Name: "照片记录", Date: time.Now().Unix()}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	photos := []models.RecordPhoto{
		{ID: "p-2", FileName: "covers/b.avif"},
		{ID: "p-1", FileName: "covers/a.avif", Sort: 3},
		{ID: "p-3", FileName: "covers/c.avif"}, // sort=0 → 按下标补 1
	}
	if err := db.ReplaceRecordPhotos("rec-p", photos); err != nil {
		t.Fatalf("ReplaceRecordPhotos: %v", err)
	}

	got, err := db.ListRecordPhotos("rec-p")
	if err != nil {
		t.Fatalf("ListRecordPhotos: %v", err)
	}
	// sort=0 的条目按循环下标补齐（p-2→1、p-3→3），与显式 sort 一起排序；
	// 并列时按 id ASC（p-1 在 p-3 前）。
	if len(got) != 3 || got[0].ID != "p-2" || got[1].ID != "p-1" || got[2].ID != "p-3" {
		t.Fatalf("photos after replace: %+v", got)
	}

	// 重复替换覆盖旧关联。
	if err := db.ReplaceRecordPhotos("rec-p", []models.RecordPhoto{{ID: "p-x", FileName: "covers/x.avif"}}); err != nil {
		t.Fatalf("ReplaceRecordPhotos 2: %v", err)
	}
	if got, _ = db.ListRecordPhotos("rec-p"); len(got) != 1 || got[0].ID != "p-x" {
		t.Fatalf("photos after second replace: %+v", got)
	}

	// 空列表清空关联。
	if err := db.ReplaceRecordPhotos("rec-p", nil); err != nil {
		t.Fatalf("ReplaceRecordPhotos empty: %v", err)
	}
	if got, _ = db.ListRecordPhotos("rec-p"); len(got) != 0 {
		t.Fatalf("photos should be cleared: %+v", got)
	}
}

func TestPurgeExpiredDeletedRecords(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	for _, id := range []string{"rec-a", "rec-b", "rec-c"} {
		if err := db.UpsertRecord(models.Record{ID: id, Name: id, Date: base}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	// rec-a、rec-b 软删且时间戳调到 31 天前；rec-c 刚删。
	if _, err := db.SoftDeleteRecords([]string{"rec-a", "rec-b", "rec-c"}); err != nil {
		t.Fatalf("SoftDeleteRecords: %v", err)
	}
	old := base - 31*86400
	if _, err := db.conn.Exec("UPDATE records SET deleted_at = ? WHERE id IN ('rec-a','rec-b')", old); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if n, _ := db.DeletedCount(); n != 3 {
		t.Fatalf("deleted count before purge: %d", n)
	}

	purged, err := db.PurgeExpiredDeletedRecords(30 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PurgeExpiredDeletedRecords: %v", err)
	}
	if purged != 2 {
		t.Fatalf("purged = %d, want 2", purged)
	}
	// rec-a 被硬删；rec-c 还在回收站里（GetRecord 过滤软删记录）。
	if n, _ := db.DeletedCount(); n != 1 {
		t.Fatalf("rec-c should survive in trash, deleted count = %d", n)
	}

	// 没有过期记录时返回 0。
	purged, err = db.PurgeExpiredDeletedRecords(30 * 24 * time.Hour)
	if err != nil || purged != 0 {
		t.Fatalf("second purge: %d %v", purged, err)
	}
}

func TestVacuumIntoAndCheckpoint(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertRecord(models.Record{ID: "rec-v", Name: "v", Date: time.Now().Unix()}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "vacuum.db")
	if err := db.VacuumInto(dst); err != nil {
		t.Fatalf("VacuumInto: %v", err)
	}
	if err := db.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
}

func TestSQLStatsAndLocation(t *testing.T) {
	db := newTestDB(t)
	if db.Location() == nil {
		t.Fatal("Location should return configured zone")
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	stats := db.SQLStats()
	if stats.OpenConnections == 0 {
		t.Fatal("SQLStats should expose pool stats")
	}
}
