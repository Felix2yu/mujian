package db

import (
	"testing"
)

func TestPruneCaldavChangelogDefault(t *testing.T) {
	db := newTestDB(t)

	// Insert some changelog entries
	for i := 0; i < 5; i++ {
		_, err := db.conn.Exec("INSERT INTO caldav_changelog (record_id) VALUES (?)", "rec")
		if err != nil {
			t.Fatal(err)
		}
	}

	n, err := db.PruneCaldavChangelogDefault()
	if err != nil {
		t.Fatal(err)
	}
	// 5 entries, keep 20000 → nothing pruned
	if n != 0 {
		t.Errorf("PruneCaldavChangelogDefault = %d, want 0 (fewer than keep)", n)
	}

	// With a small keep value, entries should be pruned
	n2, err := db.PruneCaldavChangelog(2)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 3 {
		t.Errorf("PruneCaldavChangelog(2) = %d, want 3", n2)
	}
}

func TestPruneCaldavChangelogEdgeCases(t *testing.T) {
	db := newTestDB(t)

	// Empty journal → nothing to prune
	n, err := db.PruneCaldavChangelog(100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("prune empty journal = %d, want 0", n)
	}

	// Keep 0 → should use default constant
	n2, err := db.PruneCaldavChangelog(0)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("prune with keep=0 = %d, want 0", n2)
	}
}
