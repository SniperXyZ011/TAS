package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/middleware"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/service"
)

// SyncHandler handles POST /api/v1/sync — the primary ESP32 sync endpoint.
type SyncHandler struct {
	syncSvc service.SyncService
}

// NewSyncHandler creates a SyncHandler.
func NewSyncHandler(syncSvc service.SyncService) *SyncHandler {
	return &SyncHandler{syncSvc: syncSvc}
}

// ServeHTTP handles the POST /api/v1/sync request.
func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, models.ErrorResponse{
			Error: "Method Not Allowed", Code: http.StatusMethodNotAllowed,
			Message: "only POST is accepted",
		})
		return
	}

	nodeID := middleware.NodeIDFromContext(r.Context())
	if nodeID == "" {
		writeJSON(w, http.StatusUnauthorized, models.ErrorResponse{
			Error: "Unauthorized", Code: http.StatusUnauthorized,
			Message: "node_id missing from context",
		})
		return
	}

	// Limit request body to 5 MB (prevents large payload DoS)
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	var req models.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn().Err(err).Str("node_id", nodeID).Msg("sync_handler: decode error")
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
			Error: "Bad Request", Code: http.StatusBadRequest,
			Message: "invalid JSON body: " + err.Error(),
		})
		return
	}

	resp, err := h.syncSvc.ProcessBatch(r.Context(), nodeID, req.Transactions)
	if err != nil {
		log.Error().Err(err).Str("node_id", nodeID).Msg("sync_handler: process batch error")
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
			Error: "Bad Request", Code: http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
