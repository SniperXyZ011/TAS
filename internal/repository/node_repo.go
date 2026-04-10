package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeRecord is what the repository layer returns from the DB.
type NodeRecord struct {
	NodeID      string
	Name        string
	Location    string
	APIKeyHash  string
	SecretHash  string
	Tier        string
	IsActive    bool
	CreatedAt   time.Time
	LastSeenAt  *time.Time
}

// NodeRepository defines the data-access contract for nodes.
type NodeRepository interface {
	Create(ctx context.Context, name, location, tier, apiKeyHash, secretHash string) (*NodeRecord, error)
	FindByAPIKeyHash(ctx context.Context, hash string) (*NodeRecord, error)
	GetSecretHashByNodeID(ctx context.Context, nodeID string) (string, error)
	UpdateLastSeen(ctx context.Context, nodeID string) error
	List(ctx context.Context) ([]*NodeRecord, error)
}

type pgNodeRepository struct {
	pool *pgxpool.Pool
}

// NewNodeRepository returns a Postgres-backed NodeRepository.
func NewNodeRepository(pool *pgxpool.Pool) NodeRepository {
	return &pgNodeRepository{pool: pool}
}

func (r *pgNodeRepository) Create(ctx context.Context, name, location, tier, apiKeyHash, secretHash string) (*NodeRecord, error) {
	var rec NodeRecord
	err := r.pool.QueryRow(ctx, `
		INSERT INTO nodes (name, location, tier, api_key_hash, secret_hash)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING node_id, name, location, tier, is_active, created_at
	`, name, location, tier, apiKeyHash, secretHash).Scan(
		&rec.NodeID, &rec.Name, &rec.Location, &rec.Tier, &rec.IsActive, &rec.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("node_repo: create: %w", err)
	}
	return &rec, nil
}

func (r *pgNodeRepository) FindByAPIKeyHash(ctx context.Context, hash string) (*NodeRecord, error) {
	var rec NodeRecord
	err := r.pool.QueryRow(ctx, `
		SELECT node_id, name, location, api_key_hash, secret_hash, tier, is_active, created_at, last_seen_at
		FROM nodes WHERE api_key_hash = $1 AND is_active = TRUE
	`, hash).Scan(
		&rec.NodeID, &rec.Name, &rec.Location, &rec.APIKeyHash, &rec.SecretHash,
		&rec.Tier, &rec.IsActive, &rec.CreatedAt, &rec.LastSeenAt,
	)
	if err != nil {
		return nil, fmt.Errorf("node_repo: find by hash: %w", err)
	}
	return &rec, nil
}

func (r *pgNodeRepository) GetSecretHashByNodeID(ctx context.Context, nodeID string) (string, error) {
	var secretHash string
	err := r.pool.QueryRow(ctx, `
		SELECT secret_hash FROM nodes WHERE node_id = $1 AND is_active = TRUE
	`, nodeID).Scan(&secretHash)
	if err != nil {
		return "", fmt.Errorf("node_repo: get secret: %w", err)
	}
	return secretHash, nil
}

func (r *pgNodeRepository) UpdateLastSeen(ctx context.Context, nodeID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE nodes SET last_seen_at = NOW() WHERE node_id = $1
	`, nodeID)
	if err != nil {
		return fmt.Errorf("node_repo: update last_seen: %w", err)
	}
	return nil
}

func (r *pgNodeRepository) List(ctx context.Context) ([]*NodeRecord, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT node_id, name, location, tier, is_active, created_at, last_seen_at
		FROM nodes ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("node_repo: list: %w", err)
	}
	defer rows.Close()

	var nodes []*NodeRecord
	for rows.Next() {
		var rec NodeRecord
		if err := rows.Scan(
			&rec.NodeID, &rec.Name, &rec.Location, &rec.Tier, &rec.IsActive, &rec.CreatedAt, &rec.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("node_repo: list scan: %w", err)
		}
		nodes = append(nodes, &rec)
	}
	return nodes, rows.Err()
}
