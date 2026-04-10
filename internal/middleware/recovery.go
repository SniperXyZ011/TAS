package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/rs/zerolog/log"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
)

// Recovery is an HTTP middleware that catches panics, logs the stack trace,
// and returns a 500 Internal Server Error instead of crashing the server.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				log.Error().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("panic", fmt.Sprintf("%v", rec)).
					Bytes("stack", stack).
					Msg("panic recovered")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{
					Error:   "Internal Server Error",
					Code:    http.StatusInternalServerError,
					Message: "an unexpected error occurred",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs every incoming request with method, path, and remote addr.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("request")
		next.ServeHTTP(w, r)
	})
}

// ContentTypeJSON enforces that all POST/PUT requests have Content-Type: application/json.
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			ct := r.Header.Get("Content-Type")
			if ct != "application/json" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				_ = json.NewEncoder(w).Encode(models.ErrorResponse{
					Error:   "Unsupported Media Type",
					Code:    http.StatusUnsupportedMediaType,
					Message: "Content-Type must be application/json",
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
