package mcp

import (
	"context"
	"testing"
	"time"

	"mujian/internal/models"
)

// seedCoverageData writes one of each entity so the read-only handlers have
// something to return.
func seedCoverageData(t *testing.T, s *Server) (recordID, artistID, dramaID string) {
	t.Helper()
	base := time.Date(2026, 3, 1, 19, 30, 0, 0, time.UTC).Unix()
	mustUpsert(t, s, models.Record{
		ID: "rec-cov", Name: "牡丹亭", City: "上海",
		Address: "上海大剧院", CategoryName: "昆曲", CategoryNames: []string{"昆曲"},
		ArtistNames: []string{"张军"},
		Coordinate:  &models.Coordinate{Latitude: 31.2, Longitude: 121.4},
		Date:        base,
	})
	a, err := s.db.SaveArtist(models.Artist{Name: "张军"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	d, err := s.db.SaveDrama(models.Drama{Name: "牡丹亭", CategoryNames: []string{"昆曲"}})
	if err != nil {
		t.Fatalf("SaveDrama: %v", err)
	}
	z, err := s.db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "游园", Aliases: []string{}})
	if err != nil {
		t.Fatalf("CreateZhezi: %v", err)
	}
	return "rec-cov", a.ID, z.ID
}

func TestQueryHandlers(t *testing.T) {
	s := newTestServer(t)
	recID, artistID, _ := seedCoverageData(t, s)
	ctx := context.Background()

	// get_record：命中与未命中。
	if res, _, err := s.handleGetRecord(ctx, nil, IDInput{ID: recID}); err != nil || res.IsError {
		t.Fatalf("get_record: %v %v", res, err)
	}
	if res, _, _ := s.handleGetRecord(ctx, nil, IDInput{ID: "missing"}); !res.IsError {
		t.Fatal("missing record should error")
	}

	// list_artists / list_dramas / get_stats。
	if _, _, err := s.handleListArtists(ctx, nil, noInput{}); err != nil {
		t.Fatalf("list_artists: %v", err)
	}
	if _, _, err := s.handleListDramas(ctx, nil, noInput{}); err != nil {
		t.Fatalf("list_dramas: %v", err)
	}
	if _, _, err := s.handleGetStats(ctx, nil, noInput{}); err != nil {
		t.Fatalf("get_stats: %v", err)
	}

	// get_artist_detail：按 id、按名称、缺参三种路径。
	if res, _, err := s.handleGetArtistDetail(ctx, nil, NameOrIDInput{ID: artistID}); err != nil || res.IsError {
		t.Fatalf("artist by id: %v %v", res, err)
	}
	if res, _, err := s.handleGetArtistDetail(ctx, nil, NameOrIDInput{Name: "张军"}); err != nil || res.IsError {
		t.Fatalf("artist by name: %v %v", res, err)
	}
	if res, _, _ := s.handleGetArtistDetail(ctx, nil, NameOrIDInput{}); !res.IsError {
		t.Fatal("empty input should error")
	}
	if res, _, _ := s.handleGetArtistDetail(ctx, nil, NameOrIDInput{Name: "不存在"}); !res.IsError {
		t.Fatal("unknown artist should error")
	}

	// get_drama_detail：按名称解析 + 未知名报错。
	if res, _, err := s.handleGetDramaDetail(ctx, nil, NameOrIDInput{Name: "牡丹亭"}); err != nil || res.IsError {
		t.Fatalf("drama detail: %v %v", res, err)
	}
	if res, _, _ := s.handleGetDramaDetail(ctx, nil, NameOrIDInput{Name: "不存在"}); !res.IsError {
		t.Fatal("unknown drama should error")
	}

	// value_counts：合法字段 + 越界字段。
	if res, _, err := s.handleValueCounts(ctx, nil, ValueCountsInput{Field: "company"}); err != nil || res.IsError {
		t.Fatalf("value_counts: %v %v", res, err)
	}
	if res, _, _ := s.handleValueCounts(ctx, nil, ValueCountsInput{Field: "password"}); !res.IsError {
		t.Fatal("unsupported field should error")
	}
}

func TestMutationHandlers(t *testing.T) {
	s := newTestServer(t)
	recID, _, zheziID := seedCoverageData(t, s)
	ctx := context.Background()

	// batch_update_records 标量 + 数组混合更新。
	res, _, err := s.handleBatchUpdateRecords(ctx, nil, BatchUpdateRecordsInput{
		IDs:           []string{recID},
		Name:          strPtrT("牡丹亭·纪念场"),
		Price:         fltPtrT(199),
		CategoryNames: &ArrayOp{Op: "append", Value: []string{"苏剧"}},
		DryRun:        boolPtr(false),
	})
	if err != nil || res.IsError {
		t.Fatalf("batch_update_records: %v %v", res, err)
	}
	got, _ := s.db.GetRecord(recID)
	if got.Name != "牡丹亭·纪念场" || got.Price != 199 || len(got.CategoryNames) != 2 {
		t.Fatalf("batch result: %+v", got)
	}

	// update_zhezi 改别名；delete_zhezi 删除后再次删除应报错。
	newName := "游园·惊梦"
	if res, _, err := s.handleUpdateZhezi(ctx, nil, UpdateZheziInput{
		ID: zheziID, Name: &newName, Aliases: []string{"堆花"}, Remark: strPtrT("经典折子"),
		DryRun: boolPtr(false),
	}); err != nil || res.IsError {
		t.Fatalf("update_zhezi: %v %v", res, err)
	}
	z, err := s.db.GetZhezi(zheziID)
	if err != nil || z.Name != "游园·惊梦" || len(z.Aliases) != 1 {
		t.Fatalf("zhezi after update: %+v %v", z, err)
	}

	if res, _, err := s.handleDeleteZhezi(ctx, nil, DeleteZheziInput{ID: zheziID, DryRun: boolPtr(false)}); err != nil || res.IsError {
		t.Fatalf("delete_zhezi: %v %v", res, err)
	}
	if res, _, _ := s.handleDeleteZhezi(ctx, nil, DeleteZheziInput{ID: zheziID, DryRun: boolPtr(false)}); !res.IsError {
		t.Fatal("deleting twice should error")
	}
}

func fltPtrT(f float64) *float64 { return &f }
