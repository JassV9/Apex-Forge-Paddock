// Command paddock-launcher runs on the Windows host (not in Docker) and starts Assetto Corsa
// in Practice from the pits through Content Manager when the dashboard asks, guarding the
// wheel bindings in controls.ini (docs/adr/0017-host-launcher.md).
//
//	go run ./cmd/paddock-launcher [--ac-root DIR] [--documents DIR] [--template NAME|FILE]
//
// It listens on 127.0.0.1 only.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/launcher"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("paddock-launcher stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	settings, err := parseSettings(args, os.Getenv)
	if err != nil {
		return err
	}
	if err := settings.check(logger); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	service := launcher.NewService(launcher.Config{
		Paths: settings.Paths, Version: version, Logger: logger, Timing: launcher.DefaultTiming,
		Weather:   settings.Weather,
		Processes: launcher.SystemProcesses{}, Opener: launcher.SystemOpener{},
	})
	if err := service.Start(ctx); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(settings.Port)),
		Handler:           launcher.NewHandler(service, logger),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	err = serve(ctx, server, logger, settings)
	service.Wait()
	return err
}

func serve(ctx context.Context, server *http.Server, logger *slog.Logger, settings settings) error {
	failed := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", server.Addr, "version", version,
			"ac_root", settings.Paths.ACRoot, "documents", settings.Paths.Documents,
			"template", settings.Paths.Template)
		failed <- server.ListenAndServe()
	}()
	select {
	case err := <-failed:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}
	logger.Info("shutting down; an unfinished bindings check resumes at the next start")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shut down http server: %w", err)
	}
	return nil
}
