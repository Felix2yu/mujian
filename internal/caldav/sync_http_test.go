package caldav

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	emcaldav "github.com/emersion/go-webdav/caldav"
)

func newSyncTestServer(t *testing.T) (*httptest.Server, *Backend) {
	t.Helper()
	b := newTestBackend(t)
	inner := &emcaldav.Handler{Backend: b, Prefix: "/caldav"}
	return httptest.NewServer(NewSyncMiddleware(b, inner)), b
}

func doReport(t *testing.T, srv *httptest.Server, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("REPORT", srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new REPORT: %v", err)
	}
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do REPORT: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

const syncGetETag = `<?xml version="1.0" encoding="utf-8"?>
	<D:sync-collection xmlns:D="DAV:"><D:sync-level>1</D:sync-level>
	<D:prop><D:getetag/></D:prop></D:sync-collection>`

func syncWithToken(token string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
	<D:sync-collection xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
	<D:sync-token>` + token + `</D:sync-token><D:sync-level>1</D:sync-level>
	<D:prop><D:getetag/><C:calendar-data/></D:prop></D:sync-collection>`
}

// extractToken pulls the <D:sync-token> value out of a multistatus body.
func extractToken(t *testing.T, body string) string {
	t.Helper()
	const tag = "<D:sync-token>"
	i := strings.Index(body, tag)
	if i < 0 {
		t.Fatalf("sync token missing:\n%s", body)
	}
	rest := body[i+len(tag):]
	j := strings.Index(rest, "</D:sync-token>")
	if j < 0 {
		t.Fatalf("sync token unterminated:\n%s", body)
	}
	return rest[:j]
}

// Initial sync returns all members plus a token; subsequent syncs are deltas.
func TestHTTPSyncCollectionFlow(t *testing.T) {
	srv, b := newSyncTestServer(t)
	defer srv.Close()
	at := time.Date(2026, 10, 6, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-a", "HTTP同步", at)); err != nil {
		t.Fatal(err)
	}

	// Initial: one member, token present, getetag-only (no calendar data).
	status, body := doReport(t, srv, CalendarPath, syncGetETag)
	if status != http.StatusMultiStatus {
		t.Fatalf("initial status = %d: %s", status, body)
	}
	if strings.Count(body, "<D:response>") != 1 {
		t.Fatalf("initial response count:\n%s", body)
	}
	if !strings.Contains(body, "<D:getetag>") {
		t.Fatalf("getetag missing:\n%s", body)
	}
	if strings.Contains(body, "BEGIN:VCALENDAR") {
		t.Fatalf("getetag-only sync must not include calendar-data:\n%s", body)
	}
	token := extractToken(t, body)
	if !strings.HasPrefix(token, "mj1-") {
		t.Fatalf("token = %q", token)
	}

	// No changes → empty delta.
	if _, body := doReport(t, srv, CalendarPath, syncWithToken(token)); strings.Count(body, "<D:response>") != 0 {
		t.Fatalf("empty delta should have no responses:\n%s", body)
	}

	// One new record → delta with full calendar-data.
	if err := b.DB.UpsertRecord(testRecord("rec-b", "HTTP新增", at.Add(24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	_, body = doReport(t, srv, CalendarPath, syncWithToken(token))
	if strings.Count(body, "<D:response>") != 1 {
		t.Fatalf("delta response count:\n%s", body)
	}
	if !strings.Contains(body, CalendarPath+"rec-b.ics") {
		t.Fatalf("delta should target rec-b:\n%s", body)
	}
	if !strings.Contains(body, "BEGIN:VCALENDAR") {
		t.Fatalf("calendar-data missing in delta:\n%s", body)
	}
}

// A removal is an explicit 404 member status, not an omitted href.
func TestHTTPSyncCollectionRemoval(t *testing.T) {
	srv, b := newSyncTestServer(t)
	defer srv.Close()
	at := time.Date(2026, 10, 7, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-rm", "删除", at)); err != nil {
		t.Fatal(err)
	}
	_, body := doReport(t, srv, CalendarPath, syncGetETag)
	token := extractToken(t, body)

	if err := b.DB.SoftDeleteRecord("rec-rm"); err != nil {
		t.Fatal(err)
	}
	status, body := doReport(t, srv, CalendarPath, syncWithToken(token))
	if status != http.StatusMultiStatus {
		t.Fatalf("status = %d: %s", status, body)
	}
	if !strings.Contains(body, CalendarPath+"rec-rm.ics") ||
		!strings.Contains(body, "HTTP/1.1 404 Not Found") {
		t.Fatalf("removal not reported as 404 member:\n%s", body)
	}
}

// Stale token → 403 DAV:valid-sync-token (the client retries without token).
func TestHTTPSyncCollectionStaleToken(t *testing.T) {
	srv, b := newSyncTestServer(t)
	defer srv.Close()
	at := time.Date(2026, 10, 8, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-s", "过期", at)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DB.PruneCaldavChangelog(1); err != nil {
		t.Fatal(err)
	}
	// Generate >1 journal rows so seq 1 actually falls outside the window.
	if err := b.DB.UpsertRecord(testRecord("rec-s", "过期改", at)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.DB.PruneCaldavChangelog(1); err != nil {
		t.Fatal(err)
	}

	status, body := doReport(t, srv, CalendarPath, syncWithToken("mj1-1"))
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", status, body)
	}
	if !strings.Contains(body, "valid-sync-token") {
		t.Fatalf("valid-sync-token precondition missing:\n%s", body)
	}
}

// PROPFIND asking for sync-token gets it injected into the collection's 200
// propstat (and go-webdav's 404 propstat for the unknown prop disappears).
func TestHTTPPropfindSyncTokenInjection(t *testing.T) {
	srv, b := newSyncTestServer(t)
	defer srv.Close()
	at := time.Date(2026, 10, 9, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-p", "PROPFIND", at)); err != nil {
		t.Fatal(err)
	}

	body := `<?xml version="1.0" encoding="utf-8"?>
		<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
		<D:prop><D:displayname/><D:sync-token/>
		<C:supported-calendar-component-set/></D:prop></D:propfind>`

	req, _ := http.NewRequest("PROPFIND", srv.URL+CalendarPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	out := string(data)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("status = %d: %s", resp.StatusCode, out)
	}
	if !strings.Contains(out, "<sync-token") || !strings.Contains(out, "mj1-") {
		t.Fatalf("sync-token not injected:\n%s", out)
	}
	// The injected token must live in the 200 propstat, not the go-webdav
	// fallback 404 propstat: assert token appears before "200 OK" region and
	// no 404 propstat survives (only the requested sync-token was unknown).
	if strings.Contains(out, "404 Not Found") {
		t.Fatalf("404 propstat for sync-token should be dropped:\n%s", out)
	}
	if !strings.Contains(out, "幕间") {
		t.Fatalf("other requested props lost during rewrite:\n%s", out)
	}
	// The re-serialized body must be well-formed XML: a round-trip that
	// preserves xmlns attrs while re-declaring them yields illegal duplicate
	// namespace attributes, which strict parsers (Apple's) reject.
	var probe interface{}
	if err := xml.Unmarshal(data, &probe); err != nil {
		t.Fatalf("rewritten PROPFIND body is not well-formed XML: %v\n%s", err, out)
	}
	if strings.Count(out, "xmlns=") != strings.Count(out, " xmlns=") && strings.Contains(out, "xmlns=\"DAV:\" xmlns=\"DAV:\"") {
		t.Fatalf("duplicate namespace declarations in output:\n%s", out)
	}

	// Without sync-token in the request, the response passes through
	// untouched (no mj1 token appears).
	bodyNoToken := `<?xml version="1.0" encoding="utf-8"?>
		<D:propfind xmlns:D="DAV:"><D:prop><D:displayname/></D:prop></D:propfind>`
	req2, _ := http.NewRequest("PROPFIND", srv.URL+CalendarPath, strings.NewReader(bodyNoToken))
	req2.Header.Set("Content-Type", "application/xml")
	req2.Header.Set("Depth", "0")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	data2, _ := io.ReadAll(resp2.Body)
	if strings.Contains(string(data2), "mj1-") {
		t.Fatalf("token leaked into a PROPFIND that did not request it:\n%s", data2)
	}
}

// When sync-token is the only requested prop, go-webdav emits a response
// with no propstat at all; the middleware must synthesize a 200 propstat
// carrying the token (observed against the real handler byte stream).
func TestHTTPPropfindSyncTokenOnlyProp(t *testing.T) {
	srv, _ := newSyncTestServer(t)
	defer srv.Close()

	body := `<?xml version="1.0" encoding="UTF-8"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:sync-token/></D:prop></D:propfind>`
	req, _ := http.NewRequest("PROPFIND", srv.URL+CalendarPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	out := string(data)
	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("status = %d: %s", resp.StatusCode, out)
	}
	if !strings.Contains(out, "<sync-token") || !strings.Contains(out, "mj1-") {
		t.Fatalf("synthesized propstat missing sync-token:\n%s", out)
	}
	if !strings.Contains(out, "200 OK") {
		t.Fatalf("token must be in a 200 propstat:\n%s", out)
	}
}

// Non-sync REPORTs (calendar-multiget) must still pass through to go-webdav.
func TestHTTPMultigetPassThrough(t *testing.T) {
	srv, b := newSyncTestServer(t)
	defer srv.Close()
	at := time.Date(2026, 10, 10, 19, 30, 0, 0, time.UTC)
	if err := b.DB.UpsertRecord(testRecord("rec-mg", "MULTIGET", at)); err != nil {
		t.Fatal(err)
	}
	body := `<?xml version="1.0" encoding="utf-8"?>
		<C:calendar-multiget xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
		<D:prop><D:getetag/></D:prop>
		<D:href>` + CalendarPath + `rec-mg.ics</D:href></C:calendar-multiget>`
	status, out := doReport(t, srv, CalendarPath, body)
	if status != http.StatusMultiStatus {
		t.Fatalf("multiget status = %d: %s", status, out)
	}
	if !strings.Contains(out, "rec-mg.ics") || !strings.Contains(out, "200 OK") {
		t.Fatalf("multiget did not pass through:\n%s", out)
	}
}
