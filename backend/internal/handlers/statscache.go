package handlers

import (
	"net/http"
	"sync"
	"time"
)

// statsCache is a tiny single-entry TTL cache for the read-only aggregate
// endpoints (/api/stats, /api/dashboard, /api/analytics). On the low-power
// production box these endpoints cost tens of milliseconds (GetAnalytics
// alone runs ~30 queries); caching them for a few seconds turns repeated
// dashboard loads into cache hits.
//
// Freshness contract: the cache is invalidated after every successful
// mutating /api request (see Routes()), so a client never sees stale numbers
// after its own write. TTL only bounds staleness caused by other writers,
// which for this single-user deployment is none.
type statsCache[T any] struct {
	ttl time.Duration

	mu  sync.Mutex
	val T
	ok  bool
	at  time.Time
}

func newStatsCache[T any](ttl time.Duration) *statsCache[T] {
	return &statsCache[T]{ttl: ttl}
}

func (c *statsCache[T]) get() (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ok || time.Since(c.at) > c.ttl {
		var zero T
		return zero, false
	}
	return c.val, true
}

func (c *statsCache[T]) set(v T) {
	c.mu.Lock()
	c.val, c.ok, c.at = v, true, time.Now()
	c.mu.Unlock()
}

func (c *statsCache[T]) invalidate() {
	c.mu.Lock()
	c.ok = false
	c.mu.Unlock()
}

// statusCapture records the status code written by the wrapped handler so the
// invalidation middleware can skip failed mutations.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (w *statusCapture) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapture) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Flush passes through so NDJSON streaming handlers keep working.
func (w *statusCapture) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// statsInvalidationMiddleware clears the aggregate caches after any
// successful non-GET /api request. Registered once in Routes() instead of
// sprinkling invalidation calls through every mutating handler.
func statsInvalidationMiddleware(invalidate func()) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusCapture{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			if r.Method != http.MethodGet && sw.status > 0 && sw.status < 400 {
				invalidate()
			}
		})
	}
}
