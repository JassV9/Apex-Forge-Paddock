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

type fakeTraining struct {
	err   error
	calls []string
}

func program() contracts.TrainingProgram {
	return contracts.TrainingProgram{Id: 2, SessionId: 57, CarName: "Ferrari 488 GT3", TrackName: "COTA",
		Target: 3, State: contracts.TrainingProgramStateRunning, Steps: []contracts.TrainingStep{}}
}

func (f *fakeTraining) Start(_ context.Context, sessionID int64, request contracts.TrainingCreate) (contracts.TrainingProgram, error) {
	f.calls = append(f.calls, fmt.Sprintf("start %d %d", sessionID, request.Experiments))
	return program(), f.err
}

func (f *fakeTraining) List(context.Context) ([]contracts.TrainingProgram, error) {
	f.calls = append(f.calls, "list")
	return []contracts.TrainingProgram{program()}, f.err
}

func (f *fakeTraining) Stop(_ context.Context, id int64) (contracts.TrainingProgram, error) {
	f.calls = append(f.calls, fmt.Sprintf("stop %d", id))
	return program(), f.err
}

func TestTrainingRoutes(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		err                      error
		wantCode                 int
		wantCall                 string
	}{
		{"start a program", http.MethodPost, "/sessions/57/training", `{"experiments": 3}`, nil, http.StatusCreated, "start 57 3"},
		{"too many experiments", http.MethodPost, "/sessions/57/training", `{"experiments": 50}`, nil, http.StatusUnprocessableEntity, ""},
		{"no body", http.MethodPost, "/sessions/57/training", `{}`, nil, http.StatusUnprocessableEntity, ""},
		{"a human's experiment is on the car", http.MethodPost, "/sessions/57/training", `{"experiments": 1}`, experiments.ErrConflict, http.StatusConflict, "start 57 1"},
		{"not a session", http.MethodPost, "/sessions/x/training", `{"experiments": 1}`, nil, http.StatusNotFound, ""},
		{"list", http.MethodGet, "/training", "", nil, http.StatusOK, "list"},
		{"stop", http.MethodPost, "/training/2/stop", "", nil, http.StatusOK, "stop 2"},
		{"stop an ended program", http.MethodPost, "/training/2/stop", "", experiments.ErrConflict, http.StatusConflict, "stop 2"},
		{"no program zero", http.MethodPost, "/training/0/stop", "", nil, http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeTraining{err: tt.err}
			deps := testDeps()
			deps.Training = service
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

func TestTrainingRoutesAreLeftOutWithoutAService(t *testing.T) {
	if recorder := get(t, NewHandler(testDeps()), http.MethodGet, "/training"); recorder.Code != http.StatusNotFound {
		t.Errorf("GET /training = %d, want 404", recorder.Code)
	}
}
