package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Sweeps starts, lists and stops sensitivity discovery sweeps. *discovery.Service implements it.
type Sweeps interface {
	Start(ctx context.Context, sessionID int64, request contracts.SweepCreate) (contracts.Sweep, error)
	List(ctx context.Context) ([]contracts.Sweep, error)
	Stop(ctx context.Context, id int64) (contracts.Sweep, error)
}

// registerSweeps adds the sweep routes of docs/api/openapi.yaml to the mux.
func registerSweeps(mux *http.ServeMux, logger *slog.Logger, service Sweeps) {
	routes := experimentRoutes{logger: logger}
	mux.HandleFunc("POST /sessions/{id}/sweeps", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		var request contracts.SweepCreate
		if !routes.readBody(w, r, &request) {
			return
		}
		sweep, err := service.Start(r.Context(), id, request)
		if err != nil {
			fail(w, r, logger, err)
			return
		}
		writeJSON(w, r, logger, http.StatusCreated, sweep)
	})
	mux.HandleFunc("GET /sweeps", func(w http.ResponseWriter, r *http.Request) {
		answer(w, r, logger)(service.List(r.Context()))
	})
	mux.HandleFunc("POST /sweeps/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such sweep"})
			return
		}
		answer(w, r, logger)(service.Stop(r.Context(), id))
	})
}
