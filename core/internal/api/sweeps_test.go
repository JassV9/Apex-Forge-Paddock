package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

type fakeSweeps struct {
	err   error
	calls []string
}

func sweep() contracts.Sweep {
	return contracts.Sweep{Id: 4, SessionId: 57, CarName: "Formula Hybrid 2026", TrackName: "Hungaroring",
		Parameter: "ARB_REAR", ParameterLabel: "Rear ARB", BaselineVersionId: 9, AiHintShare: 0.93,
		Requested: []int{-1, 1}, Planned: []int{-1, 1}, Skipped: []string{},
		State: contracts.SweepStateRunning, Steps: []contracts.SweepStep{}}
}

func (f *fakeSweeps) Start(_ context.Context, sessionID int64, request contracts.SweepCreate) (contracts.Sweep, error) {
	f.calls = append(f.calls, fmt.Sprintf("start %d %s %v %v", sessionID, request.Parameter, request.Steps, request.AiHintShare))
	return sweep(), f.err
}

func (f *fakeSweeps) List(context.Context) ([]contracts.Sweep, error) {
	f.calls = append(f.calls, "list")
	return []contracts.Sweep{sweep()}, f.err
}

func (f *fakeSweeps) Stop(_ context.Context, id int64) (contracts.Sweep, error) {
	f.calls = append(f.calls, fmt.Sprintf("stop %d", id))
	return sweep(), f.err
}

func TestSweepRoutes(t *testing.T) {
	const body = `{"parameter": "ARB_REAR", "steps": [-1, 1, 2, 3], "ai_hint_share": 0.93}`
	const started = "start 57 ARB_REAR [-1 1 2 3] 0.93"
	tests := []struct {
		name, method, path, body string
		err                      error
		wantCode                 int
		wantCall                 string
	}{
		{"start a sweep", http.MethodPost, "/sessions/57/sweeps", body, nil, http.StatusCreated, started},
		{"no pinned hint", http.MethodPost, "/sessions/57/sweeps", `{"parameter": "ARB_REAR", "steps": [1]}`, nil,
			http.StatusUnprocessableEntity, ""},
		{"no steps", http.MethodPost, "/sessions/57/sweeps",
			`{"parameter": "ARB_REAR", "steps": [], "ai_hint_share": 0.93}`, nil, http.StatusUnprocessableEntity, ""},
		{"a hint out of range", http.MethodPost, "/sessions/57/sweeps",
			`{"parameter": "ARB_REAR", "steps": [1], "ai_hint_share": 2}`, nil, http.StatusUnprocessableEntity, ""},
		{"the car is busy", http.MethodPost, "/sessions/57/sweeps", body, experiments.ErrConflict,
			http.StatusConflict, started},
		{"not a session", http.MethodPost, "/sessions/x/sweeps", body, nil, http.StatusNotFound, ""},
		{"list", http.MethodGet, "/sweeps", "", nil, http.StatusOK, "list"},
		{"stop", http.MethodPost, "/sweeps/4/stop", "", nil, http.StatusOK, "stop 4"},
		{"stop an ended sweep", http.MethodPost, "/sweeps/4/stop", "", experiments.ErrConflict,
			http.StatusConflict, "stop 4"},
		{"no sweep zero", http.MethodPost, "/sweeps/0/stop", "", nil, http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeSweeps{err: tt.err}
			deps := testDeps()
			deps.Sweeps = service
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))

			NewHandler(deps).ServeHTTP(recorder, request)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d (%s)", recorder.Code, tt.wantCode, recorder.Body)
			}
			if got := strings.Join(service.calls, ","); got != tt.wantCall {
				t.Errorf("calls = %q, want %q", got, tt.wantCall)
			}
		})
	}
}

func TestSweepRoutesAreLeftOutWithoutAService(t *testing.T) {
	if recorder := get(t, NewHandler(testDeps()), http.MethodGet, "/sweeps"); recorder.Code != http.StatusNotFound {
		t.Errorf("GET /sweeps = %d, want 404", recorder.Code)
	}
}
