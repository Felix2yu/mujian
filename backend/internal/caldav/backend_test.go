package caldav

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	emcaldav "github.com/emersion/go-webdav/caldav"

	"mujian/internal/db"
	"mujian/internal/models"
)

func newTestBackend(t *testing.T) *Backend {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "mujian.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	loc, _ := time.LoadLocation("Asia/Shanghai")
	database.SetLocation(loc)
	return New(database)
}

func testRecord(id, name string, at time.Time) models.Record {
	return models.Record{
		ID:       id,
		Name:     name,
		Address:  "南京保利大剧院",
		City:     "南京",
		Date:     at.Unix(),
		Duration: 120,
	}
}

func TestDiscoveryPaths(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	if p, _ := b.CurrentUserPrincipal(ctx); p != PrincipalPath {
		t.Errorf("principal = %q, want %q", p, PrincipalPath)
	}
	if p, _ := b.CalendarHomeSetPath(ctx); p != HomeSetPath {
		t.Errorf("home set = %q, want %q", p, HomeSetPath)
	}
	cals, err := b.ListCalendars(ctx)
	if err != nil || len(cals) != 1 {
		t.Fatalf("ListCalendars = %v, %v", cals, err)
	}
	if cals[0].Path != CalendarPath || cals[0].Name != calendarDisplayName {
		t.Errorf("calendar = %+v", cals[0])
	}
	if len(cals[0].SupportedComponentSet) != 1 || cals[0].SupportedComponentSet[0] != "VEVENT" {
		t.Errorf("supported components = %v, want [VEVENT]", cals[0].SupportedComponentSet)
	}
}

func TestListCalendarObjects(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 11, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-1", "桃花扇1699", at)); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}

	// Idempotent upsert must not change the ETag (content is identical).
	first, err := b.ListCalendarObjects(ctx, CalendarPath, nil)
	if err != nil {
		t.Fatalf("ListCalendarObjects: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("got %d objects, want 1", len(first))
	}
	co := first[0]
	if co.Path != CalendarPath+"/rec-1.ics" {
		t.Errorf("path = %q", co.Path)
	}
	if !strings.Contains(co.ETag, `"`) {
		t.Errorf("ETag should be quoted, got %q", co.ETag)
	}
	events := co.Data.Events()
	if len(events) != 1 {
		t.Fatalf("object has %d events, want 1", len(events))
	}
	summary, _ := events[0].Props.Text(ical.PropSummary)
	if summary != "桃花扇1699" {
		t.Errorf("SUMMARY = %q", summary)
	}
	uid, _ := events[0].Props.Text(ical.PropUID)
	if !strings.HasSuffix(uid, "@mujian") {
		t.Errorf("UID = %q, want @mujian suffix", uid)
	}

	second, _ := b.ListCalendarObjects(ctx, CalendarPath, nil)
	if second[0].ETag != co.ETag {
		t.Errorf("ETag must be deterministic: %q vs %q", second[0].ETag, co.ETag)
	}
}

func TestGetCalendarObjectRoundTrip(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 11, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-geo", "地理测试", at)); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	co, err := b.GetCalendarObject(ctx, CalendarPath+"/rec-geo.ics", nil)
	if err != nil {
		t.Fatalf("GetCalendarObject: %v", err)
	}
	if _, err := co.Data.Events()[0].Props.Text(ical.PropLocation); err != nil {
		t.Fatalf("LOCATION prop unreadable: %v", err)
	}
}

func TestGetCalendarObjectNotFound(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	for _, p := range []string{
		CalendarPath + "/missing.ics",
		CalendarPath + "/../etc/passwd.ics",
		CalendarPath + "/sub/dir.ics",
		"/other/calendars/mujian/rec-1.ics",
		CalendarPath,
	} {
		if _, err := b.GetCalendarObject(ctx, p, nil); err == nil {
			t.Errorf("GetCalendarObject(%q) should fail", p)
		}
	}
}

func TestQueryCalendarObjectsTimeRange(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	sep11 := time.Date(2026, 9, 11, 19, 30, 0, 0, time.UTC)
	oct01 := time.Date(2026, 10, 1, 19, 30, 0, 0, time.UTC)
	for _, rec := range []models.Record{
		testRecord("rec-sep", "九月场", sep11),
		testRecord("rec-oct", "十月场", oct01),
	} {
		if err := b.DB.UpsertRecord(rec); err != nil {
			t.Fatalf("UpsertRecord: %v", err)
		}
	}

	// Window covering only September: the October event must be excluded.
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	query := &emcaldav.CalendarQuery{
		CompFilter: emcaldav.CompFilter{
			Name: ical.CompCalendar,
			Comps: []emcaldav.CompFilter{{
				Name:  ical.CompEvent,
				Start: start,
				End:   end,
			}},
		},
	}
	objs, err := b.QueryCalendarObjects(ctx, CalendarPath, query)
	if err != nil {
		t.Fatalf("QueryCalendarObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("got %d objects in September window, want 1", len(objs))
	}
	evts := objs[0].Data.Events()
	summary, _ := evts[0].Props.Text(ical.PropSummary)
	if summary != "九月场" {
		t.Errorf("SUMMARY = %q, want 九月场", summary)
	}

	// No filter: both events.
	objs, err = b.QueryCalendarObjects(ctx, CalendarPath, nil)
	if err != nil || len(objs) != 2 {
		t.Fatalf("unfiltered query = %d objects, %v; want 2", len(objs), err)
	}
}

// Write operations must be rejected on the read-only backend.
func TestReadOnlyRejections(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()
	cal := ical.NewCalendar()
	if _, err := b.PutCalendarObject(ctx, CalendarPath+"/x.ics", cal, nil); err == nil {
		t.Error("PutCalendarObject should be rejected")
	}
	if err := b.DeleteCalendarObject(ctx, CalendarPath+"/x.ics"); err == nil {
		t.Error("DeleteCalendarObject should be rejected")
	}
	if err := b.CreateCalendar(ctx, &emcaldav.Calendar{Path: HomeSetPath + "other"}); err == nil {
		t.Error("CreateCalendar should be rejected")
	}
}
