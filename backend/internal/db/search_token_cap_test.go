package db

import (
	"strings"
	"testing"
	"time"

	"mujian/internal/models"
)

// 关键词分词数上限：appendSearchPredicate 会为每个词复制一份 12 列 OR，词数
// 线性放大成 12×N 个子句；粘贴长文本会把 WHERE 炸成数百个全表 LIKE。超过
// maxSearchTokens 只取前 N 个词，SearchQueryTruncated 供 UI 提示已截断。
func TestSearchTokenCap(t *testing.T) {
	if MaxSearchTokens != 8 {
		t.Fatalf("MaxSearchTokens = %d, want 8", MaxSearchTokens)
	}
	join := func(n int) string { return strings.TrimSpace(strings.Repeat("词 ", n)) }

	// SearchQueryTruncated 判定
	for _, n := range []int{0, 1, 3, MaxSearchTokens, MaxSearchTokens + 1, 50} {
		want := n > MaxSearchTokens
		if got := SearchQueryTruncated(join(n)); got != want {
			t.Errorf("SearchQueryTruncated(%d tokens) = %v, want %v", n, got, want)
		}
	}

	// appendSearchPredicate 只展开前 maxSearchTokens 个词 → groups=tokens, args=tokens×12
	for _, n := range []int{1, 5, MaxSearchTokens, MaxSearchTokens + 1, 50} {
		var where []string
		var args []interface{}
		appendSearchPredicate(RecordFilter{Query: join(n)}, &where, &args)
		wantTokens := n
		if wantTokens > MaxSearchTokens {
			wantTokens = MaxSearchTokens
		}
		if len(where) != wantTokens || len(args) != wantTokens*12 {
			t.Errorf("%d-token query: groups=%d args=%d, want groups=%d args=%d",
				n, len(where), len(args), wantTokens, wantTokens*12)
		}
	}

	// Exact 路径按整串等值匹配，不分词、不受上限影响。
	var where []string
	var args []interface{}
	appendSearchPredicate(RecordFilter{Query: join(50), Exact: true}, &where, &args)
	if len(where) != 1 || len(args) != 1 {
		t.Errorf("Exact path: groups=%d args=%d, want 1/1", len(where), len(args))
	}
}

// 超过上限的长查询其结果必须等于「只取前 maxSearchTokens 个词」的查询——
// 即截断是确定性的前 N 个词，结果为其超集，行为可预期。
func TestSearchTokenCapFunctionalParity(t *testing.T) {
	d := newTestDB(t)
	base := time.Date(2026, 8, 1, 19, 30, 0, 0, time.UTC).Unix()
	rec, err := d.CreateRecord(models.RecordRequest{
		Name: "限词回归·京剧", City: "上海", Company: "上昆", Channel: "大麦",
		Remark: "精彩 演出 回顾 值得 二刷", Date: base,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 12 个词，前 8 个包含能命中的字段词，后 4 个是不存在的词。
	full := "限词回归 京剧 上海 上昆 大麦 精彩 演出 回顾 值得 二刷 甲 乙"
	first8 := "限词回归 京剧 上海 上昆 大麦 精彩 演出 回顾"

	fullRows, err := d.ListRecords(RecordFilter{Query: full})
	if err != nil {
		t.Fatalf("ListRecords(full): %v", err)
	}
	first8Rows, err := d.ListRecords(RecordFilter{Query: first8})
	if err != nil {
		t.Fatalf("ListRecords(first8): %v", err)
	}
	if len(fullRows) != len(first8Rows) {
		t.Fatalf("truncation changed result set: full(12 tok)=%d, first8=%d", len(fullRows), len(first8Rows))
	}
	if len(fullRows) != 1 || fullRows[0].ID != rec.ID {
		t.Fatalf("expected the seeded record, got %d rows", len(fullRows))
	}
	if !SearchQueryTruncated(full) {
		t.Errorf("SearchQueryTruncated(12 tokens) = false, want true")
	}
	if SearchQueryTruncated(first8) {
		t.Errorf("SearchQueryTruncated(8 tokens) = true, want false")
	}
}
