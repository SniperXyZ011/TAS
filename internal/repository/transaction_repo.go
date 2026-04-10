package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
)

// InsertResult describes the outcome of a single transaction insert attempt.
type InsertResult struct {
	TransactionID string
	Inserted      bool // false = duplicate (ON CONFLICT DO NOTHING)
}

// TransactionRepository defines the data-access contract for transactions.
type TransactionRepository interface {
	BatchInsert(ctx context.Context, txs []models.Transaction) ([]InsertResult, error)
	ListByNode(ctx context.Context, nodeID string, limit, offset int) ([]models.Transaction, error)
	ListAll(ctx context.Context, limit, offset int) ([]models.Transaction, error)
	CountByNode(ctx context.Context, nodeID string) (int, error)
}

type pgTransactionRepository struct {
	pool *pgxpool.Pool
}

// NewTransactionRepository returns a Postgres-backed TransactionRepository.
func NewTransactionRepository(pool *pgxpool.Pool) TransactionRepository {
	return &pgTransactionRepository{pool: pool}
}

// BatchInsert inserts a slice of transactions in a single DB batch.
// ON CONFLICT (transaction_id) DO NOTHING handles deduplication at the DB level.
// Returns per-transaction InsertResult indicating whether each was inserted or skipped.
func (r *pgTransactionRepository) BatchInsert(ctx context.Context, txs []models.Transaction) ([]InsertResult, error) {
	results := make([]InsertResult, len(txs))
	for i, t := range txs {
		results[i].TransactionID = t.TransactionID
	}

	batch := &pgx.Batch{}
	for _, t := range txs {
		notes := t.Notes
		qty := t.Quantity
		if qty == 0 {
			qty = 1
		}
		batch.Queue(`
			INSERT INTO transactions
				(transaction_id, node_id, user_id, weapon_id, action, quantity, notes, timestamp, signature, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'accepted')
			ON CONFLICT (transaction_id) DO NOTHING
		`, t.TransactionID, t.NodeID, t.UserID, t.WeaponID, string(t.Action), qty, notes, t.Timestamp, t.Signature)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range txs {
		tag, err := br.Exec()
		if err != nil {
			return nil, fmt.Errorf("transaction_repo: batch insert row %d: %w", i, err)
		}
		results[i].Inserted = tag.RowsAffected() == 1
	}
	return results, nil
}

func (r *pgTransactionRepository) ListByNode(ctx context.Context, nodeID string, limit, offset int) ([]models.Transaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT transaction_id, node_id, user_id, weapon_id, action, quantity, COALESCE(notes,''), timestamp, signature
		FROM transactions
		WHERE node_id = $1
		ORDER BY timestamp DESC
		LIMIT $2 OFFSET $3
	`, nodeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("transaction_repo: list by node: %w", err)
	}
	return scanTransactions(rows)
}

func (r *pgTransactionRepository) ListAll(ctx context.Context, limit, offset int) ([]models.Transaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT transaction_id, node_id, user_id, weapon_id, action, quantity, COALESCE(notes,''), timestamp, signature
		FROM transactions
		ORDER BY timestamp DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("transaction_repo: list all: %w", err)
	}
	return scanTransactions(rows)
}

func (r *pgTransactionRepository) CountByNode(ctx context.Context, nodeID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM transactions WHERE node_id = $1`, nodeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("transaction_repo: count: %w", err)
	}
	return count, nil
}

func scanTransactions(rows pgx.Rows) ([]models.Transaction, error) {
	defer rows.Close()
	var txs []models.Transaction
	for rows.Next() {
		var t models.Transaction
		var action string
		if err := rows.Scan(
			&t.TransactionID, &t.NodeID, &t.UserID, &t.WeaponID,
			&action, &t.Quantity, &t.Notes, &t.Timestamp, &t.Signature,
		); err != nil {
			return nil, fmt.Errorf("transaction_repo: scan: %w", err)
		}
		t.Action = models.Action(action)
		txs = append(txs, t)
	}
	return txs, rows.Err()
}
