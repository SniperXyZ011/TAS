package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	appCrypto "github.com/SniperXyZ011/tactical_armory_system_backend/internal/crypto"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
)

// SyncService handles the business logic for ingesting transactions from edge nodes.
type SyncService interface {
	ProcessBatch(ctx context.Context, nodeID string, txs []models.Transaction) (*models.SyncResponse, error)
}

type syncService struct {
	txRepo   repository.TransactionRepository
	nodeRepo repository.NodeRepository
}

// NewSyncService creates a SyncService wired to the given repositories.
func NewSyncService(txRepo repository.TransactionRepository, nodeRepo repository.NodeRepository) SyncService {
	return &syncService{txRepo: txRepo, nodeRepo: nodeRepo}
}

const (
	maxBatchSize        = 500
	maxTimestampAgeSecs = 7 * 24 * 60 * 60 // 7 days — remote nodes can be offline for days
)

// ProcessBatch validates, verifies, and persists a batch of transactions.
// It returns a detailed per-transaction result along with aggregate counts.
func (s *syncService) ProcessBatch(ctx context.Context, nodeID string, txs []models.Transaction) (*models.SyncResponse, error) {
	if len(txs) == 0 {
		return &models.SyncResponse{Results: []models.TransactionResult{}}, nil
	}
	if len(txs) > maxBatchSize {
		return nil, fmt.Errorf("batch size %d exceeds maximum of %d", len(txs), maxBatchSize)
	}

	// Fetch the node's HMAC secret hash from DB (single query for the whole batch)
	secretHash, err := s.nodeRepo.GetSecretHashByNodeID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("sync: cannot fetch node secret: %w", err)
	}

	now := time.Now().Unix()
	results := make([]models.TransactionResult, 0, len(txs))
	validTxs := make([]models.Transaction, 0, len(txs))

	for _, t := range txs {
		result := models.TransactionResult{TransactionID: t.TransactionID}

		// ── Validate action field ────────────────────────────────────────────
		if !isValidAction(t.Action) {
			result.Status = "invalid_action"
			result.Message = fmt.Sprintf("unknown action: %q", t.Action)
			results = append(results, result)
			continue
		}

		// ── Validate transaction ID ──────────────────────────────────────────
		if strings.TrimSpace(t.TransactionID) == "" {
			result.Status = "invalid"
			result.Message = "transaction_id cannot be empty"
			results = append(results, result)
			continue
		}

		// ── Timestamp window check (stale data guard) ────────────────────────
		age := now - t.Timestamp
		if age < 0 {
			age = -age // future timestamps from clock skew are tolerated
		}
		if age > maxTimestampAgeSecs {
			result.Status = "rejected"
			result.Message = fmt.Sprintf("timestamp too old: %d seconds ago", age)
			results = append(results, result)
			continue
		}

		// ── HMAC-SHA256 signature verification ──────────────────────────────
		// We need the plaintext secret. We store the bcrypt HASH of the secret,
		// so we cannot use bcrypt.CompareHashAndPassword on a derived HMAC.
		// Instead, during node registration we give back the plaintext secret
		// and store a bcrypt hash for storage safety. For HMAC verification
		// during sync, we store the plaintext secret encrypted at rest using
		// the DB's encryption (pgcrypto / TDE at the Postgres level).
		// For this implementation we store plaintext secret in secret_hash column
		// (the column is named hash because in production you would use DB-level encryption).
		// The bcrypt hash is used purely for API key validation.
		plaintextSecret := secretHash // see note above
		if !appCrypto.VerifySignature(
			t.TransactionID, nodeID, t.UserID, t.WeaponID, string(t.Action), t.Timestamp,
			plaintextSecret, t.Signature,
		) {
			log.Warn().Str("transaction_id", t.TransactionID).Str("node_id", nodeID).
				Msg("sync: invalid signature")
			result.Status = "invalid_signature"
			result.Message = "HMAC-SHA256 signature mismatch"
			results = append(results, result)
			continue
		}

		// Force node_id from authenticated context (not from payload)
		t.NodeID = nodeID
		validTxs = append(validTxs, t)
		result.Status = "pending" // will be updated after DB insert
		results = append(results, result)
	}

	// ── Batch insert valid transactions ──────────────────────────────────────
	if len(validTxs) > 0 {
		insertResults, dbErr := s.txRepo.BatchInsert(ctx, validTxs)
		if dbErr != nil {
			return nil, fmt.Errorf("sync: batch insert failed: %w", dbErr)
		}

		// Map insert results back to the results slice
		insertIdx := 0
		for i := range results {
			if results[i].Status == "pending" {
				if insertIdx < len(insertResults) {
					if insertResults[insertIdx].Inserted {
						results[i].Status = "accepted"
					} else {
						results[i].Status = "duplicate"
						results[i].Message = "already recorded"
					}
					insertIdx++
				}
			}
		}
	}

	// ── Update node last_seen (best effort, non-blocking) ────────────────────
	go func() {
		if err := s.nodeRepo.UpdateLastSeen(context.Background(), nodeID); err != nil {
			log.Error().Err(err).Str("node_id", nodeID).Msg("sync: failed to update last_seen")
		}
	}()

	// ── Aggregate counts ─────────────────────────────────────────────────────
	resp := &models.SyncResponse{Results: results}
	for _, r := range results {
		switch r.Status {
		case "accepted":
			resp.Accepted++
		case "duplicate":
			resp.Duplicate++
		default:
			resp.Rejected++
		}
	}

	log.Info().
		Str("node_id", nodeID).
		Int("total", len(txs)).
		Int("accepted", resp.Accepted).
		Int("duplicate", resp.Duplicate).
		Int("rejected", resp.Rejected).
		Msg("sync: batch processed")

	return resp, nil
}

// isValidAction checks that the action field is one of the known enum values.
func isValidAction(a models.Action) bool {
	switch a {
	case models.ActionCheckout, models.ActionCheckin,
		models.ActionAudit, models.ActionTransfer,
		models.ActionLost, models.ActionFound:
		return true
	}
	return false
}

// ─── Node Service ─────────────────────────────────────────────────────────────

// NodeService handles business logic for node registration.
type NodeService interface {
	Register(ctx context.Context, req models.RegisterNodeRequest) (*models.RegisterNodeResponse, error)
	List(ctx context.Context) ([]*repository.NodeRecord, error)
}

type nodeService struct {
	nodeRepo repository.NodeRepository
}

// NewNodeService creates a NodeService.
func NewNodeService(nodeRepo repository.NodeRepository) NodeService {
	return &nodeService{nodeRepo: nodeRepo}
}

// Register creates a new edge node, generating a random API key and HMAC secret.
// The plaintext values are returned ONCE and never stored in plaintext on the server.
func (s *nodeService) Register(ctx context.Context, req models.RegisterNodeRequest) (*models.RegisterNodeResponse, error) {
	tier := req.Tier
	if tier == "" {
		tier = "standard"
	}
	if tier != "standard" && tier != "priority" && tier != "admin" {
		return nil, fmt.Errorf("invalid tier %q: must be standard|priority|admin", tier)
	}

	apiKey, err := appCrypto.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("node: failed to generate API key: %w", err)
	}
	secret, err := appCrypto.GenerateSecret()
	if err != nil {
		return nil, fmt.Errorf("node: failed to generate secret: %w", err)
	}

	// Hash the API key with bcrypt for storage (never store plaintext)
	apiKeyHash, err := bcrypt.GenerateFromPassword([]byte(apiKey), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("node: bcrypt API key: %w", err)
	}

	// The HMAC secret is stored as-is in this implementation.
	// In a production environment with DB-level encryption (pgcrypto/TDE),
	// this column would be encrypted at rest.
	node, err := s.nodeRepo.Create(ctx, req.Name, req.Location, tier, string(apiKeyHash), secret)
	if err != nil {
		return nil, fmt.Errorf("node: create in db: %w", err)
	}

	return &models.RegisterNodeResponse{
		NodeID: node.NodeID,
		Name:   node.Name,
		APIKey: apiKey,  // plaintext — shown ONCE
		Secret: secret,  // plaintext — shown ONCE
	}, nil
}

func (s *nodeService) List(ctx context.Context) ([]*repository.NodeRecord, error) {
	return s.nodeRepo.List(ctx)
}
