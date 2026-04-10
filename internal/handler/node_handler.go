package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/service"
)

// NodeHandler handles node registration and status endpoints.
type NodeHandler struct {
	nodeSvc service.NodeService
	txRepo  repository.TransactionRepository
}

// NewNodeHandler creates a NodeHandler.
func NewNodeHandler(nodeSvc service.NodeService, txRepo repository.TransactionRepository) *NodeHandler {
	return &NodeHandler{nodeSvc: nodeSvc, txRepo: txRepo}
}

// Register handles POST /api/v1/nodes — registers a new edge node (admin only).
func (h *NodeHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, models.ErrorResponse{
			Error: "Method Not Allowed", Code: http.StatusMethodNotAllowed,
		})
		return
	}

	var req models.RegisterNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
			Error: "Bad Request", Code: http.StatusBadRequest,
			Message: "invalid JSON: " + err.Error(),
		})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, models.ErrorResponse{
			Error: "Bad Request", Code: http.StatusBadRequest,
			Message: "name is required",
		})
		return
	}
	if req.Location == "" {
		req.Location = "unknown"
	}

	resp, err := h.nodeSvc.Register(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Msg("node_handler: register error")
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{
			Error: "Internal Server Error", Code: http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// ListNodes handles GET /api/v1/nodes — lists all registered nodes (admin only).
func (h *NodeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, models.ErrorResponse{Error: "Method Not Allowed", Code: 405})
		return
	}
	nodes, err := h.nodeSvc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{Error: "Internal Server Error", Code: 500})
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// ListTransactions handles GET /api/v1/transactions — paginated transaction list.
// Query params: limit (default 50, max 200), offset (default 0), node_id (optional filter).
func (h *NodeHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, models.ErrorResponse{Error: "Method Not Allowed", Code: 405})
		return
	}

	limit := parseIntParam(r, "limit", 50)
	if limit > 200 {
		limit = 200
	}
	offset := parseIntParam(r, "offset", 0)
	nodeID := r.URL.Query().Get("node_id")

	var txs []models.Transaction
	var err error
	if nodeID != "" {
		txs, err = h.txRepo.ListByNode(r.Context(), nodeID, limit, offset)
	} else {
		txs, err = h.txRepo.ListAll(r.Context(), limit, offset)
	}

	if err != nil {
		log.Error().Err(err).Msg("node_handler: list transactions error")
		writeJSON(w, http.StatusInternalServerError, models.ErrorResponse{
			Error: "Internal Server Error", Code: 500,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transactions": txs,
		"limit":        limit,
		"offset":       offset,
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("handler: failed to write JSON response")
	}
}

func parseIntParam(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}
