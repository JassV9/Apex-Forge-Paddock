// Command paddock is the core process: REST API, ingest, experiments and setups in one binary.
package main

import (
	"context"
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
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
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

	database, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()
	applied, err := store.MigrateUp(ctx, database)
	if err != nil {
		return err
	}
	logger.Info("database ready", "migrations_applied", applied)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewHandler(logger, version, database),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	return serve(ctx, server, logger)
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
