package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Experiments is every action on experiments. *experiments.Service implements it.
type Experiments interface {
	List(ctx context.Context) ([]contracts.Experiment, error)
	Get(ctx context.Context, id int64) (contracts.Experiment, error)
	Create(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error)
	Decide(ctx context.Context, id int64, accept bool) (contracts.Experiment, error)
	Plan(ctx context.Context, id int64) (contracts.Experiment, error)
	Start(ctx context.Context, id int64) (contracts.Experiment, error)
	Evaluate(ctx context.Context, id int64) (contracts.Experiment, error)
	Extend(ctx context.Context, id int64) (contracts.Experiment, error)
	Revert(ctx context.Context, id int64) (contracts.Experiment, error)
	Abort(ctx context.Context, id int64) (contracts.Experiment, error)
	Discard(ctx context.Context, id int64) (contracts.Experiment, error)
	SetupItems(ctx context.Context, sessionID int64) ([]contracts.SetupItem, error)
}

const maxRequestBody = 1 << 20

type experimentAction func(ctx context.Context, id int64) (contracts.Experiment, error)

type experimentRoutes struct {
	logger      *slog.Logger
	experiments Experiments
	actions     map[string]experimentAction
}

// registerExperiments adds the experiment routes of docs/api/openapi.yaml to the mux.
func registerExperiments(mux *http.ServeMux, logger *slog.Logger, service Experiments) {
	routes := experimentRoutes{logger: logger, experiments: service, actions: map[string]experimentAction{
		"plan": service.Plan, "start": service.Start, "evaluate": service.Evaluate,
		"extend": service.Extend, "revert": service.Revert, "abort": service.Abort,
		"discard": service.Discard,
	}}
	mux.HandleFunc("GET /sessions/{id}/setup", routes.setupItems)
	mux.HandleFunc("GET /experiments", routes.list)
	mux.HandleFunc("POST /experiments", routes.create)
	mux.HandleFunc("GET /experiments/{id}", routes.get)
	mux.HandleFunc("POST /experiments/{id}/decision", routes.decide)
	mux.HandleFunc("POST /experiments/{id}/{action}", routes.act)
}

func (e experimentRoutes) list(w http.ResponseWriter, r *http.Request) {
	list, err := e.experiments.List(r.Context())
	if err != nil {
		e.fail(w, r, err)
		return
	}
	if list == nil {
		list = []contracts.Experiment{} // an empty list is [], never null
	}
	writeJSON(w, r, e.logger, http.StatusOK, list)
}

func (e experimentRoutes) setupItems(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, r, e.logger, http.StatusUnprocessableEntity, problem{Error: "no such session"})
		return
	}
	items, err := e.experiments.SetupItems(r.Context(), id)
	if err != nil {
		e.fail(w, r, err)
		return
	}
	writeJSON(w, r, e.logger, http.StatusOK, items)
}

func (e experimentRoutes) create(w http.ResponseWriter, r *http.Request) {
	var request contracts.ExperimentCreate
	if !e.readBody(w, r, &request) {
		return
	}
	e.reply(w, r, http.StatusCreated)(e.experiments.Create(r.Context(), request))
}

func (e experimentRoutes) get(w http.ResponseWriter, r *http.Request) {
	if id, ok := e.id(w, r); ok {
		e.reply(w, r, http.StatusOK)(e.experiments.Get(r.Context(), id))
	}
}

func (e experimentRoutes) decide(w http.ResponseWriter, r *http.Request) {
	id, ok := e.id(w, r)
	if !ok {
		return
	}
	var decision contracts.ExperimentDecision
	if !e.readBody(w, r, &decision) {
		return
	}
	e.reply(w, r, http.StatusOK)(e.experiments.Decide(r.Context(), id, decision.Accept))
}

func (e experimentRoutes) act(w http.ResponseWriter, r *http.Request) {
	id, ok := e.id(w, r)
	if !ok {
		return
	}
	action, known := e.actions[r.PathValue("action")]
	if !known {
		writeJSON(w, r, e.logger, http.StatusNotFound, problem{Error: "unknown action " + r.PathValue("action")})
		return
	}
	e.reply(w, r, http.StatusOK)(action(r.Context(), id))
}

// id reads the experiment id from the path; a path that has none is answered with 404.
func (e experimentRoutes) id(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, r, e.logger, http.StatusNotFound, problem{Error: "no such experiment"})
		return 0, false
	}
	return id, true
}

// readBody decodes a contract type; the generated decoder checks required fields and enums.
func (e experimentRoutes) readBody(w http.ResponseWriter, r *http.Request, into any) bool {
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := json.NewDecoder(body).Decode(into); err != nil {
		message := fmt.Sprintf("request body is not valid: %v", err)
		writeJSON(w, r, e.logger, http.StatusUnprocessableEntity, problem{Error: message})
		return false
	}
	return true
}

// reply returns a function that writes a service result: the experiment, or the problem.
func (e experimentRoutes) reply(w http.ResponseWriter, r *http.Request, status int) func(contracts.Experiment, error) {
	return func(experiment contracts.Experiment, err error) {
		if err != nil {
			e.fail(w, r, err)
			return
		}
		writeJSON(w, r, e.logger, status, experiment)
	}
}

// fail maps a service error to a status. What the caller got wrong is said in the reply;
// what broke inside core is logged and not shown.
func (e experimentRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, experiments.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, experiments.ErrInvalid):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, experiments.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, engine.ErrUnavailable), errors.Is(err, engine.ErrRejected):
		writeError(w, r, e.logger, http.StatusBadGateway, "the analysis engine did not answer", err)
		return
	}
	if status == http.StatusInternalServerError {
		writeError(w, r, e.logger, status, "could not complete the request", err)
		return
	}
	e.logger.InfoContext(r.Context(), "request refused", "path", r.URL.Path, "status", status, "reason", err.Error())
	writeJSON(w, r, e.logger, status, problem{Error: err.Error()})
}
