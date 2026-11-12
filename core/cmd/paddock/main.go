// Command paddock is the core process: REST API, ingest, experiments and setups in one binary.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/api"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/config"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/features"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/telemetry"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
	spoolPollInterval = time.Second
	jobPollInterval   = time.Second
	// The agent writes each spool file to a temporary name and renames it, so a visible file
	// is complete; the settle time only covers file systems where a rename is not atomic.
	spoolSettleTime = time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("paddock stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.FromEnv(os.Getenv)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := openDatabase(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()
	metrics, err := telemetry.NewMetrics()
	if err != nil {
		return err
	}
	if err := startIngest(ctx, cfg, database, logger, metrics); err != nil {
		return err
	}
	analysis := engine.New(cfg.EngineURL)
	if err := startFeatures(ctx, cfg, database, analysis, logger, metrics); err != nil {
		return err
	}
	outbox := experiments.Outbox{Dir: cfg.OutboxDir}
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: api.NewHandler(api.Deps{
			Logger: logger, Version: version, Database: database,
			Sessions:    store.NewReader(database),
			Experiments: experiments.NewService(database, analysis, outbox, logger),
			Metrics:     metrics.Handler,
		}),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	return serve(ctx, server, logger)
}

func openDatabase(ctx context.Context, cfg config.Config, logger *slog.Logger) (*sql.DB, error) {
	database, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	applied, err := store.MigrateUp(ctx, database)
	if err != nil {
		return nil, errors.Join(err, database.Close())
	}
	logger.Info("database ready", "migrations_applied", applied)
	return database, nil
}

// startIngest tails the spool in the background until ctx is cancelled.
func startIngest(ctx context.Context, cfg config.Config, database *sql.DB, logger *slog.Logger, metrics *telemetry.Metrics) error {
	if cfg.SpoolDir == "" {
		logger.Info("ingest is off: PADDOCK_SPOOL_DIR is not set")
		return nil
	}
	service, err := ingest.New(ingest.Config{
		SpoolDir: cfg.SpoolDir, DataDir: cfg.DataDir,
		PollInterval: spoolPollInterval, SettleTime: spoolSettleTime,
	}, ingest.NewPostgresStore(database), logger, metrics.Provider.Meter("paddock/ingest"))
	if err != nil {
		return fmt.Errorf("set up ingest: %w", err)
	}
	logger.Info("ingest is watching the spool", "spool_dir", cfg.SpoolDir, "data_dir", cfg.DataDir)
	go func() {
		if err := service.Run(ctx); err != nil {
			logger.Error("ingest stopped", "error", err)
		}
	}()
	return nil
}

// startFeatures runs the engine jobs in the background until ctx is cancelled.
func startFeatures(ctx context.Context, cfg config.Config, database *sql.DB, analysis *engine.Client, logger *slog.Logger, metrics *telemetry.Metrics) error {
	if cfg.EngineURL == "" {
		logger.Info("pass features are off: PADDOCK_ENGINE_URL is not set")
		return nil
	}
	worker, err := features.NewWorker(database, analysis, logger, metrics.Provider.Meter("paddock/jobs"), jobPollInterval)
	if err != nil {
		return fmt.Errorf("set up the features worker: %w", err)
	}
	logger.Info("features worker is running", "engine_url", cfg.EngineURL)
	go func() {
		if err := worker.Run(ctx); err != nil {
			logger.Error("features worker stopped", "error", err)
		}
	}()
	return nil
}

// serve runs the server until ctx is cancelled, then lets in-flight requests finish.
func serve(ctx context.Context, server *http.Server, logger *slog.Logger) error {
	failed := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", server.Addr, "version", version)
		failed <- server.ListenAndServe()
	}()

	select {
	case err := <-failed:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shut down http server: %w", err)
	}
	return nil
}
