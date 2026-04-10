package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
)

type contextKey string

const (
	// ContextNodeID is the key used to store the authenticated node ID in the request context.
	ContextNodeID contextKey = "node_id"
	// ContextNodeTier stores the node's tier for downstream rate-limit decisions.
	ContextNodeTier contextKey = "node_tier"
)

// NodeAuthMiddleware validates the X-API-Key header against bcrypt hashes stored
// in the nodes table. If valid, it attaches the node_id to the request context.
func NodeAuthMiddleware(nodeRepo repository.NodeRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				writeAuthError(w, "X-API-Key header is required", http.StatusUnauthorized)
				return
			}

			// Retrieve all active nodes and compare bcrypt hashes.
			// In practice with many nodes, you'd cache this lookup.
			// For MVP: bcrypt.CompareHashAndPassword is the safe check.
			nodes, err := nodeRepo.List(r.Context())
			if err != nil {
				log.Error().Err(err).Msg("auth: failed to list nodes for API key check")
				writeAuthError(w, "internal error", http.StatusInternalServerError)
				return
			}

			var matchedNode *repository.NodeRecord
			for _, n := range nodes {
				if bcrypt.CompareHashAndPassword([]byte(n.APIKeyHash), []byte(apiKey)) == nil {
					matchedNode = n
					break
				}
			}

			if matchedNode == nil {
				log.Warn().Str("remote_addr", r.RemoteAddr).Msg("auth: invalid API key")
				writeAuthError(w, "invalid API key", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ContextNodeID, matchedNode.NodeID)
			ctx = context.WithValue(ctx, ContextNodeTier, matchedNode.Tier)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminAuthMiddleware validates the X-Admin-Key header against the configured admin key.
func AdminAuthMiddleware(adminAPIKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Admin-Key")
			if key != adminAPIKey {
				log.Warn().Str("remote_addr", r.RemoteAddr).Msg("admin_auth: invalid key")
				writeAuthError(w, "invalid admin key", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NodeIDFromContext extracts the authenticated node ID from a request context.
// Returns empty string if not set.
func NodeIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextNodeID).(string)
	return v
}

func writeAuthError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(models.ErrorResponse{
		Error:   http.StatusText(code),
		Code:    code,
		Message: msg,
	})
}
