package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/config"
	appDB "github.com/SniperXyZ011/tactical_armory_system_backend/internal/db"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/handler"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/middleware"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/repository"
	"github.com/SniperXyZ011/tactical_armory_system_backend/internal/service"
)

func main() {
	// ── Logging setup ────────────────────────────────────────────────────────
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// ── Config ───────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().
		Str("port", cfg.ServerPort).
		Str("env", cfg.Env).
		Str("log_level", cfg.LogLevel).
		Msg("starting TAS backend")

	// ── Database ─────────────────────────────────────────────────────────────
	ctx := context.Background()
	pool, err := appDB.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	// Run SQL migrations
	migrationsDir := "migrations"
	if _, statErr := os.Stat(migrationsDir); os.IsNotExist(statErr) {
		log.Fatal().Str("dir", migrationsDir).Msg("migrations directory not found")
	}
	if err := appDB.RunMigrations(ctx, pool, migrationsDir); err != nil {
		log.Fatal().Err(err).Msg("failed to run migrations")
	}

	// ── Repositories ─────────────────────────────────────────────────────────
	nodeRepo := repository.NewNodeRepository(pool)
	txRepo   := repository.NewTransactionRepository(pool)
	ammoRepo := repository.NewAmmoRepository(pool)

	// ── Services ─────────────────────────────────────────────────────────────
	syncSvc := service.NewSyncService(txRepo, nodeRepo)
	nodeSvc := service.NewNodeService(nodeRepo)

	// ── Middleware ───────────────────────────────────────────────────────────
	rateLimiter    := middleware.NewRateLimiter(cfg.NodeRateLimitRPS)
	nodeAuthMw     := middleware.NodeAuthMiddleware(nodeRepo)
	adminAuthMw    := middleware.AdminAuthMiddleware(cfg.AdminAPIKey)

	// ── Handlers ─────────────────────────────────────────────────────────────
	healthH   := handler.NewHealthHandler(pool)
	syncH     := handler.NewSyncHandler(syncSvc)
	ammoH     := handler.NewAmmoSyncHandler(ammoRepo, nodeSvc)
	nodeH     := handler.NewNodeHandler(nodeSvc, txRepo)

	// ── Router ───────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Public — no auth
	mux.HandleFunc("/health", healthH.Liveness)
	mux.HandleFunc("/ready",  healthH.Readiness)

	// Node-authenticated routes (ESP32 nodes)
	nodeRoutes := http.NewServeMux()
	nodeRoutes.Handle("/api/v1/sync",      syncH)
	nodeRoutes.Handle("/api/v1/sync/ammo", ammoH)

	// Wrap node routes with: recovery → content-type → node-auth → rate-limit
	nodeStack := middleware.Recovery(
		middleware.ContentTypeJSON(
			nodeAuthMw(
				rateLimiter.Middleware(nodeRoutes),
			),
		),
	)
	mux.Handle("/api/v1/sync", nodeStack)
	mux.Handle("/api/v1/sync/ammo", nodeStack)

	// Admin-authenticated routes (dashboard/quartermaster)
	adminRoutes := http.NewServeMux()
	adminRoutes.HandleFunc("/api/v1/nodes",            nodeH.Register)
	adminRoutes.HandleFunc("/api/v1/nodes/list",       nodeH.ListNodes)
	adminRoutes.HandleFunc("/api/v1/transactions",     nodeH.ListTransactions)

	adminStack := middleware.Recovery(
		middleware.RequestLogger(
			adminAuthMw(adminRoutes),
		),
	)
	mux.Handle("/api/v1/nodes",        adminStack)
	mux.Handle("/api/v1/nodes/list",   adminStack)
	mux.Handle("/api/v1/transactions", adminStack)

	// Global recovery + logger for anything not matched
	rootStack := middleware.Recovery(middleware.RequestLogger(mux))

	// ── HTTP Server ───────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      rootStack,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info().Str("addr", srv.Addr).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-quit
	log.Info().Msg("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}
	log.Info().Msg("server exited cleanly")
}