package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// BaseSetups builds a base setup from what a car measured. *sensitivity.BaseSetups
// implements it.
type BaseSetups interface {
	Build(ctx context.Context, sessionID int64, driver contracts.BaseSetupCreateDriver) (contracts.BaseSetupReply, error)
}

// registerBaseSetups adds POST /sessions/{id}/base-setup of docs/api/openapi.yaml to the mux.
// A reply without an experiment is 409: nothing the car measured qualifies, and the body
// says what it did measure.
func registerBaseSetups(mux *http.ServeMux, logger *slog.Logger, service BaseSetups) {
	mux.HandleFunc("POST /sessions/{id}/base-setup", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeJSON(w, r, logger, http.StatusNotFound, problem{Error: "no such session"})
			return
		}
		var request contracts.BaseSetupCreate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&request); err != nil {
			message := fmt.Sprintf("request body is not valid: %v", err)
			writeJSON(w, r, logger, http.StatusUnprocessableEntity, problem{Error: message})
			return
		}
		reply, err := service.Build(r.Context(), id, request.Driver)
		if err != nil {
			fail(w, r, logger, err)
			return
		}
		status := http.StatusCreated
		if reply.ExperimentId == nil {
			status = http.StatusConflict
		}
		writeJSON(w, r, logger, status, reply)
	})
}
