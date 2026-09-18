package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

// TestFilterDimensions exercises the new filter dimensions (channel, company,
// rating threshold, price range, status, exact-match) through the full
// ListRecords path against seeded data.
func TestFilterDimensions(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_760_000_000)

	seed := []models.RecordRequest{
		{Name: "京剧·A", Channel: "线下", Company: "A剧团", Rating: 8, Price: fltPtr(120), ActiveStatus: 0, Date: base + 1},
		{Name: "昆曲·B", Channel: "线上", Company: "B剧团", Rating: 5, Price: fltPtr(50), ActiveStatus: 1, Date: base + 2},
		{Name: "京剧·C", Channel: "线下", Company: "A剧团", Rating: 9, Price: fltPtr(200), ActiveStatus: 2, Date: base + 3},
		{Name: "无渠道", Channel: "", Company: "", Rating: 0, Price: nil, ActiveStatus: 3, Date: base + 4},
		// 多剧团：company 按逗号存多个剧团，按单一剧团筛选必须命中它。
		{Name: "拼团·D", Channel: "", Company: "B剧团,D剧团", Rating: 7, Price: nil, ActiveStatus: 0, Date: base + 5},
	}
	for i, r := range seed {
		if _, err := d.CreateRecord(r); err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}

	count := func(f RecordFilter) int {
		rows, err := d.ListRecords(f)
		if err != nil {
			t.Fatalf("ListRecords(%+v): %v", f, err)
		}
		return len(rows)
	}

	checks := []struct {
		name string
		f    RecordFilter
		want int
	}{
		{"channel=线下", RecordFilter{Channel: "线下"}, 2},
		{"company=A剧团", RecordFilter{Company: "A剧团"}, 2},
		// 多剧团列：逗号分隔的 company 必须被单一剧团命中，否则 multi-value 记录永久搜不到。
		{"company 命中多剧团记录", RecordFilter{Company: "D剧团"}, 1},
		{"company=多剧团记录的首个剧团", RecordFilter{Company: "B剧团"}, 2},
		{"rating>=8", RecordFilter{RatingMin: 8, HasRatingMin: true}, 2},
		{"rating>=0 等价于「有评分」", RecordFilter{RatingMin: 0, HasRatingMin: true}, 5},
		{"price>=100", RecordFilter{PriceMin: 100}, 2},
		{"price<=100", RecordFilter{PriceMax: 100, HasPriceMax: true}, 1}, // NULL 价格不参与比较，只有"昆曲·B"匹配
		{"price 60~150", RecordFilter{PriceMin: 60, PriceMax: 150, HasPriceMax: true}, 1}, // NULL 价格不参与比较，只有"京剧·A"匹配
		{"price<=0 只看免费", RecordFilter{PriceMax: 0, HasPriceMax: true}, 0},
		{"status=1(想看)", RecordFilter{ActiveStatus: 1, HasActiveStatus: true}, 1},
		{"status=0(正常)", RecordFilter{ActiveStatus: 0, HasActiveStatus: true}, 2},
		{"exact 京剧·A", RecordFilter{Query: "京剧·A", Exact: true}, 1},
		{"exact 京剧 (no exact name)", RecordFilter{Query: "京剧", Exact: true}, 0},
		{"fuzzy 京剧", RecordFilter{Query: "京剧"}, 2},
		{"channel=线下 AND rating>=9", RecordFilter{Channel: "线下", RatingMin: 9, HasRatingMin: true}, 1},
		{"status 与 statuses 取交集", RecordFilter{ActiveStatus: 0, HasActiveStatus: true, Statuses: []int{0, 2}}, 2},
	}
	for _, c := range checks {
		if got := count(c.f); got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// TestRecordFilterDateParts pins the year/month/start/end combinators. They
// used to be a single `if year&&month {...} else if start||end {...}` chain, so
// picking a month without a year silently returned every record while the UI
// kept showing a 月份 chip, and combining year+month with a date range silently
// dropped the range. Each dimension must now contribute and intersect.
func TestRecordFilterDateParts(t *testing.T) {
	d := newTestDB(t)
	at := func(y, m, day int) int64 {
		return time.Date(y, time.Month(m), day, 12, 0, 0, 0, time.Local).Unix()
	}
	seed := []models.RecordRequest{
		{Name: "24-03", Date: at(2024, 3, 15)},
		{Name: "25-03", Date: at(2025, 3, 2)},
		{Name: "25-07", Date: at(2025, 7, 20)},
	}
	for i, r := range seed {
		if _, err := d.CreateRecord(r); err != nil {
			t.Fatalf("seed[%d]: %v", i, err)
		}
	}
	count := func(f RecordFilter) int {
		rows, err := d.ListRecords(f)
		if err != nil {
			t.Fatalf("ListRecords(%+v): %v", f, err)
		}
		return len(rows)
	}

	checks := []struct {
		name string
		f    RecordFilter
		want int
	}{
		{"仅月份=3（跨年该月都算）", RecordFilter{Month: 3}, 2},
		{"仅年份=2025", RecordFilter{Year: 2025}, 2},
		{"年份+月份", RecordFilter{Year: 2025, Month: 3}, 1},
		{"月份 + 起始日期（此前串 start 会被丢掉）", RecordFilter{Month: 3, Start: "2025-01-01"}, 1},
		{"年份+月份 与 结束日期取交集（无交集）", RecordFilter{Year: 2025, Month: 3, End: "2025-03-01"}, 0},
		{"仅起始日期", RecordFilter{Start: "2025-01-01"}, 2},
		{"仅结束日期（含当日）", RecordFilter{End: "2025-03-02"}, 2},
	}
	for _, c := range checks {
		if got := count(c.f); got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
