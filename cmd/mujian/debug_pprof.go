//go:build pprofenable

package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/go-chi/chi/v5"
)

// registerPprof exposes net/http/pprof under /debug when the binary is built
// with the "pprofenable" build tag. It is used to collect a CPU profile for
// Profile-Guided Optimization (PGO) via /debug/pprof/profile?seconds=N.
func registerPprof(r chi.Router) {
	r.Mount("/debug", http.DefaultServeMux)
}
