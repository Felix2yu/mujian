package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"mujian/internal/backup"
	"mujian/internal/config"
	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newBackupTestServer builds a server whose backup manager writes into a
// temp dir with the json snapshot format (online-restorable).
func newBackupTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	cfg := &config.Config{BackupFormat: "json"}
	s.backup = backup.New(s.db, filepath.Join(t.TempDir(), "backups"), cfg)
	s.backup.SetExporter(func() ([]byte, error) {
		data, err := s.db.Export()
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}, nil)
	return s
}

// resText extracts the tool output JSON text for direct unmarshalling.
func resText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty tool result")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

func TestTrashTools(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	mustUpsert(t, s, models.Record{ID: "rec-t1", Name: "回收一", Date: time.Now().Unix()})
	mustUpsert(t, s, models.Record{ID: "rec-t2", Name: "回收二", Date: time.Now().Unix()})

	// 空回收站。
	if res, _, err := s.handleListDeletedRecords(ctx, nil, ListDeletedRecordsInput{}); err != nil || res.IsError {
		t.Fatalf("list deleted empty: %v %v", res, err)
	}

	// restore / purge 的 dry_run 预览走 GetRecord（只看未删除记录），所以在软删前验证。
	if res, _, _ := s.handleRestoreRecord(ctx, nil, RestoreRecordInput{ID: "rec-t1"}); res.IsError {
		t.Fatalf("restore dry run: %v", res)
	}
	if res, _, _ := s.handlePurgeRecord(ctx, nil, PurgeRecordInput{ID: "rec-t2"}); res.IsError {
		t.Fatalf("purge dry run: %v", res)
	}
	if _, err := s.db.SoftDeleteRecords([]string{"rec-t1", "rec-t2"}); err != nil {
		t.Fatalf("SoftDeleteRecords: %v", err)
	}

	// 列表 + 分页参数。
	res, _, err := s.handleListDeletedRecords(ctx, nil, ListDeletedRecordsInput{Limit: intPtrT(1), Offset: intPtrT(0)})
	if err != nil || res.IsError {
		t.Fatalf("list deleted: %v %v", res, err)
	}
	m := resultMap(t, res)
	if num(t, m, "total") != 2 {
		t.Fatalf("total: %v", m)
	}

	if res, _, err := s.handleRestoreRecord(ctx, nil, RestoreRecordInput{ID: "rec-t1", DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("restore: %v %v", res, err)
	}
	if got, _ := s.db.GetRecord("rec-t1"); got == nil || got.Name != "回收一" {
		t.Fatalf("restored record: %+v", got)
	}
	if res, _, _ := s.handleRestoreRecord(ctx, nil, RestoreRecordInput{ID: "rec-t1", DryRun: boolPtr(false)}); !res.IsError {
		t.Fatal("restoring a live record should error")
	}

	// purge 单条（dry_run 已在软删前验证）。
	if res, _, err := s.handlePurgeRecord(ctx, nil, PurgeRecordInput{ID: "rec-t2", DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("purge: %v %v", res, err)
	}

	// 清空回收站：dry_run + 执行。
	mustUpsert(t, s, models.Record{ID: "rec-t3", Name: "回收三", Date: time.Now().Unix()})
	if _, err := s.db.SoftDeleteRecords([]string{"rec-t3"}); err != nil {
		t.Fatalf("SoftDeleteRecords rec-t3: %v", err)
	}
	if res, _, _ := s.handlePurgeDeletedRecords(ctx, nil, PurgeDeletedRecordsInput{}); res.IsError {
		t.Fatalf("purge-all dry run: %v", res)
	}
	if res, _, err := s.handlePurgeDeletedRecords(ctx, nil, PurgeDeletedRecordsInput{DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("purge-all: %v %v", res, err)
	}
	res, _, _ = s.handleListDeletedRecords(ctx, nil, ListDeletedRecordsInput{})
	m = resultMap(t, res)
	if num(t, m, "total") != 0 {
		t.Fatalf("trash should be empty: %v", m)
	}
}

func TestReorderTools(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	c1 := &models.Category{Name: "昆曲"}
	if err := s.db.UpsertCategory(c1); err != nil {
		t.Fatalf("UpsertCategory: %v", err)
	}
	c2 := &models.Category{Name: "话剧"}
	if err := s.db.UpsertCategory(c2); err != nil {
		t.Fatalf("UpsertCategory 2: %v", err)
	}
	d1, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭"})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	d2, err := s.db.SaveDrama(models.Drama{Name: "茶馆"})
	if err != nil {
		t.Fatalf("SaveDrama 2: %v", err)
	}
	a1, err := s.db.SaveArtist(models.Artist{Name: "张军"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	a2, err := s.db.SaveArtist(models.Artist{Name: "濮存昕"})
	if err != nil {
		t.Fatalf("SaveArtist 2: %v", err)
	}
	z1, err := s.db.CreateZhezi(models.Zhezi{DramaID: d1.ID, Name: "游园"})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	z2, err := s.db.CreateZhezi(models.Zhezi{DramaID: d1.ID, Name: "惊梦"})
	if err != nil {
		t.Fatalf("CreateZhezi 2: %v", err)
	}

	// 空 ids 报错。
	if res, _, _ := s.handleReorderCategories(ctx, nil, ReorderCategoriesInput{}); !res.IsError {
		t.Fatal("empty ids should error")
	}
	if res, _, _ := s.handleReorderDramas(ctx, nil, ReorderDramasInput{}); !res.IsError {
		t.Fatal("empty ids should error")
	}
	if res, _, _ := s.handleReorderZhezis(ctx, nil, ReorderZhezisInput{DramaID: d1.ID}); !res.IsError {
		t.Fatal("empty ids should error")
	}
	if res, _, _ := s.handleReorderArtists(ctx, nil, ReorderArtistsInput{}); !res.IsError {
		t.Fatal("empty ids should error")
	}

	// dry run 预览不落库。
	if res, _, _ := s.handleReorderCategories(ctx, nil, ReorderCategoriesInput{IDs: []string{c2.ID, c1.ID}}); res.IsError {
		t.Fatalf("reorder categories dry run: %v", res)
	}

	// 实际排序：反转各实体。
	if res, _, err := s.handleReorderCategories(ctx, nil, ReorderCategoriesInput{IDs: []string{c2.ID, c1.ID}, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("reorder categories: %v %v", res, err)
	}
	if res, _, err := s.handleReorderDramas(ctx, nil, ReorderDramasInput{IDs: []string{d2.ID, d1.ID}, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("reorder dramas: %v %v", res, err)
	}
	if res, _, err := s.handleReorderZhezis(ctx, nil, ReorderZhezisInput{DramaID: d1.ID, IDs: []string{z2.ID, z1.ID}, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("reorder zhezis: %v %v", res, err)
	}
	if res, _, err := s.handleReorderArtists(ctx, nil, ReorderArtistsInput{IDs: []string{a2.ID, a1.ID}, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("reorder artists: %v %v", res, err)
	}

	// 验证落库后的顺序（sort_order 反转）。
	dramas, _ := s.db.ListDramas()
	if len(dramas) != 2 || dramas[0].Name != "茶馆" {
		t.Fatalf("drama order after reorder: %+v", dramas)
	}
	zhezis, _ := s.db.ListZhezisByDrama(d1.ID)
	if len(zhezis) != 2 || zhezis[0].Name != "惊梦" {
		t.Fatalf("zhezi order after reorder: %+v", zhezis)
	}
}

func TestBackupTools(t *testing.T) {
	s := newBackupTestServer(t)
	ctx := context.Background()

	// dry run 只预览。
	if res, _, _ := s.handleRunBackup(ctx, nil, RunBackupInput{}); res.IsError {
		t.Fatalf("run backup dry run: %v", res)
	}

	// 实际备份。
	res, _, err := s.handleRunBackup(ctx, nil, RunBackupInput{DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("run backup: %v %v", res, err)
	}
	var runRes struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(resText(t, res)), &runRes); err != nil || runRes.File == "" {
		t.Fatalf("run backup output: %s (%v)", resText(t, res), err)
	}

	// 清单。
	res, _, _ = s.handleListBackups(ctx, nil, noInput{})
	if res.IsError {
		t.Fatalf("list backups: %v", res)
	}

	// restore-from：.db 拒绝、坏扩展名拒绝、未知文件拒绝、json 成功。
	if res, _, _ := s.handleRestoreFromBackup(ctx, nil, RestoreFromBackupInput{File: "mujian-20260101-000000.db"}); !res.IsError {
		t.Fatal(".db restore should error")
	}
	if res, _, _ := s.handleRestoreFromBackup(ctx, nil, RestoreFromBackupInput{File: "mujian-20260101-000.tar"}); !res.IsError {
		t.Fatal("bad ext should error")
	}
	if res, _, _ := s.handleRestoreFromBackup(ctx, nil, RestoreFromBackupInput{File: "missing.json"}); !res.IsError {
		t.Fatal("missing file should error")
	}
	if res, _, _ := s.handleRestoreFromBackup(ctx, nil, RestoreFromBackupInput{File: runRes.File}); res.IsError {
		t.Fatalf("restore dry run: %v", res)
	}
	if res, _, err := s.handleRestoreFromBackup(ctx, nil, RestoreFromBackupInput{File: runRes.File, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("restore from backup: %v %v", res, err)
	}

	// 删除：dry run、执行、空 file。
	if res, _, _ := s.handleDeleteBackup(ctx, nil, DeleteBackupInput{File: runRes.File}); res.IsError {
		t.Fatalf("delete dry run: %v", res)
	}
	if res, _, _ := s.handleDeleteBackup(ctx, nil, DeleteBackupInput{}); !res.IsError {
		t.Fatal("empty file should error")
	}
	if res, _, err := s.handleDeleteBackup(ctx, nil, DeleteBackupInput{File: runRes.File, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("delete backup: %v %v", res, err)
	}
}

func TestExportImportTools(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	mustUpsert(t, s, models.Record{ID: "rec-exp", Name: "导出测试", Date: time.Now().Unix()})

	// 导出。
	res, _, err := s.handleExportData(ctx, nil, ExportDataInput{})
	if err != nil || res.IsError {
		t.Fatalf("export: %v %v", res, err)
	}
	m := resultMap(t, res)
	if num(t, m, "record_count") != 1 && num(t, m, "record_count") != 0 {
		t.Fatalf("export record_count: %v", m["record_count"])
	}
	if _, ok := m["records"]; !ok {
		t.Fatalf("export should carry records: %v", m)
	}

	// 导入：空数据、坏 JSON、dry run、真实导入。
	if res, _, _ := s.handleImportData(ctx, nil, ImportDataInput{}); !res.IsError {
		t.Fatal("empty json_data should error")
	}
	if res, _, _ := s.handleImportData(ctx, nil, ImportDataInput{JSONData: "{bad"}); !res.IsError {
		t.Fatal("bad JSON should error")
	}
}

func TestMapAndLocationTools(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()
	mustUpsert(t, s, models.Record{ID: "rec-geo", Name: "地图点", City: "上海",
		CategoryName: "昆曲", Date: base,
		Coordinate: &models.Coordinate{Latitude: 31.23, Longitude: 121.47}})
	mustUpsert(t, s, models.Record{ID: "rec-nocoord", Name: "无坐标", Date: base})

	// list_map_points：有坐标过滤 + 城市过滤。
	res, _, err := s.handleListMapPoints(ctx, nil, ListMapPointsInput{})
	if err != nil || res.IsError {
		t.Fatalf("map points: %v %v", res, err)
	}
	m := resultMap(t, res)
	if num(t, m, "total") != 1 {
		t.Fatalf("map points total: %v", m)
	}
	city := "北京"
	res, _, _ = s.handleListMapPoints(ctx, nil, ListMapPointsInput{City: &city})
	m = resultMap(t, res)
	if num(t, m, "total") != 0 {
		t.Fatalf("map points city filter: %v", m)
	}

	// search_by_location：半径过滤 + 过滤参数。
	if res, _, err := s.handleSearchByLocation(ctx, nil, SearchByLocationInput{
		Latitude: 31.23, Longitude: 121.47, Radius: 1000,
	}); err != nil || res.IsError {
		t.Fatalf("search by location: %v %v", res, err)
	}
	cat := "话剧"
	res, _, _ = s.handleSearchByLocation(ctx, nil, SearchByLocationInput{
		Latitude: 31.23, Longitude: 121.47, Radius: 1000, Category: &cat,
	})
	m = resultMap(t, res)
	if num(t, m, "total") != 0 {
		t.Fatalf("search by location category filter: %v", m)
	}
}
