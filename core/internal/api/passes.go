package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/passes"
)

// Passes reads what a session recorded. *passes.Service implements it.
type Passes interface {
	Passes(ctx context.Context, sessionID int64) (contracts.SessionPasses, error)
	Trace(ctx context.Context, sessionID int64, seq, maxPoints int) (contracts.PassTrace, error)
}

// registerPasses adds the read-only pass and sample routes of docs/api/openapi.yaml.
func registerPasses(mux *http.ServeMux, logger *slog.Logger, service Passes) {
	mux.HandleFunc("GET /sessions/{id}/passes", func(w http.ResponseWriter, r *http.Request) {
		id, ok := positive(r.PathValue("id"))
		if !ok {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		answer(w, r, logger)(service.Passes(r.Context(), int64(id)))
	})
	mux.HandleFunc("GET /sessions/{id}/passes/{seq}/samples", func(w http.ResponseWriter, r *http.Request) {
		id, idOK := positive(r.PathValue("id"))
		seq, seqOK := positive(r.PathValue("seq"))
		if !idOK || !seqOK {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such pass"})
			return
		}
		points, ok := maxPoints(r.URL.Query().Get("max_points"))
		if !ok {
			writeJSON(w, r, logger, http.StatusUnprocessableEntity, problem{Error: "max_points must be a whole number from 50 to 5000"})
			return
		}
		answer(w, r, logger)(service.Trace(r.Context(), int64(id), seq, points))
	})
}

// positive parses a path id: a whole number from 1.
func positive(text string) (int, bool) {
	value, err := strconv.Atoi(text)
	return value, err == nil && value >= 1
}

// maxPoints parses the max_points query; empty means the default.
func maxPoints(text string) (int, bool) {
	if text == "" {
		return passes.DefaultPoints, true
	}
	value, err := strconv.Atoi(text)
	return value, err == nil && value >= passes.MinPoints && value <= passes.MaxPoints
}
