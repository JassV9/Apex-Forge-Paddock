// Package api serves core's REST API. The API is specified first in docs/api/openapi.yaml;
// response bodies are the generated types in internal/contracts.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Pinger reports whether a dependency is reachable. *sql.DB implements it.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// SessionLister lists recorded sessions, newest first.
type SessionLister interface {
	ListSessions(ctx context.Context) ([]contracts.SessionSummary, error)
}

// Deps is everything the handlers need from the rest of core.
type Deps struct {
	Logger   *slog.Logger
	Version  string
	Database Pinger
	Sessions SessionLister
	// Experiments serves the /experiments routes; nil leaves them out.
	Experiments Experiments
	// Metrics serves GET /metrics for Prometheus; nil leaves the route out.
	Metrics http.Handler
}

// NewHandler returns the HTTP handler for every core route.
func NewHandler(deps Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", healthHandler(deps.Logger, deps.Version, deps.Database))
	mux.Handle("GET /sessions", sessionsHandler(deps.Logger, deps.Sessions))
	if deps.Experiments != nil {
		registerExperiments(mux, deps.Logger, deps.Experiments)
	}
	if deps.Metrics != nil {
		mux.Handle("GET /metrics", deps.Metrics)
	}
	return mux
}

func writeJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.ErrorContext(r.Context(), "write response", "path", r.URL.Path, "error", err)
	}
}

// problem is the body of every error response.
type problem struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, message string, err error) {
	logger.ErrorContext(r.Context(), message, "path", r.URL.Path, "error", err)
	writeJSON(w, r, logger, status, problem{Error: message})
}
