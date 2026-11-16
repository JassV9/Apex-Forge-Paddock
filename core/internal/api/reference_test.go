package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/reference"
)

type fakeReference struct {
	err   error
	calls []string
}

func (f *fakeReference) Session(_ context.Context, sessionID int64, setupHash string) (contracts.SessionReference, error) {
	f.calls = append(f.calls, fmt.Sprintf("session %d %q", sessionID, setupHash))
	return contracts.SessionReference{SessionId: int(sessionID), Setups: []contracts.ReferenceSetup{},
		Corners: []contracts.CornerReference{}}, f.err
}

func (f *fakeReference) ForSetup(_ context.Context, car, track, layout, setupHash string) (contracts.AiReference, error) {
	f.calls = append(f.calls, fmt.Sprintf("setup %s %s %q %s", car, track, layout, setupHash))
	return contracts.AiReference{Car: car, Corners: []contracts.AiCornerTime{}}, f.err
}

func TestReferenceRoutes(t *testing.T) {
	missing := fmt.Errorf("session 9: %w", reference.ErrNotFound)
	tests := []struct {
		name     string
		path     string
		err      error
		wantCode int
		wantCall string
		wantBody string
	}{
		{name: "a session on its latest setup", path: "/sessions/2/reference", wantCode: http.StatusOK, wantCall: `session 2 ""`, wantBody: `"corners":[]`},
		{name: "a session on an asked setup", path: "/sessions/2/reference?setup_hash=s1", wantCode: http.StatusOK, wantCall: `session 2 "s1"`},
		{name: "no such session", path: "/sessions/9/reference", err: missing, wantCode: http.StatusNotFound, wantCall: `session 9 ""`, wantBody: "not found"},
		{name: "not a session id", path: "/sessions/0/reference", wantCode: http.StatusNotFound},
		{name: "the AI on one setup", path: "/reference?car=ks_ferrari_488_gt3&track=fn_cota&layout=gp&setup_hash=s1", wantCode: http.StatusOK, wantCall: `setup ks_ferrari_488_gt3 fn_cota "gp" s1`, wantBody: `"corners":[]`},
		{name: "a track without layouts", path: "/reference?car=c&track=t&setup_hash=s1", wantCode: http.StatusOK, wantCall: `setup c t "" s1`},
		{name: "no setup named", path: "/reference?car=c&track=t", wantCode: http.StatusUnprocessableEntity, wantBody: "required"},
		{name: "no car named", path: "/reference?track=t&setup_hash=s1", wantCode: http.StatusUnprocessableEntity},
		{name: "database down", path: "/reference?car=c&track=t&setup_hash=s1", err: context.DeadlineExceeded, wantCode: http.StatusInternalServerError, wantCall: `setup c t "" s1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeReference{err: tt.err}
			deps := testDeps()
			deps.Reference = service

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
