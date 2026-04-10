package handler

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/middleware"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/service"
)

// AmmoSyncHandler handles POST /api/v1/sync/ammo — ammo log ingestion from load-cell sensors.
type AmmoSyncHandler struct {
	ammoRepo repository.AmmoRepository
	nodeSvc  service.NodeService
}

// NewAmmoSyncHandler creates an AmmoSyncHandler.
func NewAmmoSyncHandler(ammoRepo repository.AmmoRepository, nodeSvc service.NodeService) *AmmoSyncHandler {
	return &AmmoSyncHandler{ammoRepo: ammoRepo, nodeSvc: nodeSvc}
}

// ServeHTTP handles POST /api/v1/sync/ammo.
func (h *AmmoSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, models.ErrorResponse{
			Error: "Method Not Allowed", Code: http.StatusMethodNotAllowed,
		})
		return
	}

	nodeID := middleware.NodeIDFromContext(r.Context())
	if nodeID == "" {
		writeJSON(w, http.StatusUnauthorized, models.ErrorResponse{
			Error: "Unauthorized", Code: http.StatusUnauthorized,
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req models.AmmoSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
			Error: "Bad Request", Code: http.StatusBadRequest,
			Message: "invalid JSON: " + err.Error(),
		})
		return
	}

	if len(req.Logs) == 0 {
		writeJSON(w, http.StatusOK, models.AmmoSyncResponse{Inserted: 0})
		return
	}

	// Force node_id from authenticated context
	for i := range req.Logs {
		req.Logs[i].NodeID = nodeID
		if req.Logs[i].AmmoType == "" {
			writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
				Error:   "Bad Request",
				Code:    http.StatusBadRequest,
				Message: "ammo_type is required for all log entries",
			})
			return
		}
	}

	inserted, err := h.ammoRepo.BatchInsert(r.Context(), req.Logs)
	if err != nil {
		log.Error().Err(err).Str("node_id", nodeID).Msg("ammo_handler: insert error")
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{
			Error: "Internal Server Error", Code: http.StatusInternalServerError,
		})
		return
	}

	writeJSON(w, http.StatusOK, models.AmmoSyncResponse{Inserted: inserted})
}
