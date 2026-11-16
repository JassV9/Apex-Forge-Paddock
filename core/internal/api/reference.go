package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Reference compares a driver with AC's AI on the same car and setup. *reference.Service
// implements it.
type Reference interface {
	Session(ctx context.Context, sessionID int64, setupHash string) (contracts.SessionReference, error)
	ForSetup(ctx context.Context, car, track, layout, setupHash string) (contracts.AiReference, error)
}

// registerReference adds the read-only AI reference routes of docs/api/openapi.yaml.
func registerReference(mux *http.ServeMux, logger *slog.Logger, service Reference) {
	mux.HandleFunc("GET /sessions/{id}/reference", func(w http.ResponseWriter, r *http.Request) {
		id, ok := positive(r.PathValue("id"))
		if !ok {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		setupHash := r.URL.Query().Get("setup_hash")
		answer(w, r, logger)(service.Session(r.Context(), int64(id), setupHash))
	})
	mux.HandleFunc("GET /reference", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		car, track, setupHash := query.Get("car"), query.Get("track"), query.Get("setup_hash")
		if car == "" || track == "" || setupHash == "" {
			writeJSON(w, r, logger, http.StatusUnprocessableEntity,
				problem{Error: "car, track and setup_hash are required"})
			return
		}
		answer(w, r, logger)(service.ForSetup(r.Context(), car, track, query.Get("layout"), setupHash))
	})
}
