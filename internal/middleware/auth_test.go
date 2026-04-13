package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
)

// ─── Mock NodeRepository ──────────────────────────────────────────────────────

type mockNodeRepo struct {
	nodes []*repository.NodeRecord
}

func (m *mockNodeRepo) Create(_ context.Context, _, _, _, _, _ string) (*repository.NodeRecord, error) {
	return nil, nil
}
func (m *mockNodeRepo) FindByAPIKeyHash(_ context.Context, _ string) (*repository.NodeRecord, error) {
	return nil, nil
}
func (m *mockNodeRepo) GetSecretHashByNodeID(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockNodeRepo) UpdateLastSeen(_ context.Context, _ string) error { return nil }
func (m *mockNodeRepo) List(_ context.Context) ([]*repository.NodeRecord, error) {
	return m.nodes, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func hashKey(t *testing.T, key string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash failed: %v", err)
	}
	return string(h)
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestNodeAuth_MissingHeader ensures a missing X-API-Key returns 401.
func TestNodeAuth_MissingHeader(t *testing.T) {
	repo := &mockNodeRepo{}
	mw := NodeAuthMiddleware(repo)(http.HandlerFunc(okHandler))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing X-API-Key, got %d", rr.Code)
	}
}

// TestNodeAuth_InvalidKey ensures a wrong API key returns 401.
func TestNodeAuth_InvalidKey(t *testing.T) {
	validHash := hashKey(t, "correct-key")
	repo := &mockNodeRepo{
		nodes: []*repository.NodeRecord{
			{
				NodeID:     "node-abc",
				Name:       "Test Node",
				APIKeyHash: validHash, // <-- THIS is what the bug was: List() never returned this field
				Tier:       "standard",
				IsActive:   true,
				CreatedAt:  time.Now(),
			},
		},
	}
	mw := NodeAuthMiddleware(repo)(http.HandlerFunc(okHandler))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", rr.Code)
	}
}

// TestNodeAuth_ValidKey ensures a correct API key passes auth and injects node_id into context.
// This test specifically guards against the regression where List() omitted api_key_hash,
// causing APIKeyHash to be "" and all bcrypt comparisons to fail even with the correct key.
func TestNodeAuth_ValidKey(t *testing.T) {
	const plainKey = "super-secret-node-key-abc123"
	validHash := hashKey(t, plainKey)

	repo := &mockNodeRepo{
		nodes: []*repository.NodeRecord{
			{
				NodeID:     "node-xyz",
				Name:       "Alpha Node",
				APIKeyHash: validHash, // must be populated — regression guard
				Tier:       "standard",
				IsActive:   true,
				CreatedAt:  time.Now(),
			},
		},
	}

	var capturedNodeID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedNodeID = NodeIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := NodeAuthMiddleware(repo)(next)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	req.Header.Set("X-API-Key", plainKey)
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid key, got %d", rr.Code)
	}
	if capturedNodeID != "node-xyz" {
		t.Errorf("expected node_id 'node-xyz' in context, got %q", capturedNodeID)
	}
}

// TestNodeAuth_EmptyAPIKeyHash_RejectsEvenCorrectKey documents the bug:
// if List() returns a NodeRecord with an empty APIKeyHash (the old buggy behaviour),
// bcrypt.CompareHashAndPassword always fails, so the node can never authenticate.
func TestNodeAuth_EmptyAPIKeyHash_RejectsEvenCorrectKey(t *testing.T) {
	repo := &mockNodeRepo{
		nodes: []*repository.NodeRecord{
			{
				NodeID:     "node-buggy",
				APIKeyHash: "", // simulates the old List() bug — hash not fetched from DB
				IsActive:   true,
				CreatedAt:  time.Now(),
			},
		},
	}
	mw := NodeAuthMiddleware(repo)(http.HandlerFunc(okHandler))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", nil)
	req.Header.Set("X-API-Key", "any-key")
	mw.ServeHTTP(rr, req)

	// With empty hash in the mock, auth must fail — this validates the bug scenario.
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when APIKeyHash is empty (old bug), got %d", rr.Code)
	}
}

// TestAdminAuth_ValidKey ensures the admin middleware passes with the correct key.
func TestAdminAuth_ValidKey(t *testing.T) {
	mw := AdminAuthMiddleware("admin-secret")(http.HandlerFunc(okHandler))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid admin key, got %d", rr.Code)
	}
}

// TestAdminAuth_InvalidKey ensures wrong admin key returns 401.
func TestAdminAuth_InvalidKey(t *testing.T) {
	mw := AdminAuthMiddleware("admin-secret")(http.HandlerFunc(okHandler))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes", nil)
	req.Header.Set("X-Admin-Key", "wrong-admin-key")
	mw.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid admin key, got %d", rr.Code)
	}
}
