package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/models"
)

// AmmoRepository defines the data-access contract for ammo logs.
type AmmoRepository interface {
	BatchInsert(ctx context.Context, logs []models.AmmoLog) (int, error)
}

type pgAmmoRepository struct {
	pool *pgxpool.Pool
}

// NewAmmoRepository returns a Postgres-backed AmmoRepository.
func NewAmmoRepository(pool *pgxpool.Pool) AmmoRepository {
	return &pgAmmoRepository{pool: pool}
}

func (r *pgAmmoRepository) BatchInsert(ctx context.Context, logs []models.AmmoLog) (int, error) {
	inserted := 0
	for _, l := range logs {
		txID := l.TransactionID
		var txIDParam interface{}
		if txID == "" {
			txIDParam = nil
		} else {
			txIDParam = txID
		}

		tag, err := r.pool.Exec(ctx, `
			INSERT INTO ammo_logs (node_id, transaction_id, ammo_type, delta_grams, rounds, timestamp)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, l.NodeID, txIDParam, l.AmmoType, l.DeltaGrams, l.Rounds, l.Timestamp)
		if err != nil {
			return inserted, fmt.Errorf("ammo_repo: insert: %w", err)
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}
