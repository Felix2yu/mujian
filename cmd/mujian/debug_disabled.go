//go:build !pprofenable

package main

import "github.com/go-chi/chi/v5"

// registerPprof is a no-op in normal (non-pprofenable) builds.
func registerPprof(r chi.Router) {}
