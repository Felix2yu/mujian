// Package caldav exposes the mujian database as a read-only CalDAV calendar
// so iOS/macOS Calendar can sync events as first-class accounts. Events synced
// this way (unlike the /api/calendar.ics subscription) are geocoded on-device:
// LOCATION renders as a tappable map card.
//
// The protocol plumbing is provided by github.com/emersion/go-webdav; this
// file only adapts the database to its caldav.Backend interface. All write
// operations return 403 — the calendar is a read-only projection of records.
package caldav

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	stdpath "path"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	emcaldav "github.com/emersion/go-webdav/caldav"
	"github.com/emersion/go-webdav"

	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/ics"
	"mujian/internal/models"
)

// Path layout (must stay consistent with Handler.Prefix = "/caldav" in main.go;
// go-webdav classifies resources by counting path segments below the prefix).
//
// Collection paths end with "/" per CalDAV convention. Apple Calendar normalizes
// collection URLs with a trailing slash when refreshing, so hrefs we advertise
// must match what it will request — and all incoming paths are compared via
// path.Clean so both slashed and unslashed forms resolve.
const (
	// PrincipalPath identifies the CalDAV user principal.
	PrincipalPath = "/caldav/user/"
	// HomeSetPath is the calendar-home-set container.
	HomeSetPath = "/caldav/user/calendars/"
	// CalendarPath is the exposed event calendar collection (VEVENT).
	CalendarPath = "/caldav/user/calendars/mujian/"
	// TasksPath is the exposed task collection (VTODO). Ticking a task in a
	// CalDAV task client (e.g. Apple Reminders) round-trips back as
	// STATUS:COMPLETED and is stored on the record as 已观看/已到场.
	TasksPath = "/caldav/user/calendars/mujian-tasks/"
	// calendarDisplayName is shown as the calendar name in Calendar.app.
	calendarDisplayName = "幕间"
	// tasksDisplayName is shown as the task-list name in Reminders.app.
	tasksDisplayName = "幕间·提醒"
)

// Backend implements emcaldav.Backend on top of the records database.
type Backend struct {
	DB  *db.DB
	Cfg *config.Config
}

// New builds a read-only CalDAV backend over the given database.
func New(database *db.DB, cfg *config.Config) *Backend {
	return &Backend{DB: database, Cfg: cfg}
}

// reminderConfig 从服务端设置构建 ics.ReminderConfig；Cfg 为 nil 时回退到
// 默认策略（演出开始前 12 小时），保证测试等无配置场景仍可渲染。
func (b *Backend) reminderConfig() *ics.ReminderConfig {
	if b.Cfg == nil {
		return ics.DefaultReminderConfig()
	}
	rc := b.Cfg.GetReminderConfig()
	return &ics.ReminderConfig{
		Mode:        rc.Mode,
		BeforeHours: rc.BeforeHours,
		DailyHour:   rc.DailyHour,
		DailyMinute: rc.DailyMinute,
	}
}

// CurrentUserPrincipal implements webdav.UserPrincipalBackend.
func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return PrincipalPath, nil
}

// CalendarHomeSetPath implements emcaldav.Backend.
func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	return HomeSetPath, nil
}

// CreateCalendar is rejected: the calendar collection is fixed.
func (b *Backend) CreateCalendar(ctx context.Context, calendar *emcaldav.Calendar) error {
	return webdav.NewHTTPError(403, errors.New("caldav: read-only backend"))
}

// ListCalendars implements emcaldav.Backend with the mujian event calendar
// and the mujian-tasks task collection (both live under the same home-set, so
// Calendar.app picks up the VEVENT one and Reminders.app the VTODO one).
func (b *Backend) ListCalendars(ctx context.Context) ([]emcaldav.Calendar, error) {
	return []emcaldav.Calendar{b.calendar(), b.tasksCalendar()}, nil
}

// GetCalendar implements emcaldav.Backend. Path comparison is Clean-normalized
// so Apple's trailing-slash collection requests match the canonical href.
func (b *Backend) GetCalendar(ctx context.Context, p string) (*emcaldav.Calendar, error) {
	switch stdpath.Clean(p) {
	case stdpath.Clean(CalendarPath):
		cal := b.calendar()
		return &cal, nil
	case stdpath.Clean(TasksPath):
		cal := b.tasksCalendar()
		return &cal, nil
	default:
		return nil, webdav.NewHTTPError(404, fmt.Errorf("calendar %q not found", p))
	}
}

func (b *Backend) calendar() emcaldav.Calendar {
	return emcaldav.Calendar{
		Path:                  CalendarPath,
		Name:                  calendarDisplayName,
		Description:           "幕间演出记录（只读）",
		SupportedComponentSet: []string{ical.CompEvent},
	}
}

// tasksCalendar is the VTODO collection. It is read-only for listing but a
// task client may tick items (STATUS:COMPLETED), which the backend persists
// as the record's 已观看/已到场 flag (see PutCalendarObject).
func (b *Backend) tasksCalendar() emcaldav.Calendar {
	return emcaldav.Calendar{
		Path:                  TasksPath,
		Name:                  tasksDisplayName,
		Description:           "幕间演出提醒（可勾选完成）",
		SupportedComponentSet: []string{ical.CompToDo},
	}
}

// GetCalendarObject implements emcaldav.Backend. Object paths are
// "<CalendarPath|TasksPath><id>.ics". A GET/PROPFIND on a collection itself
// (browsers do this; Apple never does) falls back to serving the whole
// calendar as one object so the URL is inspectable instead of a bare 404.
func (b *Backend) GetCalendarObject(ctx context.Context, p string, req *emcaldav.CalendarCompRequest) (*emcaldav.CalendarObject, error) {
	clean := stdpath.Clean(p)
	switch clean {
	case stdpath.Clean(CalendarPath):
		recs, zheziNames, err := b.loadRecords(ctx)
		if err != nil {
			return nil, err
		}
		text := ics.GenerateCalendar(recs, b.DB.Location(), zheziNames)
		co, err := calendarObjectFromICS(CalendarPath, text, time.Now())
		if err != nil {
			return nil, err
		}
		return &co, nil
	case stdpath.Clean(TasksPath):
		recs, zheziNames, err := b.loadRecords(ctx)
		if err != nil {
			return nil, err
		}
		text := ics.GenerateTodos(recs, b.DB.Location(), zheziNames, b.reminderConfig())
		co, err := calendarObjectFromICS(TasksPath, text, time.Now())
		if err != nil {
			return nil, err
		}
		return &co, nil
	}
	id, isTask, ok := objectID(p)
	if !ok {
		return nil, webdav.NewHTTPError(404, fmt.Errorf("calendar object %q not found", p))
	}
	rec, err := b.DB.GetRecord(id)
	if err != nil {
		return nil, webdav.NewHTTPError(503, fmt.Errorf("caldav: database temporarily unavailable: %w", err))
	}
	names, err := b.DB.GetZheziNames(rec.ZheziIDs)
	if err != nil {
		names = nil
	}
	co, err := b.toCalendarObject(*rec, names, isTask, b.reminderConfig())
	if err != nil {
		return nil, err
	}
	return &co, nil
}

// ListCalendarObjects implements emcaldav.Backend (full listing). The served
// component kind (VEVENT vs VTODO) follows the requested collection.
func (b *Backend) ListCalendarObjects(ctx context.Context, path string, req *emcaldav.CalendarCompRequest) ([]emcaldav.CalendarObject, error) {
	recs, zheziNames, err := b.loadRecords(ctx)
	if err != nil {
		return nil, err
	}
	return b.toCalendarObjects(recs, zheziNames, stdpath.Clean(path) == stdpath.Clean(TasksPath), b.reminderConfig())
}

// QueryCalendarObjects implements emcaldav.Backend. Time-range filters (the
// VEVENT/VTODO comp of the client's comp-filter) are honored; prop-filters are
// ignored — over-returning objects is protocol-legal, the client drops them.
func (b *Backend) QueryCalendarObjects(ctx context.Context, path string, query *emcaldav.CalendarQuery) ([]emcaldav.CalendarObject, error) {
	recs, zheziNames, err := b.loadRecords(ctx)
	if err != nil {
		return nil, err
	}
	if query != nil {
		if start, end, ok := eventTimeRange(query.CompFilter); ok {
			recs = filterByTimeRange(recs, start, end)
		}
	}
	return b.toCalendarObjects(recs, zheziNames, stdpath.Clean(path) == stdpath.Clean(TasksPath), b.reminderConfig())
}

// PutCalendarObject accepts completion writes for VTODO (task) objects only.
// The backend is still read-only for VEVENT objects and for any unknown UID —
// ticking a reminder in a task client round-trips here as STATUS:COMPLETED,
// which we persist as the record's 已观看/已到场 flag. We then re-render the
// canonical VTODO from the database (ignoring any other fields the client may
// have sent) so the served object stays authoritative and its ETag updates.
func (b *Backend) PutCalendarObject(ctx context.Context, path string, calendar *ical.Calendar, opts *emcaldav.PutCalendarObjectOptions) (*emcaldav.CalendarObject, error) {
	id, isTask, ok := objectID(path)
	if !ok || !isTask {
		return nil, webdav.NewHTTPError(403, errors.New("caldav: only task completion is writable"))
	}
	watched := vtodoStatusCompleted(calendar)
	if err := b.DB.SetRecordWatched(id, watched); err != nil {
		return nil, webdav.NewHTTPError(503, fmt.Errorf("caldav: database temporarily unavailable: %w", err))
	}
	// Re-render the canonical object from the database so the client sees the
	// authoritative state (and a fresh ETag) rather than what it just sent.
	rec, err := b.DB.GetRecord(id)
	if err != nil {
		return nil, webdav.NewHTTPError(503, fmt.Errorf("caldav: database temporarily unavailable: %w", err))
	}
	names, err := b.DB.GetZheziNames(rec.ZheziIDs)
	if err != nil {
		names = nil
	}
	co, err := b.toCalendarObject(*rec, names, true, b.reminderConfig())
	if err != nil {
		return nil, err
	}
	return &co, nil
}

// DeleteCalendarObject is rejected: records are edited through the mujian UI/API.
func (b *Backend) DeleteCalendarObject(ctx context.Context, path string) error {
	return webdav.NewHTTPError(403, errors.New("caldav: read-only backend"))
}

// eventTimeRange walks a comp-filter tree looking for a VEVENT time-range
// (RFC 4791 §9.9). Falls back to the top-level VCALENDAR range. The boolean
// result reports whether any range was found.
func eventTimeRange(cf emcaldav.CompFilter) (start, end time.Time, found bool) {
	if cf.Name == ical.CompEvent && (!cf.Start.IsZero() || !cf.End.IsZero()) {
		return cf.Start, cf.End, true
	}
	if !cf.Start.IsZero() || !cf.End.IsZero() {
		start, end, found = cf.Start, cf.End, true
	}
	for _, sub := range cf.Comps {
		if s, e, ok := eventTimeRange(sub); ok {
			return s, e, true
		}
	}
	return start, end, found
}

// filterByTimeRange keeps records whose event window overlaps [start, end]
// (RFC 4791 time-range semantics). An open-ended bound is treated as ±10y.
func filterByTimeRange(recs []models.Record, start, end time.Time) []models.Record {
	if start.IsZero() {
		start = end.AddDate(-10, 0, 0)
	}
	if end.IsZero() {
		end = start.AddDate(10, 0, 0)
	}
	out := make([]models.Record, 0, len(recs))
	for _, rec := range recs {
		evStart := time.Unix(rec.Date, 0)
		evEnd := evStart.Add(2 * time.Hour)
		if rec.Duration > 0 {
			evEnd = evStart.Add(time.Duration(rec.Duration) * time.Minute)
		}
		if evEnd.After(start) && evStart.Before(end) {
			out = append(out, rec)
		}
	}
	return out
}

// objectID extracts the record id (and whether the path is a task object)
// from "<CalendarPath|TasksPath><id>.ics".
func objectID(p string) (id string, isTask bool, ok bool) {
	clean := stdpath.Clean(p)
	switch {
	case strings.HasPrefix(clean, TasksPath):
		id, isTask, ok = strings.TrimPrefix(clean, TasksPath), true, true
	case strings.HasPrefix(clean, CalendarPath):
		id, isTask, ok = strings.TrimPrefix(clean, CalendarPath), false, true
	default:
		return "", false, false
	}
	if !strings.HasSuffix(id, ".ics") || strings.Contains(id, "/") || id == ".ics" {
		return "", false, false
	}
	id = strings.TrimSuffix(id, ".ics")
	if id == "" {
		return "", false, false
	}
	return id, isTask, true
}

// vtodoStatusCompleted reports whether a parsed calendar's VTODO carries
// STATUS:COMPLETED (i.e. the reminder was ticked off). Non-task or missing
// STATUS defaults to false (NEEDS-ACTION).
func vtodoStatusCompleted(cal *ical.Calendar) bool {
	if cal == nil {
		return false
	}
	for _, comp := range cal.Children {
		if comp.Name != ical.CompToDo {
			continue
		}
		if prop := comp.Props.Get("STATUS"); prop != nil {
			return strings.EqualFold(prop.Value, "COMPLETED")
		}
	}
	return false
}

func (b *Backend) loadRecords(ctx context.Context) ([]models.Record, map[string]string, error) {
	recs, err := b.DB.ListRecordsContext(ctx, db.RecordFilter{NoLimit: true})
	if err != nil {
		return nil, nil, webdav.NewHTTPError(503, fmt.Errorf("caldav: database temporarily unavailable: %w", err))
	}
	zheziNames, err := b.DB.GetZheziNames(collectZheziIDs(recs))
	if err != nil {
		zheziNames = nil // DESCRIPTION simply omits 折子 on resolution failure
	}
	return recs, zheziNames, nil
}

func (b *Backend) toCalendarObjects(recs []models.Record, zheziNames map[string]string, isTask bool, rem *ics.ReminderConfig) ([]emcaldav.CalendarObject, error) {
	out := make([]emcaldav.CalendarObject, 0, len(recs))
	for _, rec := range recs {
		co, err := b.toCalendarObject(rec, zheziNames, isTask, rem)
		if err != nil {
			return nil, err
		}
		out = append(out, co)
	}
	return out, nil
}

// toCalendarObject renders one record into a parsed ical.Calendar plus a
// content-derived ETag (so client-side change detection works on edits).
// isTask selects the VTODO projection over the default VEVENT one.
func (b *Backend) toCalendarObject(rec models.Record, zheziNames map[string]string, isTask bool, rem *ics.ReminderConfig) (emcaldav.CalendarObject, error) {
	var text string
	var objPath string
	if isTask {
		text = ics.TodoCalendar(rec, b.DB.Location(), zheziNames, rem)
		objPath = TasksPath + rec.ID + ".ics"
	} else {
		text = ics.EventCalendar(rec, b.DB.Location(), zheziNames)
		objPath = CalendarPath + rec.ID + ".ics"
	}
	co, err := calendarObjectFromICS(objPath, text, time.Unix(rec.Date, 0))
	if err != nil {
		return emcaldav.CalendarObject{}, err
	}
	return co, nil
}

// calendarObjectFromICS parses rendered ICS text into a CalendarObject with a
// content-derived ETag (first 16 bytes of SHA-256, quoted per RFC 7232).
func calendarObjectFromICS(p, text string, modTime time.Time) (emcaldav.CalendarObject, error) {
	cal, err := ical.NewDecoder(strings.NewReader(text)).Decode()
	if err != nil {
		return emcaldav.CalendarObject{}, fmt.Errorf("caldav: re-parsing generated ics: %w", err)
	}
	sum := sha256.Sum256([]byte(text))
	return emcaldav.CalendarObject{
		Path:          p,
		ModTime:       modTime,
		ContentLength: int64(len(text)),
		ETag:          `"` + hex.EncodeToString(sum[:16]) + `"`,
		Data:          cal,
	}, nil
}

// collectZheziIDs returns the deduplicated 折子 ids across records (mirrors the
// helper in internal/handlers).
func collectZheziIDs(recs []models.Record) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, rec := range recs {
		for _, zid := range rec.ZheziIDs {
			if !seen[zid] {
				seen[zid] = true
				ids = append(ids, zid)
			}
		}
	}
	return ids
}
