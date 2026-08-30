package main

import (
	"crypto/subtle"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	mujianmcp "mujian/internal/mcp"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mujian/internal/backup"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/handlers"
	"mujian/internal/metrics"
	"mujian/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// fatal logs a fatal error via slog and exits, replacing log.Fatalf so the
// whole binary uses one structured logger.
func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

// slogLogger is a structured replacement for middleware.Logger. It skips the
// high-frequency low-value paths (health checks, hashed static assets,
// uploaded covers) that dominated the log volume, and keeps everything else
// at Info with a duration field for latency eyeballing.
func slogLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		p := r.URL.Path
		if p == "/healthz" || strings.HasPrefix(p, "/uploads/") || strings.HasPrefix(p, "/_app/") {
			return
		}
		slog.Info("http",
			"method", r.Method, "path", p,
			"status", ww.Status(), "ms", time.Since(start).Milliseconds(),
			"bytes", ww.BytesWritten())
	})
}

// rootFileSystem adapts an *os.Root to http.FileSystem so uploaded files are
// served strictly from within the uploads directory. os.Root (Go 1.24) rejects
// any path escaping the subtree (including ".."), hardening against directory
// traversal at the filesystem layer.
type rootFileSystem struct{ root *os.Root }

func (r rootFileSystem) Open(name string) (http.File, error) {
	// http.FileServer passes a leading-slash path (e.g. "/covers/x.jpg").
	// os.Root.Open treats a leading slash as an absolute path and rejects it
	// ("path escapes from parent"), which surfaces as a 500 for every upload.
	// Strip the leading slash so the path resolves relative to the root;
	// os.Root still rejects ".." segments, keeping traversal protection.
	return r.root.Open(strings.TrimPrefix(name, "/"))
}

//go:embed all:dist
var frontend embed.FS

// compressibleTypes is chi's default list plus text/calendar. Passing types
// to NewCompressor replaces the default list, so this must stay a superset.
var compressibleTypes = []string{
	"text/html",
	"text/css",
	"text/plain",
	"text/javascript",
	"text/markdown",
	"text/csv",
	"text/vtt",
	"text/calendar",
	"application/javascript",
	"application/x-javascript",
	"application/json",
	"application/atom+xml",
	"application/rss+xml",
	"application/xml",
	"text/xml",
	"image/svg+xml",
}

// authMiddleware enforces the optional bearer token (MJ_AUTH_TOKEN env or
// settings.json "auth_token") on /api and /mcp. When no token is configured it
// is a pass-through, preserving the intranet deployment mode. Exemptions:
//   - GET /api/settings: the SPA fetches display settings (masked secrets)
//     before it can know whether a token is required; mutations stay locked.
//   - POST /api/metrics/client: fire-and-forget beacon that cannot set headers.
//
// The token is accepted via Authorization: Bearer, X-Auth-Token, or a ?token=
// query parameter (so calendar clients can subscribe to /api/calendar.ics).
func authMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := cfg.AuthTokenValue()
			if token == "" ||
				(r.URL.Path == "/api/settings" && r.Method == http.MethodGet) ||
				r.URL.Path == "/api/metrics/client" {
				next.ServeHTTP(w, r)
				return
			}
			provided := ""
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				provided = strings.TrimPrefix(h, "Bearer ")
			}
			if provided == "" {
				provided = r.Header.Get("X-Auth-Token")
			}
			if provided == "" {
				provided = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="mujian"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}
}

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DBPath)
	if err != nil {
		fatal("failed to init db: %v", err)
	}
	defer database.Close()
	database.SetLocation(cfg.Location())

	cfg.LoadFromFile(filepath.Join(filepath.Dir(cfg.DBPath), "settings.json"))

	// 存储后端按当前配置动态解析：设置页切换 本地↔S3 或改凭据即时生效，
	// 无需重启（storage.New 仅保留给测试使用）。
	st := storage.NewDynamic(cfg)

	// Aggregate the database connection-pool stats into /metrics on scrape
	// (never on the request hot path).
	metricsM := metrics.New()
	metricsM.Extra = func() map[string]any {
		s := database.SQLStats()
		return map[string]any{
			"mujian_db_connections_open":      s.OpenConnections,
			"mujian_db_connections_in_use":    s.InUse,
			"mujian_db_connections_idle":      s.Idle,
			"mujian_db_wait_count_total":      s.WaitCount,
			"mujian_db_wait_duration_seconds": s.WaitDuration.Seconds(),
		}
	}

	r := chi.NewRouter()
	r.Use(metricsM.Middleware) // outermost: measures the full handling time
	r.Use(slogLogger)
	r.Use(middleware.Recoverer)
	// Compression level 1 instead of 5: on the low-power production CPU the
	// level-5 CPU cost outweighed the bandwidth saving for large JSON
	// responses (measured: +75% latency on a 720KB payload), while level 1
	// keeps ~95% of the size reduction. text/calendar is added to the
	// compressible types so the 400KB+ ICS feed stops being shipped raw.
	// NOTE: passing types here REPLACES chi's default list, so the defaults
	// are spelled out again.
	r.Use(middleware.NewCompressor(1, compressibleTypes...).Handler)

	registerPprof(r)

	// Lightweight instrumentation endpoints. /metrics is scrape-friendly
	// (JSON by default, Prometheus text with ?format=prometheus); the client
	// vitals beacon is a fire-and-forget POST from the SPA.
	r.Get("/metrics", metricsM.Handler())
	r.Post("/api/metrics/client", metricsM.ClientVitalsHandler())

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("db unhealthy"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// 自动备份：快照写在数据库同目录的 backups/ 下；settings.json 先于
	// Start() 加载，启动即按已保存的间隔调度。
	backupMgr := backup.New(database, filepath.Join(filepath.Dir(cfg.DBPath), "backups"), cfg)
	backupMgr.Start()
	defer backupMgr.Stop()

	// 回收站自动清理：启动时 + 每 24 小时清掉 30 天前的软删除记录。
	go func() {
		purge := func() {
			if n, err := database.PurgeExpiredDeletedRecords(30 * 24 * time.Hour); err != nil {
				slog.Warn("purge expired trash", "err", err)
			} else if n > 0 {
				slog.Info("purged expired trash", "records", n)
			}
		}
		purge()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			purge()
		}
	}()

	h := handlers.New(database, cfg, st, backupMgr)
	r.Mount("/api", authMiddleware(cfg)(h.Routes()))

	// MCP over Streamable HTTP：与 /api 同进程同库，供 AI 客户端远程调用
	// （默认无鉴权，暴露面与 /api 一致，由反向代理/内网边界保护；配置
	// MJ_AUTH_TOKEN 后与 /api 一样要求 Bearer token）。
	r.Mount("/mcp", authMiddleware(cfg)(mujianmcp.New(database).HTTPHandler()))

	// Serve uploaded covers from the uploads dir, but constrain file access to
	// that subtree using os.Root (Go 1.24) so path traversal outside the dir
	// is rejected at the filesystem layer rather than relying on string
	// cleaning alone. Falls back to http.Dir when local storage is disabled.
	var uploadFS http.FileSystem = http.Dir(cfg.UploadDir)
	if cfg.AllowLocalStorage {
		if root, rerr := os.OpenRoot(cfg.UploadDir); rerr == nil {
			uploadFS = rootFileSystem{root}
		} else {
			slog.Warn("could not open upload dir root; serving via http.Dir fallback", "dir", cfg.UploadDir, "err", rerr)
		}
	}
	uploadCache := http.StripPrefix("/uploads/", http.FileServer(uploadFS))
	r.Handle("/uploads/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
		uploadCache.ServeHTTP(w, r)
	}))

	sub, err := fs.Sub(frontend, "dist")
	if err != nil {
		fatal("failed to get frontend dist: %v", err)
	}

	fileServer := http.FileServer(http.FS(sub))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if p, ok := strings.CutPrefix(path, "/"); ok {
			path = p
		}
		if path == "" {
			path = "index.html"
		}

		f, err := sub.Open(path)
		if err != nil {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()

		// SvelteKit emits content-hashed assets under /_app/immutable; those
		// are safe to cache forever. Everything else (the HTML shell, entry
		// chunks) must be revalidated on every load so a redeploy reaches the
		// browser immediately instead of serving a stale cached page whose
		// referenced chunks no longer exist.
		if strings.HasPrefix(path, "_app/immutable/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})

	addr := "0.0.0.0:" + cfg.Port
	slog.Info("mujian server starting", "addr", addr)

	// Explicit server with header/idle timeouts instead of the zero-value
	// http.ListenAndServe defaults, which let slow clients hold connections
	// indefinitely. ReadTimeout and WriteTimeout stay at 0 on purpose: the
	// NDJSON streaming handlers (/api/covers/thumbs, /api/covers/convert-batch)
	// legitimately run for minutes and a WriteTimeout would cut them off.
	// For an intranet, single-user deployment behind Nginx, header+idle
	// timeouts are the right trade-off.
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if err := srv.ListenAndServe(); err != nil {
		fatal("server error: %v", err)
	}
}
