package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// fakeExperiments answers every call with the same experiment or error and notes the call.
type fakeExperiments struct {
	err   error
	calls []string
}

func (f *fakeExperiments) answer(call string, id int64) (contracts.Experiment, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s %d", call, id))
	return contracts.Experiment{Id: int(id), State: contracts.ExperimentStateProposed}, f.err
}

func (f *fakeExperiments) List(context.Context) ([]contracts.Experiment, error) {
	return nil, f.err
}

func (f *fakeExperiments) SetupItems(_ context.Context, sessionID int64) ([]contracts.SetupItem, error) {
	f.calls = append(f.calls, fmt.Sprintf("setup items %d", sessionID))
	return []contracts.SetupItem{{Name: "ARB_FRONT", Label: "Front ARB", Max: 4, Step: 1, Value: 3}}, f.err
}

func (f *fakeExperiments) Create(_ context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	return f.answer("create "+request.Parameter, 7)
}

func (f *fakeExperiments) Decide(_ context.Context, id int64, accept bool) (contracts.Experiment, error) {
	return f.answer(fmt.Sprintf("decide %v", accept), id)
}

func (f *fakeExperiments) Get(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("get", id)
}

func (f *fakeExperiments) Plan(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("plan", id)
}

func (f *fakeExperiments) Start(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("start", id)
}

func (f *fakeExperiments) Evaluate(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("evaluate", id)
}

func (f *fakeExperiments) Extend(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("extend", id)
}

func (f *fakeExperiments) Revert(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("revert", id)
}

func (f *fakeExperiments) Abort(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("abort", id)
}

func (f *fakeExperiments) Discard(_ context.Context, id int64) (contracts.Experiment, error) {
	return f.answer("discard", id)
}

func send(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	handler.ServeHTTP(recorder, request)
	return recorder
}

const validCreate = `{"session_id": 1, "parameter": "ARB_FRONT", "clicks": -1,
	"statement": "Softer front bar.", "predictions": []}`

func TestExperimentRoutesCallTheService(t *testing.T) {
	tests := []struct {
		method, path, body string
		wantCode           int
		wantCall           string
	}{
		{http.MethodPost, "/experiments", validCreate, http.StatusCreated, "create ARB_FRONT 7"},
		{http.MethodGet, "/experiments/3", "", http.StatusOK, "get 3"},
		{http.MethodPost, "/experiments/3/plan", "", http.StatusOK, "plan 3"},
		{http.MethodPost, "/experiments/3/start", "", http.StatusOK, "start 3"},
		{http.MethodPost, "/experiments/3/evaluate", "", http.StatusOK, "evaluate 3"},
		{http.MethodPost, "/experiments/3/extend", "", http.StatusOK, "extend 3"},
		{http.MethodPost, "/experiments/3/revert", "", http.StatusOK, "revert 3"},
		{http.MethodPost, "/experiments/3/abort", "", http.StatusOK, "abort 3"},
		{http.MethodPost, "/experiments/3/discard", "", http.StatusOK, "discard 3"},
		{http.MethodPost, "/experiments/3/decision", `{"accept": true}`, http.StatusOK, "decide true 3"},
		{http.MethodPost, "/experiments/3/decision", `{"accept": false}`, http.StatusOK, "decide false 3"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			service := &fakeExperiments{}
			deps := testDeps()
			deps.Experiments = service

			recorder := send(t, NewHandler(deps), tt.method, tt.path, tt.body)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d: %s", recorder.Code, tt.wantCode, recorder.Body)
			}
			if len(service.calls) != 1 || service.calls[0] != tt.wantCall {
				t.Errorf("service calls = %v, want [%s]", service.calls, tt.wantCall)
			}
			if !strings.Contains(recorder.Body.String(), `"state":"proposed"`) {
				t.Errorf("body = %s", recorder.Body)
			}
		})
	}
}

func TestExperimentRequestsThatNeverReachTheService(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		wantCode                 int
	}{
		{"missing field", http.MethodPost, "/experiments", `{"session_id": 1}`, http.StatusUnprocessableEntity},
		{"not JSON", http.MethodPost, "/experiments", `clicks=1`, http.StatusUnprocessableEntity},
		{"decision without a choice", http.MethodPost, "/experiments/3/decision", `{}`, http.StatusUnprocessableEntity},
		{"unknown action", http.MethodPost, "/experiments/3/launch", "", http.StatusNotFound},
		{"id is not a number", http.MethodGet, "/experiments/abc", "", http.StatusNotFound},
		{"id below 1", http.MethodPost, "/experiments/0/plan", "", http.StatusNotFound},
		{"actions are not readable", http.MethodGet, "/experiments/3/plan", "", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeExperiments{}
			deps := testDeps()
			deps.Experiments = service

			recorder := send(t, NewHandler(deps), tt.method, tt.path, tt.body)

			if recorder.Code != tt.wantCode || len(service.calls) != 0 {
				t.Errorf("status code = %d (want %d), service calls %v", recorder.Code, tt.wantCode, service.calls)
			}
		})
	}
}

func TestExperimentErrorsBecomeStatusCodes(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  int
		wantShown bool // whether the reply says what went wrong
	}{
		{"not found", fmt.Errorf("experiment 3: %w", experiments.ErrNotFound), http.StatusNotFound, true},
		{"invalid", fmt.Errorf("ARB_FRONT = 5: %w", experiments.ErrInvalid), http.StatusUnprocessableEntity, true},
		{"wrong state", fmt.Errorf("experiment 3 is planned: %w", experiments.ErrConflict), http.StatusConflict, true},
		{"engine down", fmt.Errorf("dial tcp 10.0.0.5: %w", engine.ErrUnavailable), http.StatusBadGateway, false},
		{"engine refused", fmt.Errorf("422: %w", engine.ErrRejected), http.StatusBadGateway, false},
		{"database", errors.New("pq: password authentication failed"), http.StatusInternalServerError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps()
			deps.Experiments = &fakeExperiments{err: tt.err}

			recorder := send(t, NewHandler(deps), http.MethodPost, "/experiments/3/plan", "")

			var body problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("body is not a problem: %s", recorder.Body)
			}
			if recorder.Code != tt.wantCode || (body.Error == tt.err.Error()) != tt.wantShown {
				t.Errorf("status %d (want %d), error %q", recorder.Code, tt.wantCode, body.Error)
			}
		})
	}
}

func TestSetupItemsOfASession(t *testing.T) {
	service := &fakeExperiments{}
	deps := testDeps()
	deps.Experiments = service

	recorder := send(t, NewHandler(deps), http.MethodGet, "/sessions/4/setup", "")

	var items []contracts.SetupItem
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatalf("body is not a list of setup items: %s", recorder.Body)
	}
	if recorder.Code != http.StatusOK || len(items) != 1 || items[0].Name != "ARB_FRONT" {
		t.Errorf("status %d, items %+v", recorder.Code, items)
	}
	if len(service.calls) != 1 || service.calls[0] != "setup items 4" {
		t.Errorf("service calls = %v", service.calls)
	}
	if code := send(t, NewHandler(deps), http.MethodGet, "/sessions/abc/setup", "").Code; code != http.StatusUnprocessableEntity {
		t.Errorf("session id that is not a number: status %d", code)
	}
}

func TestLedgerIsAnEmptyListWhenNothingWasProposed(t *testing.T) {
	deps := testDeps()
	deps.Experiments = &fakeExperiments{}

	recorder := send(t, NewHandler(deps), http.MethodGet, "/experiments", "")

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Errorf("GET /experiments = %d %s", recorder.Code, recorder.Body)
	}
}
