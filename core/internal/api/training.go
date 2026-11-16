package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Training starts, lists and stops AI training programs. *training.Service implements it.
type Training interface {
	Start(ctx context.Context, sessionID int64, request contracts.TrainingCreate) (contracts.TrainingProgram, error)
	List(ctx context.Context) ([]contracts.TrainingProgram, error)
	Stop(ctx context.Context, id int64) (contracts.TrainingProgram, error)
}

// registerTraining adds the training routes of docs/api/openapi.yaml to the mux.
func registerTraining(mux *http.ServeMux, logger *slog.Logger, service Training) {
	routes := experimentRoutes{logger: logger}
	mux.HandleFunc("POST /sessions/{id}/training", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		var request contracts.TrainingCreate
		if !routes.readBody(w, r, &request) {
			return
		}
		program, err := service.Start(r.Context(), id, request)
		if err != nil {
			fail(w, r, logger, err)
			return
		}
		writeJSON(w, r, logger, http.StatusCreated, program)
	})
	mux.HandleFunc("GET /training", func(w http.ResponseWriter, r *http.Request) {
		answer(w, r, logger)(service.List(r.Context()))
	})
	mux.HandleFunc("POST /training/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such training program"})
			return
		}
		answer(w, r, logger)(service.Stop(r.Context(), id))
	})
}
