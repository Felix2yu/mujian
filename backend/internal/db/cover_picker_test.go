package db

import (
	"mujian/internal/models"
	"testing"
	"time"
)

// TestListCoverPickerPaginationDeterminism 覆盖封面选择器的数据源：
// 大量封面 ref_count 相同（多数为 1），若 ORDER BY 缺少唯一兜底列，SQLite
// 的返回顺序不确定，翻页会出现重复项与漏项。这里逐页遍历并断言全覆盖且无重复。
func TestListCoverPickerPaginationDeterminism(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()

	const n = 25
	for i := 0; i < n; i++ {
		id := string(rune('a'+i%26)) + string(rune('0'+i/26))
		rec := models.Record{
			ID:           "rec-" + id,
			Name:         "演出" + id,
			CategoryName: "昆剧",
			CoverFile:    "covers/" + id + ".avif",
			Date:         base + int64(i),
			ActiveStatus: 1,
		}
		if err := db.UpsertRecord(rec); err != nil {
			t.Fatalf("UpsertRecord %v: %v", id, err)
		}
	}

	// 一批 ref_count=2 的封面，制造排序竞争。
	for i := 0; i < 5; i++ {
		id := string(rune('m' + i))
		for j := 0; j < 2; j++ {
			rec := models.Record{
				ID:           "dup-" + id + "-" + string(rune('0'+j)),
				Name:         "重复封面" + id,
				CategoryName: "京剧",
				CoverFile:    "covers/" + id + ".avif",
				Date:         base + int64(j),
				ActiveStatus: 1,
			}
			if err := db.UpsertRecord(rec); err != nil {
				t.Fatalf("UpsertRecord dup %v: %v", id, err)
			}
		}
	}

	_, total, err := db.ListCoverPicker("", 10, 0)
	if err != nil {
		t.Fatalf("ListCoverPicker: %v", err)
	}
	if total != 30 {
		t.Fatalf("expected 30 distinct covers, got %d", total)
	}

	seen := map[string]bool{}
	const limit = 7
	for page := 0; page < (total+limit-1)/limit; page++ {
		refs, pageTotal, err := db.ListCoverPicker("", limit, page*limit)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if pageTotal != total {
			t.Errorf("page %d: total drifted to %d", page, pageTotal)
		}
		if len(refs) == 0 {
			t.Fatalf("page %d returned no rows (pagination broke)", page)
		}
		for _, r := range refs {
			if seen[r.FileName] {
				t.Fatalf("duplicate cover across pages: %s", r.FileName)
			}
			seen[r.FileName] = true
			if r.RefCount == 0 {
				t.Errorf("unexpected ref_count for %s", r.FileName)
			}
			if r.SampleName == "" {
				t.Errorf("missing sample name for %s", r.FileName)
			}
		}
	}
	if len(seen) != total {
		t.Errorf("pagination covered %d of %d covers", len(seen), total)
	}
}

// TestListCoverPickerFilters 覆盖搜索语义：按演出名/分类命中，% 与 _ 应作为
// 字面量处理（否则输入 % 会退化成「匹配全部」）。
func TestListCoverPickerFilters(t *testing.T) {
	db := newTestDB(t)
	base := time.Now().Unix()
	seed := []models.Record{
		{ID: "r1", Name: "牡丹亭", CategoryName: "昆剧", CoverFile: "covers/a.avif", Date: base, ActiveStatus: 1},
		{ID: "r2", Name: "长生殿", CategoryName: "京剧", CoverFile: "covers/b.avif", Date: base, ActiveStatus: 1},
		{ID: "r3", Name: "100%哈姆雷特", CategoryName: "话剧", CoverFile: "covers/c.avif", Date: base, ActiveStatus: 1},
		// 软删除的记录不应出现在选择器里。
		{ID: "r4", Name: "已删演出", CategoryName: "昆剧", CoverFile: "covers/d.avif", Date: base, ActiveStatus: 1},
	}
	for _, r := range seed {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("seed %s: %v", r.ID, err)
		}
	}
	if err := db.SoftDeleteRecord("r4"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	cases := []struct {
		q    string
		want int
	}{
		{"", 3},
		{"牡丹", 1},
		{"昆剧", 1},
		{"不存在的关键字", 0},
		{"100%", 1}, // % 是字面量（未转义会退化成匹配全部，返回 3）
		{"100", 1},  // 前缀命中
		{"%哈姆", 1},  // 字面 % 命中名字中间的那个字符
		{"_京", 0},   // _ 是字面量：作为通配符会匹配「京剧」分类
		{"已删", 0},   // 软删除不可见
	}
	for _, c := range cases {
		_, total, err := db.ListCoverPicker(c.q, 20, 0)
		if err != nil {
			t.Fatalf("q=%q: %v", c.q, err)
		}
		if total != c.want {
			t.Errorf("q=%q: total=%d want=%d", c.q, total, c.want)
		}
	}
}
