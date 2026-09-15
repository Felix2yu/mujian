package db

import (
	"testing"

	"mujian/internal/models"
)

// Every write path relevant to the CalDAV projection must append to the sync
// journal — including deletes, which the client cannot infer from a missing
// member.
func TestCaldavChangelogTriggers(t *testing.T) {
	d := newTestDB(t)

	if seq, _ := d.CaldavCurrentSyncSeq(); seq != 0 {
		t.Fatalf("fresh db seq = %d, want 0", seq)
	}

	r := sampleRecord("rec-sync-1", 1789000000)
	if err := d.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	seq1, err := d.CaldavCurrentSyncSeq()
	if err != nil || seq1 < 1 {
		t.Fatalf("after insert seq = %d (%v)", seq1, err)
	}
	ids, err := d.CaldavChangedRecordIDs(0)
	if err != nil || len(ids) != 1 || ids[0] != "rec-sync-1" {
		t.Fatalf("changes after insert = %v, %v", ids, err)
	}

	// A second upsert (even identical) is another UPDATE event.
	if err := d.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord again: %v", err)
	}
	ids, _ = d.CaldavChangedRecordIDs(seq1)
	if len(ids) != 1 || ids[0] != "rec-sync-1" {
		t.Fatalf("changes after update = %v, want single rec-sync-1", ids)
	}

	// Soft delete (trash) must journal: the object has to leave the
	// collection explicitly on the next sync.
	seq2, _ := d.CaldavCurrentSyncSeq()
	if err := d.SoftDeleteRecord("rec-sync-1"); err != nil {
		t.Fatalf("SoftDeleteRecord: %v", err)
	}
	ids, _ = d.CaldavChangedRecordIDs(seq2)
	if len(ids) != 1 || ids[0] != "rec-sync-1" {
		t.Fatalf("changes after soft delete = %v", ids)
	}

	// Hard purge fires the DELETE trigger via records + relation cleanup.
	seq3, _ := d.CaldavCurrentSyncSeq()
	if err := d.PurgeRecord("rec-sync-1"); err != nil {
		t.Fatalf("PurgeRecord: %v", err)
	}
	ids, _ = d.CaldavChangedRecordIDs(seq3)
	found := false
	for _, id := range ids {
		if id == "rec-sync-1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("changes after purge = %v, must contain purged id", ids)
	}
}

// Relation edits (artist/zhezi/drama links) change the rendered ICS and must
// be journaled even though records itself is untouched.
func TestCaldavChangelogRelationTriggers(t *testing.T) {
	d := newTestDB(t)

	artist, err := d.SaveArtist(models.Artist{Name: "测试演员"})
	if err != nil {
		t.Fatalf("SaveArtist: %v", err)
	}
	if err := d.UpsertRecord(sampleRecord("rec-rel", 1789000001)); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	seq, _ := d.CaldavCurrentSyncSeq()

	if _, err := d.conn.Exec(
		"INSERT OR IGNORE INTO record_artists (record_id, artist_id, sort_order) VALUES (?, ?, 0)",
		"rec-rel", artist.ID,
	); err != nil {
		t.Fatalf("link artist: %v", err)
	}
	ids, _ := d.CaldavChangedRecordIDs(seq)
	if len(ids) != 1 || ids[0] != "rec-rel" {
		t.Fatalf("relation insert changes = %v, want rec-rel", ids)
	}

	seq, _ = d.CaldavCurrentSyncSeq()
	if _, err := d.conn.Exec(
		"DELETE FROM record_artists WHERE record_id = ? AND artist_id = ?",
		"rec-rel", artist.ID,
	); err != nil {
		t.Fatalf("unlink artist: %v", err)
	}
	ids, _ = d.CaldavChangedRecordIDs(seq)
	if len(ids) != 1 || ids[0] != "rec-rel" {
		t.Fatalf("relation delete changes = %v, want rec-rel", ids)
	}
}

// Multiple events for one record in one window collapse to a single id, and
// tokens outside the retained window are rejected.
func TestCaldavSyncTokenValidationAndPrune(t *testing.T) {
	d := newTestDB(t)

	// No journal: seq 0 is the valid initial token; positive tokens are not.
	if ok, err := d.CaldavSyncTokenValid(0); err != nil || !ok {
		t.Fatalf("token 0 validity = %v, %v", ok, err)
	}
	if ok, _ := d.CaldavSyncTokenValid(1); ok {
		t.Fatal("token 1 must be invalid on empty journal")
	}

	for i := 0; i < 5; i++ {
		id := "rec-bulk-" + string(rune('a'+i))
		if err := d.UpsertRecord(sampleRecord(id, 1789000000+int64(i))); err != nil {
			t.Fatalf("UpsertRecord %s: %v", id, err)
		}
		// Touch each record once more so dedup logic is exercised.
		if err := d.UpsertRecord(sampleRecord(id, 1789000000+int64(i))); err != nil {
			t.Fatalf("UpsertRecord %s again: %v", id, err)
		}
	}
	max, _ := d.CaldavCurrentSyncSeq()
	if max < 10 {
		t.Fatalf("seq after 10 record upserts = %d, want >= 10 (relation rewrites add journal rows)", max)
	}
	ids, _ := d.CaldavChangedRecordIDs(0)
	if len(ids) != 5 {
		t.Fatalf("distinct changed ids = %d, want 5: %v", len(ids), ids)
	}
	if ok, _ := d.CaldavSyncTokenValid(1); !ok {
		t.Fatal("token 1 should be valid within window")
	}
	if ok, _ := d.CaldavSyncTokenValid(max + 1); ok {
		t.Fatal("future token must be invalid")
	}

	// Retain only the last 4 journal entries: everything at/below max-4 is
	// pruned, so token 1 becomes invalid while a recent token survives.
	if n, err := d.PruneCaldavChangelog(4); err != nil || n != max-4 {
		t.Fatalf("PruneCaldavChangelog(4) = %d, %v; want %d removed", n, err, max-4)
	}
	if ok, _ := d.CaldavSyncTokenValid(1); ok {
		t.Fatal("pruned token must be invalid (client resyncs fully)")
	}
	if ok, _ := d.CaldavSyncTokenValid(max); !ok {
		t.Fatal("current token must remain valid after pruning older entries")
	}
}
