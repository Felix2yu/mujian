package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mujian/internal/config"
)

func authTestHandler(cfg *config.Config) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	return authMiddleware(cfg)(inner)
}

func doAuthReq(h http.Handler, method, target string, header func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if header != nil {
		header(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthMiddlewareDisabled(t *testing.T) {
	h := authTestHandler(&config.Config{}) // no token configured
	for _, tc := range []struct {
		method, target string
	}{
		{"GET", "/api/records"},
		{"POST", "/mcp"},
		{"PUT", "/api/settings"},
	} {
		rec := doAuthReq(h, tc.method, tc.target, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s without token config: got %d, want 200 (pass-through)", tc.method, tc.target, rec.Code)
		}
	}
}

func TestAuthMiddlewareRequiresToken(t *testing.T) {
	h := authTestHandler(&config.Config{AuthToken: "secret-token"})

	// No / wrong credentials are rejected on every protected path.
	for _, tc := range []struct{ method, target string }{
		{"GET", "/api/records"},
		{"POST", "/api/records"},
		{"PUT", "/api/settings"},
		{"POST", "/mcp"},
	} {
		rec := doAuthReq(h, tc.method, tc.target, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: got %d, want 401", tc.method, tc.target, rec.Code)
		}
		if got := rec.Body.String(); !strings.Contains(got, "unauthorized") {
			t.Errorf("401 body should mention unauthorized, got %q", got)
		}
		if w := rec.Header().Get("WWW-Authenticate"); !strings.Contains(w, "Bearer") {
			t.Errorf("401 should set WWW-Authenticate, got %q", w)
		}

		rec = doAuthReq(h, tc.method, tc.target, func(r *http.Request) {
			r.Header.Set("X-Auth-Token", "wrong")
		})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with wrong token: got %d, want 401", tc.method, tc.target, rec.Code)
		}
	}

	// Correct token is accepted via all three transports.
	for name, header := range map[string]func(*http.Request){
		"bearer":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret-token") },
		"x-token":   func(r *http.Request) { r.Header.Set("X-Auth-Token", "secret-token") },
		"query-arg": func(r *http.Request) { r.URL.RawQuery = "token=secret-token" },
	} {
		rec := doAuthReq(h, "GET", "/api/records", header)
		if rec.Code != http.StatusOK {
			t.Errorf("correct token via %s: got %d, want 200", name, rec.Code)
		}
	}
}

func TestAuthMiddlewareExemptions(t *testing.T) {
	h := authTestHandler(&config.Config{AuthToken: "secret-token"})

	// GET /api/settings stays open so the SPA can learn auth_required before
	// it knows the token.
	rec := doAuthReq(h, "GET", "/api/settings", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/settings should be exempt, got %d", rec.Code)
	}
	// ...but mutations to settings are not.
	rec = doAuthReq(h, "PUT", "/api/settings", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("PUT /api/settings must require the token, got %d", rec.Code)
	}
	// The fire-and-forget vitals beacon cannot set headers.
	rec = doAuthReq(h, "POST", "/api/metrics/client", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/metrics/client should be exempt, got %d", rec.Code)
	}
}

func caldavTestHandler(cfg *config.Config) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	return caldavAuthMiddleware(cfg)(inner)
}

// Apple Calendar authenticates CalDAV with HTTP Basic; the shared token must
// be accepted as the Basic password, and the 401 challenge must advertise
// Basic so first-contact discovery can proceed.
func TestCaldavAuthMiddleware(t *testing.T) {
	cfg := &config.Config{AuthToken: "secret-token"}
	h := caldavTestHandler(cfg)

	// Basic auth with the token as password is accepted.
	rec := doAuthReq(h, "PROPFIND", "/caldav/user/", func(r *http.Request) {
		r.SetBasicAuth("anything", "secret-token")
	})
	if rec.Code != http.StatusOK {
		t.Errorf("Basic auth with token password should pass, got %d", rec.Code)
	}
	// Bearer is accepted too (parity with /api).
	rec = doAuthReq(h, "PROPFIND", "/caldav/user/", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer secret-token")
	})
	if rec.Code != http.StatusOK {
		t.Errorf("Bearer auth should pass, got %d", rec.Code)
	}
	// Wrong password is rejected with a Basic challenge.
	rec = doAuthReq(h, "PROPFIND", "/caldav/user/", func(r *http.Request) {
		r.SetBasicAuth("u", "wrong")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong Basic password must be 401, got %d", rec.Code)
	}
	if ch := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(ch, `Basic realm=`) {
		t.Errorf("401 must advertise Basic, got %q", ch)
	}
	// No token configured: pass-through (intranet mode).
	open := caldavTestHandler(&config.Config{})
	rec = doAuthReq(open, "PROPFIND", "/caldav/user/", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("no token configured should pass through, got %d", rec.Code)
	}
}
