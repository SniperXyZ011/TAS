package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
)

// nodeRateLimiter holds per-node token-bucket limiters with access timestamps
// for idle eviction.
type nodeRateLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter holds the per-node limiter map and configuration.
type RateLimiter struct {
	mu      sync.Mutex
	nodes   map[string]*nodeRateLimiter
	rps     rate.Limit
	burst   int
}

// NewRateLimiter creates a RateLimiter with the given requests-per-second limit.
// Burst is set to 2× rps to accommodate occasional bursts from reconnecting offline nodes.
func NewRateLimiter(rps int) *RateLimiter {
	rl := &RateLimiter{
		nodes: make(map[string]*nodeRateLimiter),
		rps:   rate.Limit(rps),
		burst: rps * 2,
	}
	// Evict idle limiters every 5 minutes
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) getLimiter(nodeID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	nl, exists := rl.nodes[nodeID]
	if !exists {
		nl = &nodeRateLimiter{
			limiter: rate.NewLimiter(rl.rps, rl.burst),
		}
		rl.nodes[nodeID] = nl
	}
	nl.lastSeen = time.Now()
	return nl.limiter
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for id, nl := range rl.nodes {
			if time.Since(nl.lastSeen) > 10*time.Minute {
				delete(rl.nodes, id)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns an http.Handler middleware that enforces per-node rate limits.
// The node ID must already be in the context (requires NodeAuthMiddleware to run first).
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nodeID := NodeIDFromContext(r.Context())
		if nodeID == "" {
			// No node ID in context — skip rate limiting (e.g. health endpoints)
			next.ServeHTTP(w, r)
			return
		}

		limiter := rl.getLimiter(nodeID)
		if !limiter.Allow() {
			log.Warn().Str("node_id", nodeID).Msg("rate_limit: too many requests")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{
				Error:   "Too Many Requests",
				Code:    http.StatusTooManyRequests,
				Message: "rate limit exceeded — retry after 1 second",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
