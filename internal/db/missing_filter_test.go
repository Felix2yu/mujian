package db

import (
	"testing"

	"mujian/internal/models"
)

// TestBuildMissingPredicate is a pure-function unit test: every supported token
// must map to its emptiness predicate, unknown tokens are dropped, and an
// empty/unknown-only input yields "" (no predicate appended).
// Multiple tokens combine per `mode`: "any" ORs them (data-hygiene sweep, the
// historical default), "all" ANDs them (what a row of checkboxes reads like).
func TestBuildMissingPredicate(t *testing.T) {
	cases := []struct {
		in   string
		mode string
		want string
	}{
		{"", "", ""},
		{"   ", "", ""},
		{"bogus", "", ""},
		{"category", "", "((COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')))"},
		{"cover", "", "(((cover IS NULL OR cover = '') AND (cover_file IS NULL OR cover_file = '')))"},
		{"artist", "", "((NOT EXISTS (SELECT 1 FROM record_artists ra WHERE ra.record_id = records.id)))"},
		{"category,rating", "any", "((COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')) OR (rating IS NULL OR rating = 0))"},
		{"category,bogus,cover", "", "((COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')) OR ((cover IS NULL OR cover = '') AND (cover_file IS NULL OR cover_file = '')))"},
		// "all"：全部所列字段都为空。
		{"category,city", "all", "((COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')) AND (city IS NULL OR city = ''))"},
		{"category", "all", "((COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')))"},
	}
	for _, c := range cases {
		if got := buildMissingPredicate(c.in, c.mode); got != c.want {
			t.Fatalf("buildMissingPredicate(%q, %q) = %q, want %q", c.in, c.mode, got, c.want)
		}
	}
}

// TestMissingFieldFilter exercises the full ListRecords path with the Missing
// filter against seeded data, and cross-checks each result count against an
// independent raw COUNT(*) using the same emptiness definitions.
func TestMissingFieldFilter(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_750_000_000) // fixed unix ts, ordering only

	withCat, err := d.CreateRecord(models.RecordRequest{Name: "缺失·有分类", City: "上海", Date: base, CategoryNames: []string{"京剧"}, CoverFile: "a.jpg"})
	if err != nil {
		t.Fatalf("seed withCat: %v", err)
	}
	noCatCity, err := d.CreateRecord(models.RecordRequest{Name: "缺失·无分类有城市", City: "北京", Date: base + 1})
	if err != nil {
		t.Fatalf("seed noCatCity: %v", err)
	}
	noCatNoCityCover, err := d.CreateRecord(models.RecordRequest{Name: "缺失·无分类无城市有封面", Date: base + 2, CoverFile: "c.jpg"})
	if err != nil {
		t.Fatalf("seed noCatNoCityCover: %v", err)
	}
	_ = withCat
	_ = noCatCity
	_ = noCatNoCityCover

	must := func(token string, mode string) []string {
		rows, err := d.ListRecords(RecordFilter{Missing: token, MissingMode: mode})
		if err != nil {
			t.Fatalf("ListRecords Missing=%q mode=%q: %v", token, mode, err)
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		return ids
	}
	rawCount := func(pred string) int {
		var n int
		if err := d.conn.QueryRow("SELECT COUNT(*) FROM records WHERE " + pred).Scan(&n); err != nil {
			t.Fatalf("raw count (%s): %v", pred, err)
		}
		return n
	}

	catPred := "COALESCE(json_array_length(category_names), 0) = 0 AND (category_name IS NULL OR category_name = '')"
	cityPred := "(city IS NULL OR city = '')"
	coverPred := "((cover IS NULL OR cover = '') AND (cover_file IS NULL OR cover_file = ''))"

	checks := []struct {
		token string
		want  int
		raw   int
	}{
		{"category", 2, rawCount(catPred)}, // noCatCity, noCatNoCityCover
		{"city", 1, rawCount(cityPred)},    // noCatNoCityCover
		{"category,city", 2, rawCount(catPred + " OR " + cityPred)},
		{"cover", 1, rawCount(coverPred)}, // noCatCity only (withCat & noCatNoCityCover have cover_file)
		{"artist", 3, rawCount("NOT EXISTS (SELECT 1 FROM record_artists ra WHERE ra.record_id = records.id)")},
	}
	for _, c := range checks {
		got := must(c.token, "")
		if len(got) != c.want {
			t.Fatalf("Missing=%q: got %d rows, want %d", c.token, len(got), c.want)
		}
		if c.want != c.raw {
			t.Fatalf("Missing=%q: ListRecords returned %d but independent raw count is %d (predicate drift)", c.token, c.want, c.raw)
		}
	}

	// mode=all 是交集：既无分类又无城市的只有 noCatNoCityCover 一条，
	// 而不是 any 模式下 noCatCity + noCatNoCityCover 两条。
	if got := must("category,city", "all"); len(got) != 1 {
		t.Fatalf("Missing=category,city mode=all: got %d rows, want 1 (%v)", len(got), got)
	}
	if got := must("category,city", "any"); len(got) != 2 {
		t.Fatalf("Missing=category,city mode=any: got %d rows, want 2 (%v)", len(got), got)
	}
}
