package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// NewPool creates and validates a pgxpool connection pool.
// It retries up to maxRetries times to allow Docker Compose healthcheck time.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: invalid database URL: %w", err)
	}

	cfg.MaxConns = 25
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 1 * time.Minute

	const maxRetries = 5
	var pool *pgxpool.Pool
	for i := 1; i <= maxRetries; i++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				break
			} else {
				pool.Close()
				err = pingErr
			}
		}
		log.Warn().Err(err).Int("attempt", i).Msg("db: waiting for postgres...")
		time.Sleep(time.Duration(i*2) * time.Second)
	}
	if err != nil {
		return nil, fmt.Errorf("db: could not connect after %d attempts: %w", maxRetries, err)
	}

	log.Info().Msg("db: connected to postgres")
	return pool, nil
}

// RunMigrations executes all .sql files in migrationsDir in lexical order.
// Files are expected to be idempotent (using IF NOT EXISTS / ON CONFLICT).
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("db: cannot read migrations dir %q: %w", migrationsDir, err)
	}

	var sqlFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(sqlFiles)

	for _, f := range sqlFiles {
		content, readErr := os.ReadFile(f)
		if readErr != nil {
			return fmt.Errorf("db: cannot read migration %s: %w", f, readErr)
		}
		if _, execErr := pool.Exec(ctx, string(content)); execErr != nil {
			return fmt.Errorf("db: migration %s failed: %w", f, execErr)
		}
		log.Info().Str("file", filepath.Base(f)).Msg("db: migration applied")
	}
	return nil
}
