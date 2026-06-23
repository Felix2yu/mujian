package main

import (
	"embed"
	"io/fs"
	"log"
	"mujian/internal/db"
	"mujian/internal/handlers"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed all:dist
var frontend embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/mujian.db"
	}

	database, err := db.New(dbPath)
	if err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	h := handlers.New(database)
	r.Mount("/api", h.Routes())

	// serve frontend
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

		// try to open the file
		f, err := sub.Open(path)
		if err != nil {
			// fallback to index.html for SPA routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})

	addr := "0.0.0.0:" + port
	log.Printf("mujian server starting on %s", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func init() {
	dir := filepath.Dir("./data/mujian.db")
	os.MkdirAll(dir, 0755)
}
