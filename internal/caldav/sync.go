package caldav

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	stdpath "path"
	"strconv"
	"strings"

	emcaldav "github.com/emersion/go-webdav/caldav"

	"mujian/internal/ics"
)

// RFC 6578 collection synchronization (DAV Sync) for the read-only calendar.
//
// The server is the sole writer and never deletes a collection, so the sync
// model is the simplest authoritative one: a sync token is the changelog seq
// the client already holds; every record with a journal event after that seq
// is resolved against the CURRENT collection membership and reported either
// as a changed member (full props) or as an explicit 404 removal. Clients
// never have to guess deletions from missing members, and status flips that
// move a record between the VEVENT and VTODO collections are reported as a
// removal from one and an update in the other.
//
// On-disk change tracking lives in internal/db/caldav_sync.go (triggers +
// the caldav_changelog journal); this file only computes protocol deltas.

// ErrInvalidSyncToken means the client's token is older than the retained
// journal window (or foreign to this database). The HTTP layer maps it to
// RFC 6578 DAV:valid-sync-token with status 403, after which the client
// re-runs the report without a token for a full resync.
var ErrInvalidSyncToken = errors.New("caldav: invalid sync token")

// syncTokenPrefix versions the opaque token format so a future journal
// redesign can invalidate old tokens instead of misreading them.
const syncTokenPrefix = "mj1-"

// SyncEntry is one member change in a sync-collection response. Exactly one
// of Object (member present: added or updated) or RemovedHref (member left
// the collection, soft/hard deleted, or excluded by status) is set.
type SyncEntry struct {
	Object      *emcaldav.CalendarObject
	RemovedHref string
}

// SyncResult is the resolved delta for one collection.
type SyncResult struct {
	NewToken string
	Entries  []SyncEntry
}

// formatSyncToken renders a journal seq as an opaque client token.
func formatSyncToken(seq int64) string {
	return syncTokenPrefix + strconv.FormatInt(seq, 10)
}

// parseSyncToken parses a token. Empty input means "initial sync" and yields
// seq 0; anything unparseable is invalid.
func parseSyncToken(token string) (seq int64, initial bool, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, true, nil
	}
	if !strings.HasPrefix(token, syncTokenPrefix) {
		return 0, false, ErrInvalidSyncToken
	}
	seq, err = strconv.ParseInt(strings.TrimPrefix(token, syncTokenPrefix), 10, 64)
	if err != nil {
		return 0, false, ErrInvalidSyncToken
	}
	return seq, false, nil
}

// CurrentSyncToken returns the token describing the database state now. It
// is embedded in PROPFIND responses for the sync-token property.
func (b *Backend) CurrentSyncToken(ctx context.Context) (string, error) {
	seq, err := b.DB.CaldavCurrentSyncSeq()
	if err != nil {
		return "", err
	}
	return formatSyncToken(seq), nil
}

// SyncCollection computes the RFC 6578 delta for one collection. With no
// token (initial synchronization) every current member is returned; with a
// valid token only records touched after that seq appear, as updates or
// explicit removals.
func (b *Backend) SyncCollection(ctx context.Context, collectionPath, token string) (*SyncResult, error) {
	isTask, allowed, err := b.syncCollectionKind(collectionPath)
	if err != nil {
		return nil, err
	}

	seq, initial, err := parseSyncToken(token)
	if err != nil {
		return nil, err
	}
	if !initial {
		valid, err := b.DB.CaldavSyncTokenValid(seq)
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrInvalidSyncToken
		}
	}

	var entries []SyncEntry
	if initial {
		// RFC 6578 §3.4: initial sync reports all current members as adds.
		recs, zheziNames, err := b.loadRecords(ctx, allowed)
		if err != nil {
			return nil, err
		}
		objs, err := b.toCalendarObjects(recs, zheziNames, isTask, b.reminderConfig())
		if err != nil {
			return nil, err
		}
		entries = make([]SyncEntry, 0, len(objs))
		for i := range objs {
			entries = append(entries, SyncEntry{Object: &objs[i]})
		}
	} else {
		ids, err := b.DB.CaldavChangedRecordIDs(seq)
		if err != nil {
			return nil, err
		}
		entries = make([]SyncEntry, 0, len(ids))
		hrefBase := CalendarPath
		if isTask {
			hrefBase = TasksPath
		}
		for _, id := range ids {
			entry, err := b.resolveSyncChange(ctx, id, hrefBase, allowed, isTask)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
	}

	newSeq, err := b.DB.CaldavCurrentSyncSeq()
	if err != nil {
		return nil, err
	}
	return &SyncResult{NewToken: formatSyncToken(newSeq), Entries: entries}, nil
}

// syncCollectionKind maps a request path to its component kind and the
// ActiveStatus set published there.
func (b *Backend) syncCollectionKind(p string) (isTask bool, allowed []int, err error) {
	switch stdpath.Clean(p) {
	case stdpath.Clean(CalendarPath):
		return false, ics.StatusCalendar, nil
	case stdpath.Clean(TasksPath):
		return true, ics.StatusTasks, nil
	default:
		return false, nil, fmt.Errorf("caldav: %q is not a syncable collection", p)
	}
}

// resolveSyncChange maps one changed record id to its entry in the target
// collection. A record that no longer exists (hard purge, or soft-deleted —
// GetRecord filters deleted_at), or whose status excludes it from this
// collection, is an explicit removal; everything else is re-rendered with a
// fresh ETag. Multiple journal events for the same id within one sync
// collapse to this single current-state entry.
func (b *Backend) resolveSyncChange(ctx context.Context, id, hrefBase string, allowed []int, isTask bool) (SyncEntry, error) {
	removed := SyncEntry{RemovedHref: hrefBase + id + ".ics"}

	rec, err := b.DB.GetRecord(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return removed, nil
		}
		return SyncEntry{}, err
	}
	if !ics.StatusAllowed(rec.ActiveStatus, allowed) {
		return removed, nil
	}

	names, err := b.DB.GetZheziNames(rec.ZheziIDs)
	if err != nil {
		names = nil
	}
	co, err := b.toCalendarObject(*rec, names, isTask, b.reminderConfig())
	if err != nil {
		return SyncEntry{}, err
	}
	return SyncEntry{Object: &co}, nil
}
