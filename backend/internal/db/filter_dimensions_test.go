package db

import (
	"testing"

	"mujian/internal/models"
)

// TestFilterDimensions exercises the new filter dimensions (channel, company,
// rating threshold, price range, status, exact-match) through the full
// ListRecords path against seeded data.
func TestFilterDimensions(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_760_000_000)

	seed := []models.RecordRequest{
		{Name: "京剧·A", Channel: "线下", Company: "A剧团", Rating: 8, Price: 120, ActiveStatus: 0, Date: base + 1},
		{Name: "昆曲·B", Channel: "线上", Company: "B剧团", Rating: 5, Price: 50, ActiveStatus: 1, Date: base + 2},
		{Name: "京剧·C", Channel: "线下", Company: "A剧团", Rating: 9, Price: 200, ActiveStatus: 2, Date: base + 3},
		{Name: "无渠道", Channel: "", Company: "", Rating: 0, Price: 0, ActiveStatus: 3, Date: base + 4},
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
		{"rating>=8", RecordFilter{RatingMin: 8}, 2},
		{"price>=100", RecordFilter{PriceMin: 100}, 2},
		{"price<=100", RecordFilter{PriceMax: 100}, 2},
		{"price 60~150", RecordFilter{PriceMin: 60, PriceMax: 150}, 1},
		{"status=1(想看)", RecordFilter{ActiveStatus: 1}, 1},
		{"exact 京剧·A", RecordFilter{Query: "京剧·A", Exact: true}, 1},
		{"exact 京剧 (no exact name)", RecordFilter{Query: "京剧", Exact: true}, 0},
		{"fuzzy 京剧", RecordFilter{Query: "京剧"}, 2},
		{"channel=线下 AND rating>=9", RecordFilter{Channel: "线下", RatingMin: 9}, 1},
	}
	for _, c := range checks {
		if got := count(c.f); got != c.want {
			t.Fatalf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}
