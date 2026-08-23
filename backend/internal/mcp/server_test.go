package mcp

import (
	"context"
	"encoding/json"
	"mujian/internal/db"
	"mujian/internal/models"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(database.Close)
	return New(database)
}

func mustUpsert(t *testing.T, s *Server, r models.Record) {
	t.Helper()
	if err := s.db.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord %s: %v", r.ID, err)
	}
}

// resultMap extracts the tool's JSON text content into a map for assertions.
// Error-flagged results fail the test with their raw text.
func resultMap(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("nil result")
	}
	if len(res.Content) == 0 {
		t.Fatal("empty content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", text.Text)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		t.Fatalf("unmarshal tool output: %v\nraw: %s", err, text.Text)
	}
	return m
}

func num(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key].(float64)
	if !ok {
		t.Fatalf("key %q = %#v, want number (map=%v)", key, m[key], m)
	}
	return v
}

func seedVenueAndTroupeData(t *testing.T, s *Server) {
	t.Helper()
	base := time.Date(2026, 1, 1, 19, 30, 0, 0, time.UTC).Unix()
	day := int64(24 * 3600)

	// 张军 的三场演出，剧团名各不相同 —— 模拟待统一的脏数据。
	for i, c := range []string{"上昆", "上海昆剧团", ""} {
		mustUpsert(t, s, models.Record{
			ID: "rec-zhangjun-" + string(rune('a'+i)), Name: "牡丹亭", City: "上海",
			Address: "上海大剧院", CategoryName: "昆曲",
			ArtistNames: []string{"张军"}, Company: c,
			Coordinate: &models.Coordinate{Latitude: 31.2, Longitude: 121.4},
			Date:       base + int64(i)*day,
		})
	}
	// 无关演员的演出，不应被误伤。
	mustUpsert(t, s, models.Record{
		ID: "rec-other", Name: "长生殿", City: "北京",
		Address: "天桥艺术中心", ArtistNames: []string{"魏春荣"},
		Company: "北昆", Date: base + 10*day,
	})

	// 场馆近似重名：「上海大剧院（西店）」应可并入「上海大剧院」，并继承坐标。
	mustUpsert(t, s, models.Record{
		ID: "rec-venue-west", Name: "牡丹亭", City: "上海",
		Address: "上海大剧院（西店）", ArtistNames: []string{"张军"},
		Date: base + 20*day,
	})
}

func TestBatchUpdateCompanyByArtistDryRunThenApply(t *testing.T) {
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	// dry_run：只预览不写入。
	res, _, err := s.handleBatchUpdateCompanyByArtist(context.Background(), nil,
		BatchCompanyByArtistInput{ArtistName: "张军", Company: "上海昆剧团", DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	m := resultMap(t, res)
	// 三场主记录 + 西店一场，均含张军。
	if got := num(t, m, "matched"); got != 4 {
		t.Fatalf("matched = %v, want 4", got)
	}
	if got := num(t, m, "will_change"); got != 3 { // b 已是目标值
		t.Fatalf("will_change = %v, want 3", got)
	}
	rec, err := s.db.GetRecord("rec-zhangjun-a")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Company != "上昆" {
		t.Fatalf("dry run should not write; company = %q", rec.Company)
	}

	// 正式执行。
	res, _, err = s.handleBatchUpdateCompanyByArtist(context.Background(), nil,
		BatchCompanyByArtistInput{ArtistName: "张军", Company: "上海昆剧团"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	m = resultMap(t, res)
	if got := num(t, m, "updated"); got != 4 {
		t.Fatalf("updated = %v, want 4", got)
	}
	for _, id := range []string{"rec-zhangjun-a", "rec-zhangjun-b", "rec-zhangjun-c", "rec-venue-west"} {
		rec, err := s.db.GetRecord(id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Company != "上海昆剧团" {
			t.Fatalf("%s company = %q, want 上海昆剧团", id, rec.Company)
		}
	}
	// 其他演员的记录不受影响。
	other, _ := s.db.GetRecord("rec-other")
	if other.Company != "北昆" {
		t.Fatalf("unrelated record changed: %q", other.Company)
	}
}

func TestBatchMergeVenues(t *testing.T) {
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	// dry_run。
	res, _, err := s.handleBatchMergeVenues(context.Background(), nil,
		BatchMergeVenuesInput{SourceAddress: "上海大剧院（西店）", TargetAddress: "上海大剧院", SyncCoordinates: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	m := resultMap(t, res)
	if got := num(t, m, "source_records"); got != 1 {
		t.Fatalf("source_records = %v, want 1", got)
	}
	west, _ := s.db.GetRecord("rec-venue-west")
	if west.Address != "上海大剧院（西店）" {
		t.Fatal("dry run should not write")
	}

	// 执行合并 + 坐标同步。
	res, _, err = s.handleBatchMergeVenues(context.Background(), nil,
		BatchMergeVenuesInput{SourceAddress: "上海大剧院（西店）", TargetAddress: "上海大剧院", SyncCoordinates: true})
	if err != nil {
		t.Fatal(err)
	}
	m = resultMap(t, res)
	if got := num(t, m, "updated"); got != 1 {
		t.Fatalf("updated = %v, want 1", got)
	}
	west, _ = s.db.GetRecord("rec-venue-west")
	if west.Address != "上海大剧院" {
		t.Fatalf("address = %q, want merged", west.Address)
	}
	if west.Coordinate == nil || west.Coordinate.Latitude != 31.2 {
		t.Fatalf("coordinate not synced from target venue: %+v", west.Coordinate)
	}
}

func TestBatchCreateZhezisSkipsExisting(t *testing.T) {
	s := newTestServer(t)

	drama, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭", CategoryName: "昆曲"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.CreateZhezi(models.Zhezi{DramaID: drama.ID, Name: "游园"}); err != nil {
		t.Fatal(err)
	}

	res, _, err := s.handleBatchCreateZhezis(context.Background(), nil,
		BatchCreateZhezisInput{DramaID: drama.ID, Names: []string{"游园", "惊梦", "冥判", "拾画"}})
	if err != nil {
		t.Fatal(err)
	}
	m := resultMap(t, res)
	created := m["created"].([]any)
	skipped := m["skipped_exists"].([]any)
	if len(created) != 3 || len(skipped) != 1 {
		t.Fatalf("created=%v skipped=%v", created, skipped)
	}
	if skipped[0] != "游园" {
		t.Fatalf("expected 游园 skipped, got %v", skipped)
	}

	zhezis, err := s.db.ListZhezisByDrama(drama.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(zhezis) != 4 {
		t.Fatalf("zhezis = %d, want 4", len(zhezis))
	}

	// 按名称解析剧目再写一次，别名也不应重复创建。
	res, _, err = s.handleBatchCreateZhezis(context.Background(), nil,
		BatchCreateZhezisInput{DramaName: "牡丹亭", Names: []string{"游园惊梦"}})
	if err != nil {
		t.Fatal(err)
	}
	zhezis, _ = s.db.ListZhezisByDrama(drama.ID)
	if len(zhezis) != 5 {
		t.Fatalf("after second batch zhezis = %d, want 5", len(zhezis))
	}
}

func TestFindArtistMatching(t *testing.T) {
	s := newTestServer(t)
	mustUpsert(t, s, models.Record{ID: "r1", Name: "n", ArtistNames: []string{"张军"}})
	artist, partial, err := s.findArtist("张军")
	if err != nil || artist == nil {
		t.Fatalf("exact match failed: %v %v", err, partial)
	}

	// 别名匹配。
	a2, err := s.db.SaveArtist(models.Artist{ID: artist.ID, Name: artist.Name, Aliases: []string{"军哥"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = a2
	artist, _, err = s.findArtist("军哥")
	if err != nil || artist == nil {
		t.Fatalf("alias match failed: %v", err)
	}

	// 部分匹配返回候选而非报错。
	artist, partial, err = s.findArtist("张")
	if err != nil {
		t.Fatalf("partial match error: %v", err)
	}
	if artist != nil || len(partial) == 0 {
		t.Fatalf("want candidates, got artist=%v partial=%v", artist, partial)
	}

	// 完全未知：findArtist 返回错误。
	artist, partial, err = s.findArtist("不存在的人")
	if artist != nil || partial != nil || err == nil {
		t.Fatalf("unknown artist should error; got artist=%v partial=%v err=%v", artist, partial, err)
	}
}

func TestResolveDrama(t *testing.T) {
	s := newTestServer(t)
	d1, _ := s.db.SaveDrama(models.Drama{Name: "牡丹亭"})
	if _, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭·青春版"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭·典藏版"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.resolveDrama("", "牡丹亭")
	if err != nil || got.ID != d1.ID {
		t.Fatalf("exact resolve: %v %v", got, err)
	}
	// 歧义名称必须报错而不是猜。
	if _, err := s.resolveDrama("", "牡丹亭·"); err == nil {
		t.Fatal("ambiguous name should fail")
	} else if !strings.Contains(err.Error(), "drama_id") {
		t.Fatalf("error should list ids: %v", err)
	}
	// 唯一部分匹配直接采用。
	got, err = s.resolveDrama("", "青春版")
	if err != nil || got.Name != "牡丹亭·青春版" {
		t.Fatalf("unique partial resolve: %v %v", got, err)
	}
}

func TestSearchRecordsByArtistName(t *testing.T) {
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	res, _, err := s.handleSearchRecords(context.Background(), nil,
		SearchRecordsInput{ArtistName: "张军"})
	if err != nil {
		t.Fatal(err)
	}
	m := resultMap(t, res)
	if got := num(t, m, "total"); got != 4 { // 三场 + 西店一场
		t.Fatalf("total = %v, want 4", got)
	}

	// 未知演员报工具级错误（IsError=true），而非协议错误。
	res, _, err = s.handleSearchRecords(context.Background(), nil,
		SearchRecordsInput{ArtistName: "路人"})
	if err != nil {
		t.Fatalf("should be a tool-level error, not protocol: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected isError result for unknown artist")
	}
}

func TestListVenuesGroups(t *testing.T) {
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	groups, err := s.db.ListVenueGroups("")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[0].Address != "上海大剧院" || groups[0].RecordCount != 3 {
		t.Fatalf("busiest venue wrong: %+v", groups[0])
	}
	if !groups[0].HasCoord {
		t.Fatal("上海大剧院 has coordinates")
	}
	if len(groups[1].Cities) == 0 || groups[1].Cities[0] != "上海" {
		t.Fatalf("cities missing: %+v", groups[1])
	}

	filtered, err := s.db.ListVenueGroups("西店")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].Address != "上海大剧院（西店）" {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestGetValueCounts(t *testing.T) {
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	counts, err := s.db.GetValueCounts("company")
	if err != nil {
		t.Fatal(err)
	}
	byValue := map[string]int{}
	for _, c := range counts {
		byValue[c.Value] = c.Count
	}
	// 张军的 3 条被统一前是「上昆」×1、「上海昆剧团」×1、空×1；无关 1 条「北昆」。
	if byValue["上昆"] != 1 || byValue["北昆"] != 1 || byValue["上海昆剧团"] != 1 {
		t.Fatalf("counts = %+v", byValue)
	}
	if _, ok := byValue[""]; ok {
		t.Fatal("empty values must be excluded")
	}

	if _, err := s.db.GetValueCounts("address"); err == nil {
		t.Fatal("non-whitelisted field must be rejected")
	}
}

func TestHTTPRoundTrip(t *testing.T) {
	// 端到端：通过 Streamable HTTP transport（与线上 /mcp 端点相同的配置）
	// 完成 MCP 握手并调用工具，验证远程客户端（如 opencode remote MCP）
	// 将看到的完整链路。
	if testing.Short() {
		t.Skip("short mode")
	}
	s := newTestServer(t)
	seedVenueAndTroupeData(t, s)

	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             srv.URL,
		DisableStandaloneSSE: true, // 服务端为 Stateless，不提供独立 SSE 流
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_records",
		Arguments: map[string]any{"artist_name": "张军"},
	})
	if err != nil {
		t.Fatalf("call search_records: %v", err)
	}
	m := resultMap(t, res)
	if num(t, m, "total") != 4 {
		t.Fatalf("http total = %v, want 4", m["total"])
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_venues",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call list_venues: %v", err)
	}
	m = resultMap(t, res)
	if num(t, m, "total_groups") != 3 {
		t.Fatalf("venue groups = %v, want 3", m["total_groups"])
	}
}
