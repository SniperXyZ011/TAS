package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler handles GET /health (liveness) and GET /ready (readiness).
type HealthHandler struct {
	pool *pgxpool.Pool
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// Liveness responds 200 OK as long as the server process is running.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Readiness checks if the DB connection pool is reachable.
// Returns 200 if ready, 503 if not.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.pool.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable","reason":"database unreachable"}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
