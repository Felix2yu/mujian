package db

import (
	"mujian/internal/models"
	"testing"
	"time"
)

func TestMergeAliasDelta(t *testing.T) {
	tests := []struct {
		targetName string
		targetAl   []string
		sourceName string
		sourceAl   []string
		wantLen    int
	}{
		{"A", nil, "B", nil, 1},                          // source name only
		{"A", []string{"B"}, "B", nil, 0},                // source name already in target aliases
		{"A", nil, "B", []string{"C", "D"}, 3},           // source name + aliases all new
		{"A", []string{"C"}, "B", []string{"C", "D"}, 2}, // C already in target
		{"A", nil, "A", nil, 0},                          // same name
	}
	for _, tc := range tests {
		got := mergeAliasDelta(tc.targetName, tc.targetAl, tc.sourceName, tc.sourceAl)
		if len(got) != tc.wantLen {
			t.Errorf("mergeAliasDelta(%q, %v, %q, %v) = %v (len=%d, want %d)",
				tc.targetName, tc.targetAl, tc.sourceName, tc.sourceAl, got, len(got), tc.wantLen)
		}
	}
}

func TestPreviewMergeArtists(t *testing.T) {
	db := newTestDB(t)
	s, _ := db.SaveArtist(models.Artist{Name: "源演员"})
	tgt, _ := db.SaveArtist(models.Artist{Name: "目标演员"})
	base := time.Now().Unix()

	_ = db.UpsertRecord(models.Record{ID: "pma1", Name: "A", ArtistIDs: []string{s.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "pma2", Name: "B", ArtistIDs: []string{tgt.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "pma3", Name: "C", ArtistIDs: []string{s.ID, tgt.ID}, Date: base})

	preview, err := db.PreviewMergeArtists(s.ID, tgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "artist" {
		t.Errorf("kind = %q, want artist", preview.Kind)
	}
	if preview.Source.ID != s.ID || preview.Target.ID != tgt.ID {
		t.Errorf("source/target mismatch: %+v", preview)
	}
	if preview.RecordsRepoint != 1 {
		t.Errorf("RecordsRepoint = %d, want 1", preview.RecordsRepoint)
	}
	if preview.RecordsDedupe != 1 {
		t.Errorf("RecordsDedupe = %d, want 1", preview.RecordsDedupe)
	}
	if len(preview.AliasesAdded) != 1 || preview.AliasesAdded[0] != "源演员" {
		t.Errorf("AliasesAdded = %v, want [源演员]", preview.AliasesAdded)
	}

	// Error cases
	if _, err := db.PreviewMergeArtists("", tgt.ID); err == nil {
		t.Error("empty source should error")
	}
	if _, err := db.PreviewMergeArtists(s.ID, s.ID); err == nil {
		t.Error("same source/target should error")
	}
	if _, err := db.PreviewMergeArtists("nonexistent", tgt.ID); err == nil {
		t.Error("missing source should error")
	}
}

func TestPreviewMergeDramas(t *testing.T) {
	db := newTestDB(t)
	s, _ := db.SaveDrama(models.Drama{Name: "源剧目"})
	tgt, _ := db.SaveDrama(models.Drama{Name: "目标剧目"})
	// Add zhezis
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: s.ID, Name: "折子A"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: tgt.ID, Name: "折子A"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: s.ID, Name: "折子B"})
	base := time.Now().Unix()

	_ = db.UpsertRecord(models.Record{ID: "pmd1", Name: "A", DramaIDs: []string{s.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "pmd2", Name: "B", DramaIDs: []string{tgt.ID}, Date: base})

	preview, err := db.PreviewMergeDramas(s.ID, tgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "drama" {
		t.Errorf("kind = %q, want drama", preview.Kind)
	}
	if preview.ZhezisMoved != 1 || preview.ZhezisDeduped != 1 {
		t.Errorf("zhezis: moved=%d deduped=%d", preview.ZhezisMoved, preview.ZhezisDeduped)
	}
	if preview.RecordsRepoint != 1 {
		t.Errorf("RecordsRepoint = %d, want 1", preview.RecordsRepoint)
	}

	// Error cases
	if _, err := db.PreviewMergeDramas("", tgt.ID); err == nil {
		t.Error("empty source should error")
	}
	if _, err := db.PreviewMergeDramas(s.ID, s.ID); err == nil {
		t.Error("same source/target should error")
	}
}

func TestMergeArtists(t *testing.T) {
	db := newTestDB(t)
	s, _ := db.SaveArtist(models.Artist{Name: "源演员", Aliases: []string{"旧名"}})
	tgt, _ := db.SaveArtist(models.Artist{Name: "目标演员"})
	base := time.Now().Unix()

	_ = db.UpsertRecord(models.Record{ID: "ma1", Name: "A", ArtistIDs: []string{s.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "ma2", Name: "B", ArtistIDs: []string{tgt.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "ma3", Name: "C", ArtistIDs: []string{s.ID, tgt.ID}, Date: base})

	res, err := db.MergeArtists(s.ID, tgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.RecordsRepointed != 1 {
		t.Errorf("RecordsRepointed = %d, want 1", res.RecordsRepointed)
	}
	if res.RecordsDeduped != 1 {
		t.Errorf("RecordsDeduped = %d, want 1", res.RecordsDeduped)
	}
	if len(res.AliasesAdded) == 0 {
		t.Error("should have added aliases")
	}

	// Source should be deleted
	if _, err := db.GetArtist(s.ID); err == nil {
		t.Error("source artist should be deleted after merge")
	}

	// Error cases
	if _, err := db.MergeArtists("", tgt.ID); err == nil {
		t.Error("empty source should error")
	}
	if _, err := db.MergeArtists(s.ID, s.ID); err == nil {
		t.Error("same source/target should error")
	}
	if _, err := db.MergeArtists("nonexistent", tgt.ID); err == nil {
		t.Error("missing source should error")
	}
}

func TestMergeDramas(t *testing.T) {
	db := newTestDB(t)
	s, _ := db.SaveDrama(models.Drama{Name: "源剧目", Remark: "源备注"})
	tgt, _ := db.SaveDrama(models.Drama{Name: "目标剧目"})
	// Zhezis
	zUnique, _ := db.CreateZhezi(models.Zhezi{DramaID: s.ID, Name: "独有折子"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: s.ID, Name: "共有折子"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: tgt.ID, Name: "共有折子"})
	base := time.Now().Unix()

	_ = db.UpsertRecord(models.Record{ID: "md1", Name: "A", DramaIDs: []string{s.ID}, Date: base})
	_ = db.UpsertRecord(models.Record{ID: "md2", Name: "B", DramaIDs: []string{tgt.ID}, Date: base})
	// Link record to the unique zhezi
	_ = db.UpsertRecord(models.Record{ID: "md3", Name: "C", DramaIDs: []string{s.ID}, ZheziIDs: []string{zUnique.ID}, Date: base})

	res, err := db.MergeDramas(s.ID, tgt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.ZhezisMoved != 1 {
		t.Errorf("ZhezisMoved = %d, want 1", res.ZhezisMoved)
	}
	if res.ZhezisDeduped != 1 {
		t.Errorf("ZhezisDeduped = %d, want 1", res.ZhezisDeduped)
	}
	if res.RecordsRepointed != 2 {
		t.Errorf("RecordsRepointed = %d, want 2", res.RecordsRepointed)
	}
	if !res.RemarkTakenOver {
		t.Error("RemarkTakenOver should be true (target has no remark)")
	}

	// Source should be deleted
	if _, err := db.GetDrama(s.ID); err == nil {
		t.Error("source drama should be deleted after merge")
	}

	// Error cases
	if _, err := db.MergeDramas("", tgt.ID); err == nil {
		t.Error("empty source should error")
	}
	if _, err := db.MergeDramas(s.ID, s.ID); err == nil {
		t.Error("same source/target should error")
	}
	if _, err := db.MergeDramas("nonexistent", tgt.ID); err == nil {
		t.Error("missing source should error")
	}
}

func TestListZhezisByDramaTx(t *testing.T) {
	db := newTestDB(t)
	d, _ := db.SaveDrama(models.Drama{Name: "测试剧"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "折子1"})
	_, _ = db.CreateZhezi(models.Zhezi{DramaID: d.ID, Name: "折子2"})

	tx, err := db.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	zs, err := db.listZhezisByDramaTx(tx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(zs) != 2 {
		t.Errorf("listZhezisByDramaTx = %d zhezis, want 2", len(zs))
	}
}
