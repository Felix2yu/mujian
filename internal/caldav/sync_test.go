package caldav

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mujian/internal/models"
)

func TestSyncTokenParseFormat(t *testing.T) {
	tok := formatSyncToken(42)
	if tok != "mj1-42" {
		t.Fatalf("token = %q", tok)
	}
	seq, initial, err := parseSyncToken("mj1-42")
	if err != nil || initial || seq != 42 {
		t.Fatalf("parse mj1-42 = %d, %v, %v", seq, initial, err)
	}
	if _, initial, err := parseSyncToken(""); err != nil || !initial {
		t.Fatalf("empty token must mean initial sync, got initial=%v err=%v", initial, err)
	}
	for _, bad := range []string{"mj1-", "other-1", "mj1-x", "mj2-42"} {
		if _, _, err := parseSyncToken(bad); !errors.Is(err, ErrInvalidSyncToken) {
			t.Errorf("parse %q = %v, want ErrInvalidSyncToken", bad, err)
		}
	}
}

// Full sync → incremental add → no-op sync, across both collections.
func TestSyncCollectionInitialAndIncremental(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 2, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-a", "首演", at)); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}

	// Initial sync (no token): every current member is returned as an add.
	res, err := b.SyncCollection(ctx, CalendarPath, "")
	if err != nil {
		t.Fatalf("initial event sync: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Object == nil {
		t.Fatalf("initial entries = %+v, want 1 add", res.Entries)
	}
	if !strings.HasPrefix(res.NewToken, syncTokenPrefix) {
		t.Fatalf("new token = %q", res.NewToken)
	}
	token := res.NewToken

	// Task collection is independent: its initial sync also contains the
	// normal-status record.
	tasks, err := b.SyncCollection(ctx, TasksPath, "")
	if err != nil {
		t.Fatalf("initial task sync: %v", err)
	}
	if len(tasks.Entries) != 1 {
		t.Fatalf("initial task entries = %d, want 1", len(tasks.Entries))
	}

	// Another record: only the new member comes back, not a full dump.
	if err := b.DB.UpsertRecord(testRecord("rec-b", "次演", at.Add(24*time.Hour))); err != nil {
		t.Fatalf("UpsertRecord rec-b: %v", err)
	}
	res, err = b.SyncCollection(ctx, CalendarPath, token)
	if err != nil {
		t.Fatalf("incremental sync: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Object == nil {
		t.Fatalf("delta entries = %+v, want 1 add", res.Entries)
	}
	if res.Entries[0].Object.Path != CalendarPath+"rec-b.ics" {
		t.Errorf("delta path = %q", res.Entries[0].Object.Path)
	}
	token = res.NewToken

	// No changes since the token: empty delta, same token.
	res, err = b.SyncCollection(ctx, CalendarPath, token)
	if err != nil {
		t.Fatalf("empty delta sync: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("delta entries = %+v, want none", res.Entries)
	}
	if res.NewToken != token {
		t.Errorf("token changed without changes: %q vs %q", res.NewToken, token)
	}

	// A foreign/garbage collection path is rejected.
	if _, err := b.SyncCollection(ctx, "/caldav/user/calendars/other/", ""); err == nil {
		t.Error("sync on unknown collection should fail")
	}
}

// A status flip moves a record between collections: it must be an explicit
// removal from the collection it left (not inferred) and an update in the
// one it joined.
func TestSyncCollectionStatusMovement(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 3, 19, 30, 0, 0, time.UTC)
	rec := testRecord("rec-move", "迁移", at)
	if err := b.DB.UpsertRecord(rec); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	evBase, err := b.SyncCollection(ctx, CalendarPath, "")
	if err != nil {
		t.Fatal(err)
	}
	taskBase, err := b.SyncCollection(ctx, TasksPath, "")
	if err != nil {
		t.Fatal(err)
	}

	// Normal(0) → 想看(1): stays in the VEVENT calendar, leaves VTODO.
	rec.ActiveStatus = models.StatusWantWatch
	if err := b.DB.UpsertRecord(rec); err != nil {
		t.Fatalf("flip wantwatch: %v", err)
	}
	ev, err := b.SyncCollection(ctx, CalendarPath, evBase.NewToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Entries) != 1 || ev.Entries[0].Object == nil ||
		ev.Entries[0].Object.Path != CalendarPath+"rec-move.ics" {
		t.Fatalf("events delta on move = %+v", ev.Entries)
	}
	tk, err := b.SyncCollection(ctx, TasksPath, taskBase.NewToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.Entries) != 1 || tk.Entries[0].RemovedHref != TasksPath+"rec-move.ics" {
		t.Fatalf("tasks delta on move = %+v, want explicit removal", tk.Entries)
	}

	// 想看(1) → 已取消(2): leaves the VEVENT calendar as well.
	evToken := ev.NewToken
	rec.ActiveStatus = models.StatusCancelled
	if err := b.DB.UpsertRecord(rec); err != nil {
		t.Fatalf("flip cancelled: %v", err)
	}
	ev, err = b.SyncCollection(ctx, CalendarPath, evToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Entries) != 1 || ev.Entries[0].RemovedHref != CalendarPath+"rec-move.ics" {
		t.Fatalf("events delta on cancel = %+v, want removal", ev.Entries)
	}
}

// Soft delete and hard purge are both explicit removals; the ETag changes on
// update even when several events collapse into one delta entry.
func TestSyncCollectionDeletion(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 4, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-del", "删除测试", at)); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	base, err := b.SyncCollection(ctx, CalendarPath, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := b.DB.SoftDeleteRecord("rec-del"); err != nil {
		t.Fatalf("SoftDeleteRecord: %v", err)
	}
	res, err := b.SyncCollection(ctx, CalendarPath, base.NewToken)
	if err != nil {
		t.Fatalf("sync after soft delete: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].RemovedHref != CalendarPath+"rec-del.ics" {
		t.Fatalf("soft delete delta = %+v, want 1 explicit removal", res.Entries)
	}

	// Restore + purge cycle: restore is an update (member rejoins), hard
	// purge is another removal.
	token := res.NewToken
	if err := b.DB.RestoreRecord("rec-del"); err != nil {
		t.Fatalf("RestoreRecord: %v", err)
	}
	res, err = b.SyncCollection(ctx, CalendarPath, token)
	if err != nil {
		t.Fatalf("sync after restore: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Object == nil {
		t.Fatalf("restore delta = %+v, want re-add", res.Entries)
	}

	if err := b.DB.SoftDeleteRecord("rec-del"); err != nil {
		t.Fatal(err)
	}
	if err := b.DB.PurgeRecord("rec-del"); err != nil {
		t.Fatalf("PurgeRecord: %v", err)
	}
	res, err = b.SyncCollection(ctx, CalendarPath, res.NewToken)
	if err != nil {
		t.Fatalf("sync after purge: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].RemovedHref != CalendarPath+"rec-del.ics" {
		t.Fatalf("purge delta = %+v, want removal", res.Entries)
	}
}

// A token outside the retained journal window (or malformed) forces a full
// resync via ErrInvalidSyncToken instead of silently returning wrong data.
func TestSyncCollectionInvalidToken(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-x", "X", at)); err != nil {
		t.Fatal(err)
	}

	if _, err := b.SyncCollection(ctx, CalendarPath, "garbage"); !errors.Is(err, ErrInvalidSyncToken) {
		t.Errorf("garbage token = %v, want ErrInvalidSyncToken", err)
	}

	// Simulate journal pruning: touch the record once more, keep only the
	// newest entry, then present the older seq token.
	if err := b.DB.UpsertRecord(testRecord("rec-x", "X改", at.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DB.PruneCaldavChangelog(1); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := b.SyncCollection(ctx, CalendarPath, "mj1-1"); !errors.Is(err, ErrInvalidSyncToken) {
		t.Errorf("pruned token = %v, want ErrInvalidSyncToken", err)
	}
	// Empty token still performs a valid initial sync against a pruned journal.
	res, err := b.SyncCollection(ctx, CalendarPath, "")
	if err != nil {
		t.Fatalf("initial sync after prune: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("initial entries after prune = %d, want 1", len(res.Entries))
	}
}
