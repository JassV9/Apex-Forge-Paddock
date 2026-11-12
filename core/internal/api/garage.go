package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Garage is what the ledger has taught about the cars. *sensitivity.Service implements it.
type Garage interface {
	Profiles(ctx context.Context) ([]contracts.SensitivityProfile, error)
	Proposals(ctx context.Context, sessionID int64) (contracts.Proposals, error)
	Scorecard(ctx context.Context) (contracts.Scorecard, error)
}

// registerGarage adds the sensitivity, proposal and scorecard routes of
// docs/api/openapi.yaml to the mux.
func registerGarage(mux *http.ServeMux, logger *slog.Logger, service Garage) {
	mux.HandleFunc("GET /sensitivity", func(w http.ResponseWriter, r *http.Request) {
		answer(w, r, logger)(service.Profiles(r.Context()))
	})
	mux.HandleFunc("GET /scorecard", func(w http.ResponseWriter, r *http.Request) {
		answer(w, r, logger)(service.Scorecard(r.Context()))
	})
	mux.HandleFunc("GET /sessions/{id}/proposals", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		answer(w, r, logger)(service.Proposals(r.Context(), id))
	})
}

// answer writes what a service returned: the body with 200, or the error as the experiment
// routes would answer it.
func answer(w http.ResponseWriter, r *http.Request, logger *slog.Logger) func(body any, err error) {
	return func(body any, err error) {
		if err != nil {
			fail(w, r, logger, err)
			return
		}
		writeJSON(w, r, logger, http.StatusOK, body)
	}
}
