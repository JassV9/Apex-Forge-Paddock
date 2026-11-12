package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

type fakeDiagnoses struct {
	err   error
	calls []string
}

func (f *fakeDiagnoses) answer(call string, sessionID int64) (contracts.Diagnosis, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s %d", call, sessionID))
	return contracts.Diagnosis{
		Evidence: []contracts.Evidence{}, Hypotheses: []contracts.Hypothesis{},
		Explanation: contracts.Explanation{
			Provider: contracts.ExplanationProviderNone, Summary: "No symptom.",
			Ranking: []contracts.RankedHypothesis{},
		},
	}, f.err
}

func (f *fakeDiagnoses) Run(_ context.Context, sessionID int64) (contracts.Diagnosis, error) {
	return f.answer("run", sessionID)
}

func (f *fakeDiagnoses) Latest(_ context.Context, sessionID int64) (contracts.Diagnosis, error) {
	return f.answer("latest", sessionID)
}

func TestDiagnosisRoutes(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		err      error
		wantCode int
		wantCall string
	}{
		{name: "diagnose now", method: http.MethodPost, path: "/sessions/4/diagnosis", wantCode: http.StatusCreated, wantCall: "run 4"},
		{name: "read the latest", method: http.MethodGet, path: "/sessions/4/diagnosis", wantCode: http.StatusOK, wantCall: "latest 4"},
		{name: "none stored", method: http.MethodGet, path: "/sessions/4/diagnosis", err: experiments.ErrNotFound, wantCode: http.StatusNotFound, wantCall: "latest 4"},
		{name: "features not ready", method: http.MethodPost, path: "/sessions/4/diagnosis", err: experiments.ErrConflict, wantCode: http.StatusConflict, wantCall: "run 4"},
		{name: "engine down", method: http.MethodPost, path: "/sessions/4/diagnosis", err: engine.ErrUnavailable, wantCode: http.StatusBadGateway, wantCall: "run 4"},
		{name: "not a session id", method: http.MethodGet, path: "/sessions/abc/diagnosis", wantCode: http.StatusNotFound},
		{name: "no session zero", method: http.MethodPost, path: "/sessions/0/diagnosis", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnoses := &fakeDiagnoses{err: tt.err}
			deps := testDeps()
			deps.Diagnoses = diagnoses

			recorder := get(t, NewHandler(deps), tt.method, tt.path)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d (%s)", recorder.Code, tt.wantCode, recorder.Body)
			}
			if strings.Join(diagnoses.calls, ",") != tt.wantCall {
				t.Errorf("calls = %v, want %q", diagnoses.calls, tt.wantCall)
			}
			if tt.wantCode >= http.StatusBadRequest {
				return
			}
			var got contracts.Diagnosis
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("body is not a Diagnosis: %v", err)
			}
			if strings.Contains(recorder.Body.String(), "null") {
				t.Errorf("empty lists must be [], not null: %s", recorder.Body)
			}
		})
	}
}

func TestDiagnosisRoutesAreLeftOutWithoutAService(t *testing.T) {
	recorder := get(t, NewHandler(testDeps()), http.MethodGet, "/sessions/4/diagnosis")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want 404", recorder.Code)
	}
}
