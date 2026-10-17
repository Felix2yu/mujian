package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mujian/internal/models"
)

// TestSearchRecordsNewFilters covers the filter dimensions added to align the
// MCP tool with the web UI's filter panel: missing/compact/offset/statuses.
func TestSearchRecordsNewFilters(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()
	mustUpsert(t, s, models.Record{ID: "rec-full", Name: "完整记录", City: "上海",
		Address: "上海大剧院", Company: "上海昆剧团", CategoryName: "昆曲",
		CategoryNames: []string{"昆曲"}, Rating: 5, ActiveStatus: 1,
		Date: base})
	mustUpsert(t, s, models.Record{ID: "rec-bare", Name: "缺字段记录", Date: base})

	// missing：数据卫生查询，只命中缺分类/缺城市的记录。
	res, _, err := s.handleSearchRecords(ctx, nil, SearchRecordsInput{Missing: "category,city"})
	if err != nil || res.IsError {
		t.Fatalf("missing search: %v %v", res, err)
	}
	raw := resText(t, res)
	if !strings.Contains(raw, "rec-bare") || strings.Contains(raw, "rec-full") {
		t.Fatalf("missing filter should only match rec-bare: %s", raw)
	}

	// compact：只输出投影字段，且包含 id/name。
	res, _, err = s.handleSearchRecords(ctx, nil, SearchRecordsInput{Query: "完整记录", Compact: true})
	if err != nil || res.IsError {
		t.Fatalf("compact search: %v %v", res, err)
	}
	var out struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal([]byte(resText(t, res)), &out); err != nil {
		t.Fatalf("compact unmarshal: %v", err)
	}
	if len(out.Records) != 1 {
		t.Fatalf("compact records: %v", out.Records)
	}
	for _, key := range []string{"id", "name", "dateText", "address", "artist_names"} {
		if _, ok := out.Records[0][key]; !ok {
			t.Fatalf("compact record missing key %q: %v", key, out.Records[0])
		}
	}
	for _, key := range []string{"remark", "seat", "friends"} {
		if _, ok := out.Records[0][key]; ok {
			t.Fatalf("compact record should not carry %q: %v", key, out.Records[0])
		}
	}

	// offset/limit 分页。
	res, _, _ = s.handleSearchRecords(ctx, nil, SearchRecordsInput{Limit: 1, Offset: 1})
	m := resultMap(t, res)
	if num(t, m, "returned") != 1 {
		t.Fatalf("limit=1 should return 1: %v", m)
	}

	// statuses 多选优先于默认可见状态。
	res, _, err = s.handleSearchRecords(ctx, nil, SearchRecordsInput{Statuses: []int{1}})
	if err != nil || res.IsError {
		t.Fatalf("statuses search: %v %v", res, err)
	}
	if num(t, resultMap(t, res), "total") != 1 {
		t.Fatal("statuses=[1] should match rec-full only")
	}
}

// TestMergeArtistsTool covers duplicate-artist cleanup end to end: dry run
// preview, then the real merge (repoint, dedupe, alias fold, source delete).
func TestMergeArtistsTool(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()

	a1, err := s.db.SaveArtist(models.Artist{Name: "张军", Aliases: []string{"小张"}, Bio: "昆曲演员"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	a2, err := s.db.SaveArtist(models.Artist{Name: "张 军", Aliases: []string{"大军"}})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	mustUpsert(t, s, models.Record{ID: "rec-m1", Name: "独挂记录", Date: base, ArtistIDs: []string{a1.ID}})
	mustUpsert(t, s, models.Record{ID: "rec-m2", Name: "双挂记录", Date: base, ArtistIDs: []string{a1.ID, a2.ID}})

	// dry_run 预览不落库。
	res, _, err := s.handleMergeArtists(ctx, nil, MergeArtistsInput{
		SourceID: a2.ID, TargetID: a1.ID, DryRun: boolPtr(true)})
	if err != nil || res.IsError {
		t.Fatalf("merge dry run: %v %v", res, err)
	}
	if _, err := s.db.GetArtist(a2.ID); err != nil {
		t.Fatal("dry run must not delete source")
	}

	// 真实合并：a2 → a1。
	res, _, err = s.handleMergeArtists(ctx, nil, MergeArtistsInput{SourceID: a2.ID, TargetID: a1.ID, DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("merge: %v %v", res, err)
	}
	if _, err := s.db.GetArtist(a2.ID); err == nil {
		t.Fatal("source artist should be deleted")
	}
	merged, err := s.db.GetArtist(a1.ID)
	if err != nil {
		t.Fatalf("target artist: %v", err)
	}
	// rec-m1 的链接被改挂；rec-m2 与 target 本就关联，仅去掉重复链接。
	detail, err := s.db.GetArtistDetail(a1.ID)
	if err != nil {
		t.Fatalf("artist detail: %v", err)
	}
	if len(detail.Records) != 2 {
		t.Fatalf("target should have 2 records, got %d", len(detail.Records))
	}
	aliasSet := map[string]bool{}
	for _, al := range merged.Aliases {
		aliasSet[al] = true
	}
	for _, want := range []string{"张 军", "大军", "小张"} {
		if !aliasSet[want] {
			t.Fatalf("alias %q not merged: %v", want, merged.Aliases)
		}
	}
	if merged.Bio != "昆曲演员" {
		t.Fatalf("bio should be taken over from source: %q", merged.Bio)
	}

	// 同名/缺参报错路径。
	if res, _, _ := s.handleMergeArtists(ctx, nil, MergeArtistsInput{SourceID: a1.ID, TargetID: a1.ID}); !res.IsError {
		t.Fatal("same-id merge should error")
	}
	if res, _, _ := s.handleMergeArtists(ctx, nil, MergeArtistsInput{SourceID: a1.ID}); !res.IsError {
		t.Fatal("missing target should error")
	}
}

// TestMergeDramasTool covers duplicate-drama cleanup: record links move,
// same-name zhezis dedupe onto the target's, others move.
func TestMergeDramasTool(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()

	d1, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭", Aliases: []string{}})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	d2, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭（全本）", Aliases: []string{"全本牡丹亭"}})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	z1, err := s.db.CreateZhezi(models.Zhezi{DramaID: d1.ID, Name: "游园"})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	zDup, err := s.db.CreateZhezi(models.Zhezi{DramaID: d2.ID, Name: "游园"})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	zMove, err := s.db.CreateZhezi(models.Zhezi{DramaID: d2.ID, Name: "离魂"})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	mustUpsert(t, s, models.Record{ID: "rec-d1", Name: "挂d1", Date: base, DramaIDs: []string{d1.ID}})
	mustUpsert(t, s, models.Record{ID: "rec-d2", Name: "挂d2", Date: base, DramaIDs: []string{d2.ID}, ZheziIDs: []string{zDup.ID}})

	res, _, err := s.handleMergeDramas(ctx, nil, MergeDramasInput{SourceID: d2.ID, TargetID: d1.ID, DryRun: boolPtr(false)})
	if err != nil || res.IsError {
		t.Fatalf("merge dramas: %v %v", res, err)
	}
	if _, err := s.db.GetDrama(d2.ID); err == nil {
		t.Fatal("source drama should be deleted")
	}
	detail, err := s.db.GetDramaDetail(d1.ID)
	if err != nil {
		t.Fatalf("drama detail: %v", err)
	}
	if len(detail.Records) != 2 {
		t.Fatalf("target should have 2 records, got %d", len(detail.Records))
	}
	names := map[string]bool{}
	for _, z := range detail.Zhezis {
		names[z.Name] = true
	}
	if !names["游园"] || !names["离魂"] {
		t.Fatalf("zhezis should contain 游园/离魂, got %v", names)
	}
	// rec-d2 的同名折子链接应改挂到 target 的 游园。
	got, _ := s.db.GetRecord("rec-d2")
	found := false
	for _, id := range got.ZheziIDs {
		if id == z1.ID {
			found = true
		}
		if id == zDup.ID {
			t.Fatalf("deduped zhezi link should be repointed, still %s", id)
		}
	}
	if !found {
		t.Fatalf("record should link to target 游园 %s, got %v", z1.ID, got.ZheziIDs)
	}
	if _, err := s.db.GetZhezi(zMove.ID); err != nil {
		t.Fatalf("moved zhezi should survive: %v", err)
	}
}

// TestBatchUpdateArtistIDs covers the batch artist_ids array op, which acts on
// record_artists (the canonical link table) rather than a records JSON column.
func TestBatchUpdateArtistIDs(t *testing.T) {
	s := newTestServer(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()

	a1, err := s.db.SaveArtist(models.Artist{Name: "甲"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	a2, err := s.db.SaveArtist(models.Artist{Name: "乙"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	mustUpsert(t, s, models.Record{ID: "rec-art", Name: "演员批量", Date: base, ArtistIDs: []string{a1.ID}})

	// append 乙。
	if res, _, err := s.handleBatchUpdateRecords(ctx, nil, BatchUpdateRecordsInput{
		IDs: []string{"rec-art"}, ArtistIDs: &ArrayOp{Op: "append", Value: []string{a2.ID}},
		DryRun: boolPtr(false),
	}); err != nil || res.IsError {
		t.Fatalf("batch append artist_ids: %v %v", res, err)
	}
	got, _ := s.db.GetRecord("rec-art")
	if len(got.ArtistIDs) != 2 {
		t.Fatalf("expected 2 artists, got %v", got.ArtistIDs)
	}

	// remove 甲。
	if res, _, err := s.handleBatchUpdateRecords(ctx, nil, BatchUpdateRecordsInput{
		IDs: []string{"rec-art"}, ArtistIDs: &ArrayOp{Op: "remove", Value: []string{a1.ID}},
		DryRun: boolPtr(false),
	}); err != nil || res.IsError {
		t.Fatalf("batch remove artist_ids: %v %v", res, err)
	}
	got, _ = s.db.GetRecord("rec-art")
	if len(got.ArtistIDs) != 1 || got.ArtistIDs[0] != a2.ID {
		t.Fatalf("expected only 乙, got %v", got.ArtistIDs)
	}
}
