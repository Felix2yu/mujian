package handlers

import (
	"context"
	"net/http"
)

type contextKey string

const userIDKey contextKey = "userID"

func (h *Handler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		userID, ok := h.sessions.Get(cookie.Value)
		if !ok {
			http.Error(w, `{"error":"invalid session"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUserID(r *http.Request) int64 {
	if id, ok := r.Context().Value(userIDKey).(int64); ok {
		return id
	}
	return 0
}
