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

// fakeBaseSetups proposes experiment 9 unless told nothing qualifies.
type fakeBaseSetups struct {
	err     error
	nothing bool
	calls   []string
}

func (f *fakeBaseSetups) Build(_ context.Context, sessionID int64, driver contracts.BaseSetupCreateDriver) (contracts.BaseSetupReply, error) {
	f.calls = append(f.calls, fmt.Sprintf("build %d %s", sessionID, driver))
	reply := contracts.BaseSetupReply{CarName: "Ferrari 488 GT3", BaseSetup: contracts.BaseSetup{
		Channel: "corner_time_s", Changes: []contracts.BaseSetupChange{},
		LeftOut: []contracts.BaseSetupLeftOut{}, Measured: []contracts.BaseSetupMeasured{},
	}}
	if f.nothing {
		refusal := "This car has measured nothing yet."
		reply.Error = &refusal
	} else {
		id := 9
		reply.ExperimentId = &id
	}
	return reply, f.err
}

func TestBaseSetupRoute(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		service  fakeBaseSetups
		wantCode int
		wantCall string
		wantBody string
	}{
		{name: "proposed", path: "/sessions/4/base-setup", body: `{"driver":"ai"}`, wantCode: http.StatusCreated, wantCall: "build 4 ai", wantBody: `"experiment_id":9`},
		{name: "nothing qualifies", path: "/sessions/4/base-setup", body: `{"driver":"human"}`, service: fakeBaseSetups{nothing: true}, wantCode: http.StatusConflict, wantCall: "build 4 human", wantBody: `"error":"This car has measured nothing yet."`},
		{name: "no driver", path: "/sessions/4/base-setup", body: `{}`, wantCode: http.StatusUnprocessableEntity},
		{name: "unknown driver", path: "/sessions/4/base-setup", body: `{"driver":"robot"}`, wantCode: http.StatusUnprocessableEntity},
		{name: "unknown session", path: "/sessions/4/base-setup", body: `{"driver":"ai"}`, service: fakeBaseSetups{err: experiments.ErrNotFound}, wantCode: http.StatusNotFound, wantCall: "build 4 ai"},
		{name: "engine down", path: "/sessions/4/base-setup", body: `{"driver":"ai"}`, service: fakeBaseSetups{err: engine.ErrUnavailable}, wantCode: http.StatusBadGateway, wantCall: "build 4 ai"},
		{name: "not a session id", path: "/sessions/abc/base-setup", body: `{"driver":"ai"}`, wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.service
			deps := testDeps()
			deps.BaseSetups = &service

			recorder := send(t, NewHandler(deps), http.MethodPost, tt.path, tt.body)

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d (%s)", recorder.Code, tt.wantCode, recorder.Body)
			}
			if strings.Join(service.calls, ",") != tt.wantCall {
				t.Errorf("calls = %v, want %q", service.calls, tt.wantCall)
			}
			if body := recorder.Body.String(); !strings.Contains(body, tt.wantBody) {
				t.Errorf("body = %s, want %s", body, tt.wantBody)
			}
		})
	}
}

func TestBaseSetupRouteIsLeftOutWithoutAService(t *testing.T) {
	recorder := send(t, NewHandler(testDeps()), http.MethodPost, "/sessions/4/base-setup", `{"driver":"ai"}`)
	if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST without the service = %d, want 404 or 405", recorder.Code)
	}
}
