package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Diagnoses runs and reads the diagnosis of a session. *diagnosis.Service implements it.
type Diagnoses interface {
	Run(ctx context.Context, sessionID int64) (contracts.Diagnosis, error)
	Latest(ctx context.Context, sessionID int64) (contracts.Diagnosis, error)
}

type diagnose func(ctx context.Context, sessionID int64) (contracts.Diagnosis, error)

// registerDiagnoses adds the diagnosis routes of docs/api/openapi.yaml to the mux.
func registerDiagnoses(mux *http.ServeMux, logger *slog.Logger, service Diagnoses) {
	mux.HandleFunc("POST /sessions/{id}/diagnosis", diagnosisHandler(logger, service.Run, http.StatusCreated))
	mux.HandleFunc("GET /sessions/{id}/diagnosis", diagnosisHandler(logger, service.Latest, http.StatusOK))
}

func diagnosisHandler(logger *slog.Logger, action diagnose, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		diagnosis, err := action(r.Context(), id)
		if err != nil {
			fail(w, r, logger, err)
			return
		}
		writeJSON(w, r, logger, status, diagnosis)
	}
}
