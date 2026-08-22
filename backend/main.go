package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/handlers"
	"mujian/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// fatal logs a fatal error via slog and exits, replacing log.Fatalf so the
// whole binary uses one structured logger.
func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
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

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DBPath)
	if err != nil {
		fatal("failed to init db: %v", err)
	}
	defer database.Close()
	database.SetLocation(cfg.Location())

	cfg.LoadFromFile(filepath.Join(filepath.Dir(cfg.DBPath), "settings.json"))

	st := storage.New(cfg)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	registerPprof(r)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("db unhealthy"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	h := handlers.New(database, cfg, st)
	r.Mount("/api", h.Routes())

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
		fileServer.ServeHTTP(w, r)
	})

	addr := "0.0.0.0:" + cfg.Port
	slog.Info("mujian server starting", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		fatal("server error: %v", err)
	}
}
