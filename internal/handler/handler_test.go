package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appCrypto "github.com/SniperXyZ011/tactical_armory_system_backend/internal/crypto"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/middleware"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/service"
)

// ─── Mocks ────────────────────────────────────────────────────────────────────

const handlerTestSecret = "handler_test_secret_32bytes_long!!"
const handlerTestNodeID = "test-node-uuid-handler"

type mockSyncSvc struct {
	resp *models.SyncResponse
	err  error
}

func (m *mockSyncSvc) ProcessBatch(_ context.Context, _ string, txs []models.Transaction) (*models.SyncResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.resp != nil {
		return m.resp, nil
	}
	return &models.SyncResponse{
		Accepted: len(txs),
		Results:  make([]models.TransactionResult, len(txs)),
	}, nil
}

type mockNodeSvc struct{}

func (m *mockNodeSvc) Register(_ context.Context, req models.RegisterNodeRequest) (*models.RegisterNodeResponse, error) {
	return &models.RegisterNodeResponse{
		NodeID: "new-node-uuid",
		Name:   req.Name,
		APIKey: "generated-api-key",
		Secret: "generated-secret",
	}, nil
}
func (m *mockNodeSvc) List(_ context.Context) ([]*repository.NodeRecord, error) {
	return []*repository.NodeRecord{}, nil
}

type mockTxRepoHandler struct{}

func (m *mockTxRepoHandler) BatchInsert(_ context.Context, _ []models.Transaction) ([]repository.InsertResult, error) {
	return nil, nil
}
func (m *mockTxRepoHandler) ListByNode(_ context.Context, _ string, _, _ int) ([]models.Transaction, error) {
	return []models.Transaction{}, nil
}
func (m *mockTxRepoHandler) ListAll(_ context.Context, _, _ int) ([]models.Transaction, error) {
	return []models.Transaction{}, nil
}
func (m *mockTxRepoHandler) CountByNode(_ context.Context, _ string) (int, error) { return 0, nil }

// ─── contextWith injects node_id into request context ─────────────────────────

func ctxWithNode(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.ContextNodeID, handlerTestNodeID)
	return r.WithContext(ctx)
}

// ─── SyncHandler Tests ────────────────────────────────────────────────────────

func TestSyncHandler_WrongMethod(t *testing.T) {
	h := NewSyncHandler(&mockSyncSvc{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sync", nil)
	req = ctxWithNode(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestSyncHandler_NoNodeInContext(t *testing.T) {
	h := NewSyncHandler(&mockSyncSvc{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync",
		bytes.NewBufferString(`{"transactions":[]}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSyncHandler_InvalidJSON(t *testing.T) {
	h := NewSyncHandler(&mockSyncSvc{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync",
		bytes.NewBufferString(`not json`))
	req.Header.Set("Content-Type", "application/json")
	req = ctxWithNode(req)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSyncHandler_ValidBatch(t *testing.T) {
	ts := time.Now().Unix()
	sig := appCrypto.ComputeHMAC(
		appCrypto.BuildPayload("tx-h-001", handlerTestNodeID, "user1", "gun1", "checkout", ts),
		handlerTestSecret,
	)
	batch := models.SyncRequest{
		Transactions: []models.Transaction{
			{
				TransactionID: "tx-h-001",
				NodeID:        handlerTestNodeID,
				UserID:        "user1",
				WeaponID:      "gun1",
				Action:        models.ActionCheckout,
				Timestamp:     ts,
				Signature:     sig,
			},
		},
	}

	body, _ := json.Marshal(batch)
	h := NewSyncHandler(&mockSyncSvc{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = ctxWithNode(req)

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// ─── NodeHandler Tests ────────────────────────────────────────────────────────

func TestNodeHandler_Register_MissingName(t *testing.T) {
	svc := service.NewNodeService(nil) // will not be called for validation error
	_ = svc
	h := NewNodeHandler(&mockNodeSvc{}, &mockTxRepoHandler{})

	body := bytes.NewBufferString(`{"location":"FOB-Alpha"}`)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", body)
	req.Header.Set("Content-Type", "application/json")

	h.Register(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", rr.Code)
	}
}

func TestNodeHandler_Register_Success(t *testing.T) {
	h := NewNodeHandler(&mockNodeSvc{}, &mockTxRepoHandler{})
	body, _ := json.Marshal(models.RegisterNodeRequest{Name: "ESP32-Node-1", Location: "FOB-Alpha"})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	h.Register(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d — %s", rr.Code, rr.Body.String())
	}

	var resp models.RegisterNodeResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.APIKey == "" || resp.Secret == "" {
		t.Error("register response must include plaintext APIKey and Secret")
	}
}

func TestNodeHandler_ListTransactions(t *testing.T) {
	h := NewNodeHandler(&mockNodeSvc{}, &mockTxRepoHandler{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions?limit=10&offset=0", nil)

	h.ListTransactions(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ─── Health Handler Tests ─────────────────────────────────────────────────────

func TestHealthLiveness(t *testing.T) {
	h := &HealthHandler{} // pool is nil but liveness doesn't use it
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.Liveness(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
