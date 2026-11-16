package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/passes"
)

type fakePasses struct {
	err   error
	calls []string
}

func (f *fakePasses) Passes(_ context.Context, sessionID int64) (contracts.SessionPasses, error) {
	f.calls = append(f.calls, fmt.Sprintf("passes %d", sessionID))
	return contracts.SessionPasses{
		SessionId: int(sessionID), Corners: []contracts.CornerSummary{}, Passes: []contracts.PassRecord{},
	}, f.err
}

func (f *fakePasses) Trace(_ context.Context, sessionID int64, seq, maxPoints int) (contracts.PassTrace, error) {
	f.calls = append(f.calls, fmt.Sprintf("trace %d %d %d", sessionID, seq, maxPoints))
	return contracts.PassTrace{SessionId: int(sessionID), Seq: seq, Corner: 3, NRecorded: 2, N: 2, Step: 1}, f.err
}

func TestPassesRoutes(t *testing.T) {
	missing := fmt.Errorf("session 9: %w", passes.ErrNotFound)
	tests := []struct {
		name     string
		path     string
		err      error
		wantCode int
		wantCall string
		wantBody string
	}{
		{name: "the passes", path: "/sessions/2/passes", wantCode: http.StatusOK, wantCall: "passes 2", wantBody: `"corners":[]`},
		{name: "no such session", path: "/sessions/9/passes", err: missing, wantCode: http.StatusNotFound, wantCall: "passes 9", wantBody: "session 9: not found"},
		{name: "not a session id", path: "/sessions/x/passes", wantCode: http.StatusNotFound},
		{name: "samples at the default size", path: "/sessions/2/passes/7/samples", wantCode: http.StatusOK, wantCall: "trace 2 7 1000", wantBody: `"corner":3`},
		{name: "samples thinned", path: "/sessions/2/passes/7/samples?max_points=200", wantCode: http.StatusOK, wantCall: "trace 2 7 200"},
		{name: "too few points", path: "/sessions/2/passes/7/samples?max_points=49", wantCode: http.StatusUnprocessableEntity, wantBody: "max_points"},
		{name: "too many points", path: "/sessions/2/passes/7/samples?max_points=5001", wantCode: http.StatusUnprocessableEntity},
		{name: "points not a number", path: "/sessions/2/passes/7/samples?max_points=all", wantCode: http.StatusUnprocessableEntity},
		{name: "no pass zero", path: "/sessions/2/passes/0/samples", wantCode: http.StatusNotFound},
		{name: "pass without samples", path: "/sessions/2/passes/7/samples", err: passes.ErrNotFound, wantCode: http.StatusNotFound, wantCall: "trace 2 7 1000"},
		{name: "files unreadable", path: "/sessions/2/passes/7/samples", err: context.DeadlineExceeded, wantCode: http.StatusInternalServerError, wantCall: "trace 2 7 1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakePasses{err: tt.err}
			deps := testDeps()
			deps.Passes = service

			recorder := get(t, NewHandler(deps), http.MethodGet, tt.path)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d (%s)", recorder.Code, tt.wantCode, recorder.Body.String())
			}
			if got := strings.Join(service.calls, ","); got != tt.wantCall {
				t.Errorf("calls = %q, want %q", got, tt.wantCall)
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", recorder.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestPassesRoutesAreReadOnly(t *testing.T) {
	deps := testDeps()
	deps.Passes = &fakePasses{}

	recorder := get(t, NewHandler(deps), http.MethodPost, "/sessions/2/passes")

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /sessions/2/passes = %d, want 405", recorder.Code)
	}
}
