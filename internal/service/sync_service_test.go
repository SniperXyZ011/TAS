package service

import (
	"context"
	"testing"
	"time"

	appCrypto "github.com/SniperXyZ011/tactical_armory_system_backend/internal/crypto"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
)

// ─── Mocks ───────────────────────────────────────────────────────────────────

const testSecret = "unit_test_secret_exactly_32bytes!!"
const testNodeID = "node-uuid-1234"

type mockTxRepo struct {
	insertCalled bool
	insertResult []repository.InsertResult
}

func (m *mockTxRepo) BatchInsert(_ context.Context, txs []models.Transaction) ([]repository.InsertResult, error) {
	m.insertCalled = true
	if m.insertResult != nil {
		return m.insertResult, nil
	}
	results := make([]repository.InsertResult, len(txs))
	for i, t := range txs {
		results[i] = repository.InsertResult{TransactionID: t.TransactionID, Inserted: true}
	}
	return results, nil
}

func (m *mockTxRepo) ListByNode(_ context.Context, _ string, _, _ int) ([]models.Transaction, error) {
	return nil, nil
}
func (m *mockTxRepo) ListAll(_ context.Context, _, _ int) ([]models.Transaction, error) {
	return nil, nil
}
func (m *mockTxRepo) CountByNode(_ context.Context, _ string) (int, error) { return 0, nil }

type mockNodeRepo struct {
	secret string
}

func (m *mockNodeRepo) Create(_ context.Context, _, _, _, _, _ string) (*repository.NodeRecord, error) {
	return &repository.NodeRecord{NodeID: testNodeID, Name: "test"}, nil
}
func (m *mockNodeRepo) FindByAPIKeyHash(_ context.Context, _ string) (*repository.NodeRecord, error) {
	return &repository.NodeRecord{NodeID: testNodeID, SecretHash: m.secret}, nil
}
func (m *mockNodeRepo) GetSecretHashByNodeID(_ context.Context, _ string) (string, error) {
	return m.secret, nil
}
func (m *mockNodeRepo) UpdateLastSeen(_ context.Context, _ string) error { return nil }
func (m *mockNodeRepo) List(_ context.Context) ([]*repository.NodeRecord, error) {
	return nil, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func makeSignedTx(id, weaponID string, action models.Action) models.Transaction {
	ts := time.Now().Unix()
	sig := appCrypto.ComputeHMAC(
		appCrypto.BuildPayload(id, testNodeID, "user-1", weaponID, string(action), ts),
		testSecret,
	)
	return models.Transaction{
		TransactionID: id,
		NodeID:        testNodeID,
		UserID:        "user-1",
		WeaponID:      weaponID,
		Action:        action,
		Quantity:      1,
		Timestamp:     ts,
		Signature:     sig,
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestProcessBatch_EmptyInput(t *testing.T) {
	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted != 0 || resp.Rejected != 0 {
		t.Errorf("expected 0 counts for empty batch, got %+v", resp)
	}
}

func TestProcessBatch_ValidBatch(t *testing.T) {
	txs := []models.Transaction{
		makeSignedTx("tx-001", "AK-47-serial-001", models.ActionCheckout),
		makeSignedTx("tx-002", "AK-47-serial-002", models.ActionCheckin),
	}

	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, txs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Accepted != 2 {
		t.Errorf("expected 2 accepted, got %d", resp.Accepted)
	}
	if resp.Rejected != 0 {
		t.Errorf("expected 0 rejected, got %d", resp.Rejected)
	}
}

func TestProcessBatch_InvalidSignature(t *testing.T) {
	tx := makeSignedTx("tx-bad-sig", "weapon-1", models.ActionCheckout)
	tx.Signature = "badhexvalue0000000000000000000000000000000000000000000000000000000"

	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, []models.Transaction{tx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Rejected != 1 {
		t.Errorf("expected 1 rejected, got %d", resp.Rejected)
	}
	if resp.Results[0].Status != "invalid_signature" {
		t.Errorf("expected status invalid_signature, got %s", resp.Results[0].Status)
	}
}

func TestProcessBatch_InvalidAction(t *testing.T) {
	ts := time.Now().Unix()
	tx := models.Transaction{
		TransactionID: "tx-bad-action",
		NodeID:        testNodeID,
		UserID:        "user-1",
		WeaponID:      "gun-1",
		Action:        models.Action("explode"), // not valid
		Timestamp:     ts,
		Signature:     "irrelevant",
	}

	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, []models.Transaction{tx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Results[0].Status != "invalid_action" {
		t.Errorf("expected invalid_action, got %s", resp.Results[0].Status)
	}
}

func TestProcessBatch_DuplicateDetected(t *testing.T) {
	tx := makeSignedTx("tx-dup-001", "weapon-1", models.ActionCheckout)

	mockRepo := &mockTxRepo{
		insertResult: []repository.InsertResult{
			{TransactionID: "tx-dup-001", Inserted: false}, // simulate duplicate
		},
	}

	svc := NewSyncService(mockRepo, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, []models.Transaction{tx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Duplicate != 1 {
		t.Errorf("expected 1 duplicate, got %d", resp.Duplicate)
	}
}

func TestProcessBatch_StalestampRejected(t *testing.T) {
	ts := time.Now().Unix() - (8 * 24 * 60 * 60) // 8 days ago — beyond 7-day window
	sig := appCrypto.ComputeHMAC(
		appCrypto.BuildPayload("tx-stale", testNodeID, "user-1", "weapon-1", "checkout", ts),
		testSecret,
	)
	tx := models.Transaction{
		TransactionID: "tx-stale",
		NodeID:        testNodeID,
		UserID:        "user-1",
		WeaponID:      "weapon-1",
		Action:        models.ActionCheckout,
		Timestamp:     ts,
		Signature:     sig,
	}

	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	resp, err := svc.ProcessBatch(context.Background(), testNodeID, []models.Transaction{tx})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Rejected != 1 {
		t.Errorf("expected 1 rejected (stale timestamp), got %d", resp.Rejected)
	}
}

func TestProcessBatch_ExceedsMaxBatchSize(t *testing.T) {
	txs := make([]models.Transaction, maxBatchSize+1)
	svc := NewSyncService(&mockTxRepo{}, &mockNodeRepo{secret: testSecret})
	_, err := svc.ProcessBatch(context.Background(), testNodeID, txs)
	if err == nil {
		t.Fatal("expected error for oversized batch")
	}
}
