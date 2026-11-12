package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

type fakeGarage struct {
	err   error
	calls []string
}

func (f *fakeGarage) Profiles(context.Context) ([]contracts.SensitivityProfile, error) {
	f.calls = append(f.calls, "profiles")
	return []contracts.SensitivityProfile{{
		CarId: 1, CarName: "Mazda MX-5 Cup", ModelVersion: "m-1",
		Labels: contracts.SensitivityProfileLabels{}, Effects: []contracts.SensitivityEffect{},
	}}, f.err
}

func (f *fakeGarage) Proposals(_ context.Context, sessionID int64) (contracts.Proposals, error) {
	f.calls = append(f.calls, fmt.Sprintf("proposals %d", sessionID))
	return contracts.Proposals{
		Stage: contracts.ProposalsStageColdStart, Ranked: []contracts.Proposal{},
		Skipped: []contracts.SkippedCandidate{},
	}, f.err
}

func (f *fakeGarage) Scorecard(context.Context) (contracts.Scorecard, error) {
	f.calls = append(f.calls, "scorecard")
	return contracts.Scorecard{BySource: []contracts.SourceScore{}, ByState: []contracts.StateCount{}}, f.err
}

func TestGarageRoutes(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		err      error
		wantCode int
		wantCall string
		wantBody string
	}{
		{name: "sensitivity profiles", path: "/sensitivity", wantCode: http.StatusOK, wantCall: "profiles", wantBody: `"car_name":"Mazda MX-5 Cup"`},
		{name: "scorecard", path: "/scorecard", wantCode: http.StatusOK, wantCall: "scorecard", wantBody: `"by_source":[]`},
		{name: "what to test next", path: "/sessions/4/proposals", wantCode: http.StatusOK, wantCall: "proposals 4", wantBody: `"stage":"cold_start"`},
		{name: "session not diagnosed", path: "/sessions/4/proposals", err: experiments.ErrNotFound, wantCode: http.StatusNotFound, wantCall: "proposals 4"},
		{name: "engine down", path: "/sensitivity", err: engine.ErrUnavailable, wantCode: http.StatusBadGateway, wantCall: "profiles"},
		{name: "database down", path: "/scorecard", err: context.DeadlineExceeded, wantCode: http.StatusInternalServerError, wantCall: "scorecard"},
		{name: "not a session id", path: "/sessions/abc/proposals", wantCode: http.StatusNotFound},
		{name: "no session zero", path: "/sessions/0/proposals", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			garage := &fakeGarage{err: tt.err}
			deps := testDeps()
			deps.Garage = garage

			recorder := get(t, NewHandler(deps), http.MethodGet, tt.path)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d (%s)", recorder.Code, tt.wantCode, recorder.Body)
			}
			if strings.Join(garage.calls, ",") != tt.wantCall {
				t.Errorf("calls = %v, want %q", garage.calls, tt.wantCall)
			}
			body := recorder.Body.String()
			if !strings.Contains(body, tt.wantBody) || strings.Contains(body, "null") {
				t.Errorf("body = %s, want %s and no null", body, tt.wantBody)
			}
		})
	}
}

func TestGarageRoutesAreLeftOutWithoutAService(t *testing.T) {
	for _, path := range []string{"/sensitivity", "/scorecard", "/sessions/4/proposals"} {
		if recorder := get(t, NewHandler(testDeps()), http.MethodGet, path); recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, recorder.Code)
		}
	}
}
