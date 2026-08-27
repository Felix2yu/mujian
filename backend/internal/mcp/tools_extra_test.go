package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"mujian/internal/db"
	"mujian/internal/models"
	"path/filepath"
	"testing"
	"time"
)

// ---------- helpers ----------

func intPtrT(i int) *int { return &i }

// boolPtr is a test helper for the *bool dry_run field.
func boolPtr(b bool) *bool { return &b }

// seedBasicData writes a record + an artist + a drama + a zhezi so CRUD tests
// have anchors to read back and mutate.
func seedBasicData(t *testing.T, s *Server) (recID, artistID, dramaID, zheziID string) {
	t.Helper()
	base := time.Date(2026, 5, 1, 19, 30, 0, 0, time.UTC).Unix()
	mustUpsert(t, s, models.Record{
		ID:            "rec-basic",
		Name:          "牡丹亭",
		City:          "上海",
		Address:       "上海大剧院",
		CategoryName:  "昆曲",
		CategoryNames: []string{"昆曲"},
		ArtistNames:   []string{"张军"},
		DramaIDs:      []string{},
		Date:          base,
	})
	a, err := s.db.SaveArtist(models.Artist{Name: "张军", Aliases: []string{"军哥"}})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	d, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭", CategoryNames: []string{"昆曲"}})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	z, err := s.db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "游园", Aliases: []string{"堆花"}})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	return "rec-basic", a.ID, d.ID, z.ID
}

// ---------- record CRUD ----------

func TestCreateRecord(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// name 必填：空名直接报工具级错误，不写库。
	if res, _, _ := s.handleCreateRecord(ctx, nil, CreateRecordInput{Name: "   "}); !res.IsError {
		t.Fatal("blank name should be rejected")
	}

	// dry_run：只预览不创建。
	res, _, err := s.handleCreateRecord(ctx, nil, CreateRecordInput{
		Name:          "长生殿",
		City:          "北京",
		CategoryName:  "昆曲",
		CategoryNames: StringOrArray{"昆曲", "京昆"},
		ArtistNames:   StringOrArray{"魏春荣"},
		Price:         180,
		DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatalf("expected dry_run flag, got %v", m)
	}
	// 库里不应新增记录。
	if rs, _ := s.db.ListRecords(db.RecordFilter{}); len(rs) != 0 {
		t.Fatalf("dry run must not persist; got %d records", len(rs))
	}

	// 真实创建。
	res, _, err = s.handleCreateRecord(ctx, nil, CreateRecordInput{
		Name:          "长生殿",
		City:          "北京",
		CategoryName:  "昆曲",
		CategoryNames: StringOrArray{"昆曲", "京昆"},
		ArtistNames:   StringOrArray{"魏春荣"},
		Price:         180,
		DryRun:        boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("create: %v %v", res, err)
	}
	created := resultMap(t, res)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("created record missing id: %v", created)
	}
	got, err := s.db.GetRecord(id)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got.Name != "长生殿" || got.City != "北京" || got.Price != 180 {
		t.Fatalf("created record mismatch: %+v", got)
	}
	if len(got.CategoryNames) != 2 || len(got.ArtistNames) != 1 || got.ArtistNames[0] != "魏春荣" {
		t.Fatalf("array fields not persisted: %+v", got)
	}
}

func TestUpdateRecord(t *testing.T) {
	s := newTestServer(t)
	recID, _, _, _ := seedBasicData(t, s)
	ctx := context.Background()

	// id 必填。
	if res, _, _ := s.handleUpdateRecord(ctx, nil, UpdateRecordInput{}); !res.IsError {
		t.Fatal("empty id should be rejected")
	}
	// 不存在的 id 报工具级错误。
	if res, _, _ := s.handleUpdateRecord(ctx, nil, UpdateRecordInput{ID: "nope"}); !res.IsError {
		t.Fatal("missing record should error")
	}

	// dry_run：返回 original / updated 预览，不落库。
	res, _, err := s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:         recID,
		Name:       strPtrT("牡丹亭·上本"),
		Price:      fltPtrT(220),
		CategoryNames: &ArrayOp{Op: "append", Value: []string{"京昆"}},
		DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatalf("expected dry_run flag")
	}
	unchanged, _ := s.db.GetRecord(recID)
	if unchanged.Name != "牡丹亭" || unchanged.Price != 0 {
		t.Fatalf("dry run must not write; got %+v", unchanged)
	}

	// 应用：标量 + 数组 append 混合。
	res, _, err = s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:            recID,
		Name:          strPtrT("牡丹亭·上本"),
		Price:         fltPtrT(220),
		CategoryNames: &ArrayOp{Op: "append", Value: []string{"京昆"}},
		DryRun:        boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("apply: %v %v", res, err)
	}
	got, _ := s.db.GetRecord(recID)
	if got.Name != "牡丹亭·上本" || got.Price != 220 {
		t.Fatalf("scalar update failed: %+v", got)
	}
	if len(got.CategoryNames) != 2 {
		t.Fatalf("category_names append failed: %+v", got.CategoryNames)
	}

	// 数组 set / remove：在多元素场景下验证，避免空数组回退到 scalar
	// category_name（normalizeCategories 在列表为空时用 category_name 兜底）。
	res, _, err = s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:            recID,
		CategoryNames: &ArrayOp{Op: "set", Value: []string{"昆曲", "京剧"}},
		DryRun:        boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("set: %v %v", res, err)
	}
	got, _ = s.db.GetRecord(recID)
	if len(got.CategoryNames) != 2 || got.CategoryNames[0] != "昆曲" || got.CategoryNames[1] != "京剧" {
		t.Fatalf("set failed: %+v", got.CategoryNames)
	}
	res, _, err = s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:            recID,
		CategoryNames: &ArrayOp{Op: "remove", Value: []string{"京剧"}},
		DryRun:        boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("remove: %v %v", res, err)
	}
	got, _ = s.db.GetRecord(recID)
	if len(got.CategoryNames) != 1 || got.CategoryNames[0] != "昆曲" {
		t.Fatalf("remove failed: %+v", got.CategoryNames)
	}
}

func TestDeleteRecord(t *testing.T) {
	s := newTestServer(t)
	recID, _, _, _ := seedBasicData(t, s)
	ctx := context.Background()

	if res, _, _ := s.handleDeleteRecord(ctx, nil, DeleteRecordInput{}); !res.IsError {
		t.Fatal("empty id should be rejected")
	}
	if res, _, _ := s.handleDeleteRecord(ctx, nil, DeleteRecordInput{ID: "nope"}); !res.IsError {
		t.Fatal("missing record should error")
	}

	// dry_run 预览。
	res, _, err := s.handleDeleteRecord(ctx, nil, DeleteRecordInput{ID: recID, DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	if _, err := s.db.GetRecord(recID); err != nil {
		t.Fatal("dry run must not delete")
	}

	// 应用删除。
	res, _, err = s.handleDeleteRecord(ctx, nil, DeleteRecordInput{ID: recID, DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("delete: %v %v", res, err)
	}
	if _, err := s.db.GetRecord(recID); err == nil {
		t.Fatal("record should be gone after delete")
	}
}

func TestBatchDeleteRecords(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	for i, id := range []string{"bd1", "bd2", "bd3"} {
		mustUpsert(t, s, models.Record{ID: id, Name: fmt.Sprintf("rec-%d", i)})
	}

	// 空 ids 报错。
	if res, _, _ := s.handleBatchDeleteRecords(ctx, nil, BatchDeleteRecordsInput{}); !res.IsError {
		t.Fatal("empty ids should be rejected")
	}

	// dry_run：列出命中与未命中。
	res, _, err := s.handleBatchDeleteRecords(ctx, nil, BatchDeleteRecordsInput{
		IDs:    []string{"bd1", "bd2", "ghost"},
		DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if num(t, m, "requested") != 3 {
		t.Fatalf("requested = %v", m["requested"])
	}
	items := m["records"].([]any)
	if len(items) != 3 {
		t.Fatalf("records preview len = %d, want 3", len(items))
	}

	// 应用：bd1、bd2 删除，bd3 保留。
	res, _, err = s.handleBatchDeleteRecords(ctx, nil, BatchDeleteRecordsInput{IDs: []string{"bd1", "bd2"}, DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("apply: %v %v", res, err)
	}
	m = resultMap(t, res)
	if num(t, m, "deleted") != 2 || num(t, m, "requested") != 2 {
		t.Fatalf("batch delete summary wrong: %v", m)
	}
	if _, err := s.db.GetRecord("bd1"); err == nil {
		t.Fatal("bd1 should be deleted")
	}
	if _, err := s.db.GetRecord("bd3"); err != nil {
		t.Fatal("bd3 should survive")
	}
}

// TestUpdateRecordDryRunPreview exercises the many per-field `if in.X != nil`
// branches and the dry_run preview of handleUpdateRecord without touching the DB.
func TestUpdateRecordDryRunPreview(t *testing.T) {
	s := newTestServer(t)
	recID, _, _, _ := seedBasicData(t, s)
	ctx := context.Background()
	rating := 4

	res, _, err := s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:            recID,
		Name:          strPtrT("预览名"),
		City:          strPtrT("苏州"),
		Address:       strPtrT("苏州剧院"),
		Company:       strPtrT("苏昆"),
		Channel:       strPtrT("线下"),
		Rating:        &rating,
		Price:         fltPtrT(88),
		Seat:          strPtrT("A1"),
		Remark:        strPtrT("备注"),
		CategoryNames: &ArrayOp{Op: "set", Value: []string{"昆曲"}},
		DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	updated := m["updated"].(map[string]any)
	if updated["name"] != "预览名" || updated["city"] != "苏州" || updated["company"] != "苏昆" || updated["channel"] != "线下" {
		t.Fatalf("preview missing fields: %v", updated)
	}
	if updated["rating"] != float64(4) || updated["price"] != float64(88) || updated["seat"] != "A1" || updated["remark"] != "备注" {
		t.Fatalf("preview scalar mismatch: %v", updated)
	}
	// 库不变。
	if got, _ := s.db.GetRecord(recID); got.Name != "牡丹亭" {
		t.Fatalf("dry run must not persist: %+v", got)
	}
}

// TestBatchUpdateRecordsDryRun covers the large dry_run preview block (every
// field enumeration) plus apply of scalar + multi-mode array ops.
func TestBatchUpdateRecordsDryRun(t *testing.T) {
	s := newTestServer(t)
	recID, _, _, _ := seedBasicData(t, s)
	ctx := context.Background()
	rating := 5

	upd := BatchUpdateRecordsInput{
		IDs:           []string{recID},
		Name:          strPtrT("更新名"),
		City:          strPtrT("杭州"),
		Company:       strPtrT("浙昆"),
		Rating:        &rating,
		Price:         fltPtrT(99),
		CategoryNames: &ArrayOp{Op: "append", Value: []string{"婺剧"}},
		ArtistNames:   &ArrayOp{Op: "set", Value: []string{"新演员"}},
		TagIDs:        &ArrayOp{Op: "remove", Value: []string{"t1"}},
		DramaIDs:      &ArrayOp{Op: "append", Value: []string{"d1"}},
		DryRun: boolPtr(true),
	}

	res, _, err := s.handleBatchUpdateRecords(ctx, nil, upd)
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	if num(t, m, "requested") != 1 {
		t.Fatalf("requested = %v", m["requested"])
	}
	if len(m["changes"].([]any)) != 9 {
		t.Fatalf("changes len = %d, want 9", len(m["changes"].([]any)))
	}
	if unchanged, _ := s.db.GetRecord(recID); unchanged.Name != "牡丹亭" {
		t.Fatalf("dry run must not persist: %+v", unchanged)
	}

	// 应用：多字段写入。
	upd.DryRun = boolPtr(false)
	res, _, err = s.handleBatchUpdateRecords(ctx, nil, upd)
	if err != nil || res.IsError {
		t.Fatalf("apply: %v %v", res, err)
	}
	if num(t, resultMap(t, res), "updated") != 1 {
		t.Fatalf("updated mismatch: %v", resultMap(t, res)["updated"])
	}
	got, _ := s.db.GetRecord(recID)
	if got.Name != "更新名" || got.City != "杭州" || got.Company != "浙昆" || got.Rating != 5 || got.Price != 99 {
		t.Fatalf("batch apply scalar mismatch: %+v", got)
	}
	if len(got.CategoryNames) != 2 || got.CategoryNames[1] != "婺剧" {
		t.Fatalf("category_names mismatch: %+v", got.CategoryNames)
	}
	if len(got.ArtistNames) != 1 || got.ArtistNames[0] != "新演员" {
		t.Fatalf("artist_names mismatch: %+v", got.ArtistNames)
	}
}

// ---------- artist CRUD ----------

func TestArtistCRUD(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// create：name 必填 + 真实创建。
	if res, _, _ := s.handleCreateArtist(ctx, nil, CreateArtistInput{}); !res.IsError {
		t.Fatal("blank name should be rejected")
	}
	res, _, err := s.handleCreateArtist(ctx, nil, CreateArtistInput{
		Name: "魏春荣", Aliases: StringOrArray{"魏姐"}, Remark: "北昆", DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleCreateArtist(ctx, nil, CreateArtistInput{
		Name: "魏春荣", Aliases: StringOrArray{"魏姐"}, Remark: "北昆",
		DryRun: boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("create: %v %v", res, err)
	}
	created := resultMap(t, res)
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("artist missing id: %v", created)
	}

	// update：改名称 / 别名 / 简介，dry_run 预览。
	res, _, err = s.handleUpdateArtist(ctx, nil, UpdateArtistInput{
		ID: id, Name: strPtrT("魏春荣·青年版"), Bio: strPtrT("昆曲闺门旦"),
		Aliases: &ArrayOp{Op: "append", Value: []string{"小魏"}}, DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("update dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true || m["name"] != "魏春荣·青年版" {
		t.Fatalf("update preview wrong: %v", m)
	}
	res, _, err = s.handleUpdateArtist(ctx, nil, UpdateArtistInput{
		ID: id, Name: strPtrT("魏春荣·青年版"), Bio: strPtrT("昆曲闺门旦"),
		Aliases: &ArrayOp{Op: "append", Value: []string{"小魏"}},
		DryRun:  boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("update: %v %v", res, err)
	}
	got, _ := s.db.GetArtist(id)
	if got.Name != "魏春荣·青年版" || got.Bio != "昆曲闺门旦" || len(got.Aliases) != 2 {
		t.Fatalf("artist after update: %+v", got)
	}

	// delete：dry_run 与应用。
	res, _, err = s.handleDeleteArtist(ctx, nil, DeleteArtistInput{ID: id, DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("delete dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleDeleteArtist(ctx, nil, DeleteArtistInput{ID: id, DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("delete: %v %v", res, err)
	}
	if _, err := s.db.GetArtist(id); err == nil {
		t.Fatal("artist should be gone after delete")
	}
}

// ---------- drama CRUD ----------

func TestDramaCRUD(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// create：name 必填 + 多剧种。
	if res, _, _ := s.handleCreateDrama(ctx, nil, CreateDramaInput{}); !res.IsError {
		t.Fatal("blank name should be rejected")
	}
	res, _, err := s.handleCreateDrama(ctx, nil, CreateDramaInput{
		Name: "1699·桃花扇", CategoryNames: StringOrArray{"昆曲", "传奇"}, DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleCreateDrama(ctx, nil, CreateDramaInput{
		Name: "1699·桃花扇", CategoryNames: StringOrArray{"昆曲", "传奇"},
		DryRun: boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("create: %v %v", res, err)
	}
	d := resultMap(t, res)
	dramaID, _ := d["id"].(string)
	if dramaID == "" {
		t.Fatalf("drama missing id: %v", d)
	}
	got, _ := s.db.GetDrama(dramaID)
	if len(got.CategoryNames) != 2 {
		t.Fatalf("drama category_names not persisted: %+v", got.CategoryNames)
	}

	// update：剧种覆盖 + 备注 + 清空覆盖（空数组）。
	res, _, err = s.handleUpdateDrama(ctx, nil, UpdateDramaInput{
		ID: dramaID, CategoryNames: []string{"昆曲"}, Remark: strPtrT("南昆代表作"), DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("update dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleUpdateDrama(ctx, nil, UpdateDramaInput{
		ID: dramaID, CategoryNames: []string{"昆曲"}, Remark: strPtrT("南昆代表作"),
		DryRun: boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("update: %v %v", res, err)
	}
	got, _ = s.db.GetDrama(dramaID)
	if len(got.CategoryNames) != 1 || got.CategoryNames[0] != "昆曲" || got.Remark != "南昆代表作" {
		t.Fatalf("drama after update: %+v", got)
	}
	// 空白名称应被拒绝（已由 server_test 覆盖，这里再确认 update 路径）。
	if res, _, _ := s.handleUpdateDrama(ctx, nil, UpdateDramaInput{ID: dramaID, Name: strPtrT(" ")}); !res.IsError {
		t.Fatal("blank drama name should be rejected")
	}

	// delete：连带折子一起删。
	if _, err := s.db.CreateZhezi(models.Zhezi{DramaID: dramaID, Name: "眠香"}); err != nil {
		t.Fatal(err)
	}
	if res, _, err := s.handleDeleteDrama(ctx, nil, DeleteDramaInput{ID: dramaID, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("delete drama: %v %v", res, err)
	}
	if _, err := s.db.GetDrama(dramaID); err == nil {
		t.Fatal("drama should be gone")
	}
	zs, _ := s.db.ListZhezisByDrama(dramaID)
	if len(zs) != 0 {
		t.Fatalf("zhezis should be cascade-deleted, got %d", len(zs))
	}
}

// ---------- category CRUD ----------

func TestCategoryCRUD(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()

	// list 初始为空或已有数据，至少能返回。
	if _, _, err := s.handleListCategories(ctx, nil, ListCategoriesInput{}); err != nil {
		t.Fatalf("list_categories: %v", err)
	}

	// create：name 必填。
	if res, _, _ := s.handleCreateCategory(ctx, nil, CreateCategoryInput{}); !res.IsError {
		t.Fatal("blank name should be rejected")
	}
	res, _, err := s.handleCreateCategory(ctx, nil, CreateCategoryInput{Name: "昆曲", DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleCreateCategory(ctx, nil, CreateCategoryInput{Name: "昆曲", DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("create: %v %v", res, err)
	}
	c := resultMap(t, res)
	catID, _ := c["id"].(string)
	if catID == "" {
		t.Fatalf("category missing id: %v", c)
	}

	// update。
	res, _, err = s.handleUpdateCategory(ctx, nil, UpdateCategoryInput{ID: catID, Name: strPtrT("昆剧"), DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("update dry run: %v %v", res, err)
	}
	res, _, err = s.handleUpdateCategory(ctx, nil, UpdateCategoryInput{ID: catID, Name: strPtrT("昆剧"), DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("update: %v %v", res, err)
	}
	cats, _ := s.db.ListCategories()
	found := false
	for _, cc := range cats {
		if cc.ID == catID && cc.Name == "昆剧" {
			found = true
		}
	}
	if !found {
		t.Fatalf("category not updated: %+v", cats)
	}

	// delete。
	res, _, err = s.handleDeleteCategory(ctx, nil, DeleteCategoryInput{ID: catID, DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("delete dry run: %v %v", res, err)
	}
	if _, _, err := s.handleDeleteCategory(ctx, nil, DeleteCategoryInput{ID: catID, DryRun: boolPtr(false)}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cats, _ = s.db.ListCategories()
	for _, cc := range cats {
		if cc.ID == catID {
			t.Fatal("category should be gone")
		}
	}
}

// ---------- zhezi extras ----------

func TestZheziCreateAndUpdateExtras(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	d, err := s.db.SaveDrama(models.Drama{Name: "临川四梦"})
	if err != nil {
		t.Fatal(err)
	}

	// 空 names 报错。
	if res, _, _ := s.handleBatchCreateZhezis(ctx, nil, BatchCreateZhezisInput{DramaID: d.ID, Names: StringOrArray{}}); !res.IsError {
		t.Fatal("empty names should be rejected")
	}
	// dry_run 模式：只预览不写库。
	res, _, err := s.handleBatchCreateZhezis(ctx, nil, BatchCreateZhezisInput{
		DramaID: d.ID, Names: StringOrArray{"邯郸梦", "南柯梦"}, DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("dry run: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	if zs, _ := s.db.ListZhezisByDrama(d.ID); len(zs) != 0 {
		t.Fatalf("dry run must not write; got %d", len(zs))
	}

	// update_zhezi：空名不覆盖（保留原值）；别名/备注正常写入。
	z, _ := s.db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "紫钗记"})
	res, _, err = s.handleUpdateZhezi(ctx, nil, UpdateZheziInput{
		ID: z.ID, Name: strPtrT("   "), Aliases: []string{"折柳"}, Remark: strPtrT("经典"),
		DryRun: boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("update zhezi: %v %v", res, err)
	}
	got, _ := s.db.GetZhezi(z.ID)
	if got.Name != "紫钗记" || len(got.Aliases) != 1 || got.Remark != "经典" {
		t.Fatalf("zhezi after update: %+v", got)
	}
}

// ---------- search filters ----------

func TestSearchRecordsFilters(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 19, 30, 0, 0, time.UTC).Unix()
	rows := []struct {
		id, name, city, cat string
	}{
		{"s1", "牡丹亭", "上海", "昆曲"},
		{"s2", "长生殿", "北京", "昆曲"},
		{"s3", "桃花扇", "南京", "昆曲"},
		{"s4", "白蛇传", "上海", "京剧"},
		{"s5", "锁麟囊", "上海", "京剧"},
	}
	for i, r := range rows {
		mustUpsert(t, s, models.Record{
			ID: r.id, Name: r.name, City: r.city,
			CategoryName: r.cat, CategoryNames: []string{r.cat}, Date: base + int64(i),
		})
	}

	// 关键词（命中 name）。
	res, _, err := s.handleSearchRecords(ctx, nil, SearchRecordsInput{Query: "牡丹亭"})
	if err != nil || res.IsError {
		t.Fatalf("query: %v %v", res, err)
	}
	if num(t, resultMap(t, res), "total") != 1 {
		t.Fatalf("query total want 1")
	}

	// 城市精确过滤。
	res, _, _ = s.handleSearchRecords(ctx, nil, SearchRecordsInput{City: "上海"})
	if num(t, resultMap(t, res), "total") != 3 {
		t.Fatalf("city 上海 want 3, got %v", resultMap(t, res)["total"])
	}

	// 剧种（category_names 数组）过滤。
	res, _, _ = s.handleSearchRecords(ctx, nil, SearchRecordsInput{Category: "昆曲"})
	if num(t, resultMap(t, res), "total") != 3 {
		t.Fatalf("category 昆曲 want 3, got %v", resultMap(t, res)["total"])
	}

	// limit / 截断。
	res, _, _ = s.handleSearchRecords(ctx, nil, SearchRecordsInput{Limit: 2})
	m := resultMap(t, res)
	if num(t, m, "total") != 5 || num(t, m, "returned") != 2 {
		t.Fatalf("limit wrong: %v", m)
	}
	if m["truncated"] != true {
		t.Fatalf("expected truncated=true, got %v", m["truncated"])
	}

	// 未知剧名走 resolveDrama 报错路径（工具级错误，而非协议错误）。
	res, _, err = s.handleSearchRecords(ctx, nil, SearchRecordsInput{DramaName: "不存在的剧目"})
	if err != nil {
		t.Fatalf("should be tool-level error: %v", err)
	}
	if !res.IsError {
		t.Fatal("unknown drama name should produce IsError")
	}
}

// TestDryRunDefaultsToPreview locks in the MCP safety contract requested by the
// user: a mutating tool called WITHOUT specifying dry_run must only preview and
// leave the database untouched. Callers must pass dry_run:false to actually apply.
func TestDryRunDefaultsToPreview(t *testing.T) {
	s := newTestServer(t)
	recID, _, _, _ := seedBasicData(t, s)
	ctx := context.Background()

	// 不传 dry_run：update 仅预览，库不变。
	res, _, err := s.handleUpdateRecord(ctx, nil, UpdateRecordInput{
		ID:   recID,
		Name: strPtrT("不应落库"),
	})
	if err != nil || res.IsError {
		t.Fatalf("update: %v %v", res, err)
	}
	m := resultMap(t, res)
	if m["dry_run"] != true {
		t.Fatalf("default dry_run should be true, got %v", m)
	}
	got, _ := s.db.GetRecord(recID)
	if got.Name != "牡丹亭" {
		t.Fatalf("default must not persist; name = %q", got.Name)
	}

	// 不传 dry_run：delete 仅预览，记录仍在。
	res, _, err = s.handleDeleteRecord(ctx, nil, DeleteRecordInput{ID: recID})
	if err != nil || res.IsError {
		t.Fatalf("delete: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("default dry_run should be true for delete")
	}
	if _, err := s.db.GetRecord(recID); err != nil {
		t.Fatal("default delete must not remove the record")
	}
}

// ---------- helper unit tests ----------

func TestApplyArrayOp(t *testing.T) {
	cur := []string{"a", "b"}

	if got := applyArrayOp(cur, nil); len(got) != 2 {
		t.Fatalf("nil op must return current, got %v", got)
	}
	if got := applyArrayOp(cur, &ArrayOp{Op: "set", Value: []string{"x", "y"}}); len(got) != 2 || got[0] != "x" {
		t.Fatalf("set failed: %v", got)
	}
	// append 去重，不重复添加已有元素。
	got := applyArrayOp(cur, &ArrayOp{Op: "append", Value: []string{"b", "c"}})
	if len(got) != 3 || got[2] != "c" {
		t.Fatalf("append dedupe failed: %v", got)
	}
	// remove。
	got = applyArrayOp(cur, &ArrayOp{Op: "remove", Value: []string{"a"}})
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("remove failed: %v", got)
	}
	// 未知 op 视为 no-op。
	if got := applyArrayOp(cur, &ArrayOp{Op: "bogus", Value: []string{"z"}}); len(got) != 2 {
		t.Fatalf("unknown op should be no-op, got %v", got)
	}
}

func TestStringOrArrayUnmarshal(t *testing.T) {
	var arr StringOrArray
	if err := json.Unmarshal([]byte(`["x","y","z"]`), &arr); err != nil || len(arr) != 3 {
		t.Fatalf("array form failed: %v %v", arr, err)
	}
	var str StringOrArray
	if err := json.Unmarshal([]byte(`"a, b , c"`), &str); err != nil || len(str) != 3 || str[1] != "b" {
		t.Fatalf("comma-string form failed: %v %v", str, err)
	}
	var empty StringOrArray
	if err := json.Unmarshal([]byte(`""`), &empty); err != nil || len(empty) != 0 {
		t.Fatalf("empty string should yield no elements: %v %v", empty, err)
	}
	var arrEmpty StringOrArray
	if err := json.Unmarshal([]byte(`[]`), &arrEmpty); err != nil || len(arrEmpty) != 0 {
		t.Fatalf("empty array failed: %v %v", arrEmpty, err)
	}
}

func TestFindArtistEdgeCases(t *testing.T) {
	s := newTestServer(t)
	// findArtist 会 trim 查询词：存入干净名称，用带空格的查询应命中。
	if _, err := s.db.SaveArtist(models.Artist{Name: "周雪峰"}); err != nil {
		t.Fatal(err)
	}
	a, _, err := s.findArtist("  周雪峰  ")
	if err != nil || a == nil {
		t.Fatalf("trimmed query match failed: %v", err)
	}
	// 空名报错。
	if _, _, err := s.findArtist("   "); err == nil {
		t.Fatal("blank name should error")
	}
}

// ---------- cover handlers ----------

// seedCoverFiles writes records referencing the given cover files and registers
// matching cover metadata so the cover tools have something to operate on.
func seedCoverFiles(t *testing.T, s *Server, files ...string) {
	t.Helper()
	for i, f := range files {
		id := fmt.Sprintf("rec-cover-%d", i)
		mustUpsert(t, s, models.Record{ID: id, Name: "封面测试", CoverFile: f})
		if err := s.db.UpsertCoverMeta("hash-"+f, f, filepath.Ext(f), int64(100+i)); err != nil {
			t.Fatalf("UpsertCoverMeta %s: %v", f, err)
		}
	}
}

func TestCoverHandlers(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	seedCoverFiles(t, s, "covers/a.avif", "covers/b.avif")

	// list_covers：返回封面 + total。
	res, _, err := s.handleListCovers(ctx, nil, ListCoversInput{})
	if err != nil || res.IsError {
		t.Fatalf("list_covers: %v %v", res, err)
	}
	m := resultMap(t, res)
	if num(t, m, "total") != 2 {
		t.Fatalf("cover total want 2, got %v", m["total"])
	}

	// cover_duplicates：两个不同文件、相同 hash → 应成组。
	if err := s.db.UpsertCoverMeta("duphash", "covers/a.avif", ".avif", 100); err == nil {
		// 覆盖为相同 hash 之前已存在，确保 a、b 同 hash。
	}
	if err := s.db.UpsertCoverMeta("duphash", "covers/b.avif", ".avif", 100); err != nil {
		t.Fatal(err)
	}
	res, _, err = s.handleCoverDuplicates(ctx, nil, CoverDuplicatesInput{})
	if err != nil || res.IsError {
		t.Fatalf("cover_duplicates: %v %v", res, err)
	}
	m = resultMap(t, res)
	if num(t, m, "count") != 1 {
		t.Fatalf("dup groups want 1, got %v", m["count"])
	}

	// merge_covers：dry_run 预览 → 应用后引用全部指向 target，source meta 删除，重复组消失。
	res, _, err = s.handleMergeCovers(ctx, nil, MergeCoversInput{
		Target: "covers/b.avif", Sources: []string{"covers/a.avif"}, DryRun: boolPtr(true),
	})
	if err != nil || res.IsError {
		t.Fatalf("merge dry run: %v %v", res, err)
	}
	if resultMap(t, res)["dry_run"] != true {
		t.Fatal("expected dry_run flag")
	}
	res, _, err = s.handleMergeCovers(ctx, nil, MergeCoversInput{
		Target: "covers/b.avif", Sources: []string{"covers/a.avif"},
		DryRun: boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("merge: %v %v", res, err)
	}

	// 两条记录的封面都应改为 target。
	for _, id := range []string{"rec-cover-0", "rec-cover-1"} {
		got, _ := s.db.GetRecord(id)
		if got.CoverFile != "covers/b.avif" {
			t.Fatalf("%s cover_file = %q, want covers/b.avif", id, got.CoverFile)
		}
	}
	// source meta 被删，target meta 仍在。
	if s.db.CoverMetaExists("covers/a.avif") {
		t.Fatal("source cover meta should be deleted")
	}
	if !s.db.CoverMetaExists("covers/b.avif") {
		t.Fatal("target cover meta should remain")
	}
	// 重复组应消失。
	res, _, _ = s.handleCoverDuplicates(ctx, nil, CoverDuplicatesInput{})
	if num(t, resultMap(t, res), "count") != 0 {
		t.Fatalf("dup groups should be 0 after merge, got %v", resultMap(t, res)["count"])
	}

	// cover_orphans / cleanup_covers：被引用的封面不算孤立，不清理。
	res, _, err = s.handleCoverOrphans(ctx, nil, CoverOrphansInput{})
	if err != nil || res.IsError {
		t.Fatalf("cover_orphans: %v %v", res, err)
	}
	if num(t, resultMap(t, res), "count") != 0 {
		t.Fatalf("no orphans expected, got %v", resultMap(t, res)["count"])
	}
	res, _, err = s.handleCleanupCovers(ctx, nil, CleanupCoversInput{})
	if err != nil || res.IsError {
		t.Fatalf("cleanup_covers: %v %v", res, err)
	}
	if num(t, resultMap(t, res), "count") != 0 {
		t.Fatalf("cleanup should remove 0 referenced covers, got %v", resultMap(t, res)["count"])
	}
	if !s.db.CoverMetaExists("covers/b.avif") {
		t.Fatal("referenced cover must survive cleanup")
	}
}
