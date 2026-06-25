package main

import (
	"embed"
	"io/fs"
	"log"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/handlers"
	"mujian/internal/storage"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed all:dist
var frontend embed.FS

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()
	database.SetLocation(cfg.Location())

	cfg.LoadFromFile(filepath.Join(filepath.Dir(cfg.DBPath), "settings.json"))

	st := storage.New(cfg)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	h := handlers.New(database, cfg, st)
	r.Mount("/api", h.Routes())

	uploadCache := http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir)))
	r.Handle("/uploads/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
		uploadCache.ServeHTTP(w, r)
	}))

	sub, err := fs.Sub(frontend, "dist")
	if err != nil {
		log.Fatalf("failed to get frontend dist: %w", err)
	}

	fileServer := http.FileServer(http.FS(sub))
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
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
	log.Printf("mujian server starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
