// Package metrics provides a lightweight, dependency-free instrumentation
// layer for the HTTP API: per-route request counts, status distribution and
// log-spaced latency histograms, plus optional client-side Web Vitals.
//
// Design constraints (single-user app on a low-power CPU):
//   - hot path does only map lookup + atomic adds (no locks, no allocations)
//   - cardinality is bounded: routes are keyed by chi route pattern (+ method),
//     never by raw path; excess patterns collapse into an "overflow" entry
//   - everything lives in memory; nothing is persisted and nothing writes to
//     the database, so metrics collection cannot affect business data
package metrics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

// bucketBoundsMs are log-spaced upper bounds in milliseconds. The last
// implicit bucket (+Inf) catches everything above 10s.
var bucketBoundsMs = []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

const (
	maxRoutes      = 256 // cardinality guard; beyond this everything lands in overflow
	slowReqMs      = 100 // requests slower than this are logged at WARN
	clientRingSize = 128 // client Web Vitals samples kept in memory
)

type routeStat struct {
	// per-bucket counts (bucket i holds observations with
	// bounds[i-1] < d <= bounds[i]); made cumulative at scrape time.
	buckets []atomic.Int64
	// total observations and total duration in microseconds.
	count    atomic.Int64
	sumMicro atomic.Int64
	byStatus [6]atomic.Int64 // index = status code class (2..5); 0/1 unused
	lastSeen atomic.Int64    // unix nanos of most recent request
}

func newRouteStat() *routeStat {
	return &routeStat{buckets: make([]atomic.Int64, len(bucketBoundsMs)+1)}
}

func (s *routeStat) observe(d time.Duration, status int) {
	ms := float64(d.Microseconds()) / 1000.0
	idx := len(bucketBoundsMs) // +Inf bucket
	for i, b := range bucketBoundsMs {
		if ms <= b {
			idx = i
			break
		}
	}
	s.buckets[idx].Add(1)
	s.count.Add(1)
	s.sumMicro.Add(d.Microseconds())
	if status >= 200 && status < 600 {
		s.byStatus[status/100].Add(1)
	}
	s.lastSeen.Store(time.Now().UnixNano())
}

// approximate quantile from cumulative histogram counts. Returns ms.
func (s *routeStat) quantileMs(q float64) float64 {
	total := s.count.Load()
	if total == 0 {
		return 0
	}
	target := int64(q * float64(total))
	cum := int64(0)
	for i := range s.buckets {
		bucketCount := s.buckets[i].Load()
		cum += bucketCount
		if cum >= target {
			if i == 0 {
				return bucketBoundsMs[0]
			}
			if i == len(bucketBoundsMs) {
				return bucketBoundsMs[len(bucketBoundsMs)-1]
			}
			// linear interpolation inside the bucket
			lo := bucketBoundsMs[i-1]
			hi := bucketBoundsMs[i]
			prev := cum - bucketCount
			need := target - prev
			if bucketCount == 0 {
				return hi
			}
			return lo + (hi-lo)*float64(need)/float64(bucketCount)
		}
	}
	return bucketBoundsMs[len(bucketBoundsMs)-1]
}

// ClientVitals is one sample of browser-side Web Vitals for a page load.
type ClientVitals struct {
	Route    string `json:"route"`
	FCP      int64  `json:"fcp_ms,omitempty"`
	LCP      int64  `json:"lcp_ms,omitempty"`
	CLS      string `json:"cls,omitempty"`
	TTFB     int64  `json:"ttfb_ms,omitempty"`
	LongTask int64  `json:"longtask_ms,omitempty"`
	At       string `json:"at"`
}

// Metrics aggregates all instrumentation state.
type Metrics struct {
	startedAt time.Time

	mu    sync.RWMutex
	routes map[string]*routeStat // key: "METHOD route-pattern"
	spilled bool                // set when maxRoutes overflowed

	clientMu sync.Mutex
	client   []ClientVitals

	// Extra is called on every /metrics scrape (never on the hot path) to
	// attach process/db gauges, e.g. database connection-pool stats.
	Extra func() map[string]any
}

func New() *Metrics {
	return &Metrics{
		startedAt: time.Now(),
		routes:    make(map[string]*routeStat),
		client:    make([]ClientVitals, 0, clientRingSize),
	}
}

func (m *Metrics) statFor(method, pattern string) *routeStat {
	key := method + " " + pattern
	m.mu.RLock()
	s, ok := m.routes[key]
	m.mu.RUnlock()
	if ok {
		return s
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.routes[key]; ok { // double check under write lock
		return s
	}
	if len(m.routes) >= maxRoutes {
		m.spilled = true
		return m.overflowStat()
	}
	s = newRouteStat()
	m.routes[key] = s
	return s
}

func (m *Metrics) overflowStat() *routeStat {
	const key = "OVERFLOW"
	s, ok := m.routes[key]
	if !ok {
		s = newRouteStat()
		m.routes[key] = s
	}
	return s
}

// statusWriter captures the response status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Flush passes through so NDJSON streaming handlers keep working.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Middleware instruments every request. Register it on the root chi router
// (before other middlewares) so it measures the full handling time.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if pattern == "" {
				pattern = "unmatched"
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			d := time.Since(start)
			m.statFor(r.Method, pattern).observe(d, status)
			if d.Milliseconds() >= slowReqMs && pattern != "unmatched" {
				slog.Warn("slow request",
					"method", r.Method, "route", pattern, "status", status,
					"ms", d.Milliseconds())
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

// ClientVitalsHandler records one browser Web Vitals sample (fire-and-forget;
// the frontend sends via navigator.sendBeacon, which POSTs with no CORS
// preflight). Bad bodies are silently ignored on purpose: telemetry must
// never produce user-visible errors.
func (m *Metrics) ClientVitalsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() { w.WriteHeader(http.StatusNoContent) }()
		if r.Method != http.MethodPost {
			return
		}
		var v ClientVitals
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			return
		}
		if v.Route == "" || len(v.Route) > 120 {
			return
		}
		v.At = time.Now().UTC().Format(time.RFC3339)
		m.clientMu.Lock()
		m.client = append(m.client, v)
		if len(m.client) > clientRingSize {
			m.client = m.client[len(m.client)-clientRingSize:]
		}
		m.clientMu.Unlock()
	}
}

// Handler serves the metrics snapshot. JSON by default; Prometheus text
// exposition format with ?format=prometheus.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") == "prometheus" {
			w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			w.Write([]byte(m.prometheus()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m.snapshot())
	}
}

type routeJSON struct {
	Route    string   `json:"route"`
	Method   string   `json:"method"`
	Count    int64    `json:"count"`
	SumMs    float64  `json:"sum_ms"`
	P50Ms    float64  `json:"p50_ms"`
	P95Ms    float64  `json:"p95_ms"`
	P99Ms    float64  `json:"p99_ms"`
	Status2xx int64   `json:"status_2xx"`
	Status3xx int64   `json:"status_3xx"`
	Status4xx int64   `json:"status_4xx"`
	Status5xx int64   `json:"status_5xx"`
	LastSeen string  `json:"last_seen,omitempty"`
}

func (m *Metrics) snapshot() map[string]any {
	m.mu.RLock()
	type km struct {
		k string
		s *routeStat
	}
	stats := make([]km, 0, len(m.routes))
	for k, s := range m.routes {
		stats = append(stats, km{k, s})
	}
	spilled := m.spilled
	m.mu.RUnlock()

	sort.Slice(stats, func(i, j int) bool { return stats[i].k < stats[j].k })
	routes := make([]routeJSON, 0, len(stats))
	for _, e := range stats {
		method, route, _ := strings.Cut(e.k, " ")
		s := e.s
		if s.count.Load() == 0 {
			continue
		}
		rj := routeJSON{
			Route: route, Method: method,
			Count: s.count.Load(), SumMs: float64(s.sumMicro.Load()) / 1000,
			P50Ms: s.quantileMs(0.50), P95Ms: s.quantileMs(0.95), P99Ms: s.quantileMs(0.99),
			Status2xx: s.byStatus[2].Load(), Status3xx: s.byStatus[3].Load(),
			Status4xx: s.byStatus[4].Load(), Status5xx: s.byStatus[5].Load(),
		}
		if t := s.lastSeen.Load(); t > 0 {
			rj.LastSeen = time.Unix(0, t).UTC().Format(time.RFC3339)
		}
		routes = append(routes, rj)
	}

	out := map[string]any{
		"uptime_seconds":  int64(time.Since(m.startedAt).Seconds()),
		"routes_overflow": spilled,
		"routes":          routes,
		"client_vitals":   m.clientSnapshot(),
		"runtime": map[string]any{
			"goroutines":    runtime.NumGoroutine(),
			"go_version":    runtime.Version(),
			"num_cpu":       runtime.NumCPU(),
			"heap_alloc_mb": heapAllocMB(),
		},
	}
	if m.Extra != nil {
		if extra := m.Extra(); extra != nil {
			out["extra"] = extra
		}
	}
	return out
}

func (m *Metrics) clientSnapshot() []ClientVitals {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	out := make([]ClientVitals, len(m.client))
	copy(out, m.client)
	return out
}

func heapAllocMB() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc) / (1 << 20)
}

// prometheus renders the histogram in the Prometheus text exposition format.
// Metric names are prefixed mujian_ ; labels: method, route.
func (m *Metrics) prometheus() string {
	m.mu.RLock()
	type km struct {
		k string
		s *routeStat
	}
	stats := make([]km, 0, len(m.routes))
	for k, s := range m.routes {
		stats = append(stats, km{k, s})
	}
	m.mu.RUnlock()
	sort.Slice(stats, func(i, j int) bool { return stats[i].k < stats[j].k })

	var b strings.Builder
	b.WriteString("# HELP mujian_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE mujian_http_requests_total counter\n")
	b.WriteString("# HELP mujian_http_request_duration_seconds HTTP request latency.\n")
	b.WriteString("# TYPE mujian_http_request_duration_seconds histogram\n")

	for _, e := range stats {
		method, route, _ := strings.Cut(e.k, " ")
		s := e.s
		if s.count.Load() == 0 {
			continue
		}
		lbl := fmt.Sprintf("method=%q,route=%q", method, route)
		// status distribution
		for class, name := range map[int]string{2: "2xx", 3: "3xx", 4: "4xx", 5: "5xx"} {
			if v := s.byStatus[class].Load(); v > 0 {
				fmt.Fprintf(&b, "mujian_http_requests_total{%s,status=%q} %d\n", lbl, name, v)
			}
		}
		// cumulative buckets
		cum := int64(0)
		for i, bound := range bucketBoundsMs {
			cum += s.buckets[i].Load()
			fmt.Fprintf(&b, "mujian_http_request_duration_seconds_bucket{%s,le=%q} %d\n",
				lbl, strconv.FormatFloat(bound/1000, 'f', -1, 64), cum)
		}
		cum += s.buckets[len(bucketBoundsMs)].Load()
		fmt.Fprintf(&b, "mujian_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\n", lbl, cum)
		fmt.Fprintf(&b, "mujian_http_request_duration_seconds_sum{%s} %f\n", lbl, float64(s.sumMicro.Load())/1e6)
		fmt.Fprintf(&b, "mujian_http_request_duration_seconds_count{%s} %d\n", lbl, s.count.Load())
	}

	// gauges
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	b.WriteString("# TYPE mujian_runtime_goroutines gauge\n")
	fmt.Fprintf(&b, "mujian_runtime_goroutines %d\n", runtime.NumGoroutine())
	b.WriteString("# TYPE mujian_runtime_heap_alloc_bytes gauge\n")
	fmt.Fprintf(&b, "mujian_runtime_heap_alloc_bytes %d\n", ms.HeapAlloc)
	b.WriteString("# TYPE mujian_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "mujian_uptime_seconds %f\n", time.Since(m.startedAt).Seconds())

	if m.Extra != nil {
		if extra := m.Extra(); extra != nil {
			keys := make([]string, 0, len(extra))
			for k := range extra {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "# TYPE %s gauge\n", k)
				fmt.Fprintf(&b, "%s %v\n", k, extra[k])
			}
		}
	}
	return b.String()
}
