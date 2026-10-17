package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestObserveAndQuantiles(t *testing.T) {
	s := newRouteStat()
	// 10 observations: 1..10 ms
	for i := 1; i <= 10; i++ {
		s.observe(time.Duration(i)*time.Millisecond, http.StatusOK)
	}
	if got := s.count.Load(); got != 10 {
		t.Fatalf("count = %d, want 10", got)
	}
	if got := s.quantileMs(0.5); got < 4 || got > 6 {
		t.Fatalf("p50 = %v, want ~5", got)
	}
	if got := s.quantileMs(0.95); got < 9 || got > 10.01 {
		t.Fatalf("p95 = %v, want ~9-10", got)
	}
	if got := s.byStatus[2].Load(); got != 10 {
		t.Fatalf("2xx = %d, want 10", got)
	}
}

func TestMiddlewareRecordsRoutePattern(t *testing.T) {
	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/api/records", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	r.Get("/api/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/records", nil))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/records/abc", nil))

	snap := m.snapshot()
	routes, _ := snap["routes"].([]routeJSON)
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2 (pattern-keyed, not raw path)", len(routes))
	}
	seen := map[string]int64{}
	for _, rj := range routes {
		seen[rj.Route] = rj.Count
	}
	if seen["/api/records"] != 1 || seen["/api/records/{id}"] != 1 {
		t.Fatalf("per-pattern counts wrong: %v", seen)
	}
}

func TestMiddlewareKeepsStreamingFlush(t *testing.T) {
	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/stream", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer lost Flusher support")
		}
		w.Write([]byte("line\n"))
		fl.Flush()
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/stream", nil))
	if !strings.Contains(rec.Body.String(), "line") {
		t.Fatal("stream body missing")
	}
}

func TestPrometheusOutput(t *testing.T) {
	m := New()
	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/api/ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r.Get("/api/bad", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/ok", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/bad", nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics?format=prometheus", nil)
	m.Handler()(rec, req)

	out := rec.Body.String()
	for _, want := range []string{
		`mujian_http_requests_total{method="GET",route="/api/ok",status="2xx"} 1`,
		`mujian_http_requests_total{method="GET",route="/api/bad",status="5xx"} 1`,
		`mujian_http_request_duration_seconds_bucket{method="GET",route="/api/ok",le="+Inf"} 1`,
		`mujian_http_request_duration_seconds_count{method="GET",route="/api/ok"} 1`,
		"mujian_runtime_goroutines",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("prometheus output missing %q\nGot:\n%s", want, out)
		}
	}
}

func TestClientVitalsHandler(t *testing.T) {
	m := New()
	h := m.ClientVitalsHandler()

	body := `{"route":"/","fcp_ms":120,"lcp_ms":300,"ttfb_ms":3}`
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/api/metrics/client", strings.NewReader(body)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	// malformed body must be swallowed silently
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest("POST", "/api/metrics/client", strings.NewReader("{bad")))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("bad body status = %d, want 204", rec.Code)
	}

	snap := m.snapshot()
	vitals, _ := snap["client_vitals"].([]ClientVitals)
	if len(vitals) != 1 || vitals[0].FCP != 120 || vitals[0].Route != "/" {
		t.Fatalf("client vitals = %+v", vitals)
	}
}

func TestClientVitalsRingBuffer(t *testing.T) {
	m := New()
	h := m.ClientVitalsHandler()
	for i := 0; i < clientRingSize+20; i++ {
		h(httptest.NewRecorder(), httptest.NewRequest(
			"POST", "/api/metrics/client",
			strings.NewReader(`{"route":"/"}`)))
	}
	if got := len(m.clientSnapshot()); got != clientRingSize {
		t.Fatalf("ring size = %d, want %d", got, clientRingSize)
	}
}

func TestSnapshotIsJSON(t *testing.T) {
	m := New()
	rec := httptest.NewRecorder()
	m.Handler()(rec, httptest.NewRequest("GET", "/metrics", nil))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}
}

func TestQuantileEmpty(t *testing.T) {
	s := newRouteStat()
	if got := s.quantileMs(0.5); got != 0 {
		t.Fatalf("empty quantile = %v, want 0", got)
	}
}
