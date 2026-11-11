// Package api serves core's REST API. The API is specified first in docs/api/openapi.yaml;
// response bodies are the generated types in internal/contracts.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Pinger reports whether a dependency is reachable. *sql.DB implements it.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// NewHandler returns the HTTP handler for every core route.
func NewHandler(logger *slog.Logger, version string, database Pinger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", healthHandler(logger, version, database))
	return mux
}

func writeJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.ErrorContext(r.Context(), "write response", "path", r.URL.Path, "error", err)
	}
}
