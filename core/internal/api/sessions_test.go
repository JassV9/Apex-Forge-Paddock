package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestListSessions(t *testing.T) {
	demo := contracts.SessionSummary{
		Id: 1, SpoolId: "demo-fa26-cota", CarName: "VRC Formula Alpha 2026",
		TrackName: "Circuit of the Americas", Layout: "layout_f1_2025",
		StartedAt: "2026-10-26T15:10:03Z", Source: contracts.SessionSummarySourceDemo,
		Passes: 136, Corners: 17,
	}
	tests := []struct {
		name     string
		sessions fakeSessions
		wantCode int
		wantBody []contracts.SessionSummary
	}{
		{name: "nothing recorded yet", sessions: fakeSessions{}, wantCode: http.StatusOK, wantBody: []contracts.SessionSummary{}},
		{
			name:     "one session",
			sessions: fakeSessions{list: []contracts.SessionSummary{demo}},
			wantCode: http.StatusOK,
			wantBody: []contracts.SessionSummary{demo},
		},
		{
			name:     "database error",
			sessions: fakeSessions{err: errors.New("connection reset")},
			wantCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps()
			deps.Sessions = tt.sessions

			recorder := get(t, NewHandler(deps), http.MethodGet, "/sessions")

			if recorder.Code != tt.wantCode {
				t.Fatalf("status code = %d, want %d", recorder.Code, tt.wantCode)
			}
			if tt.wantBody == nil {
				if strings.Contains(recorder.Body.String(), "connection reset") {
					t.Error("the error response leaks the internal error")
				}
				return
			}
			var got []contracts.SessionSummary
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("body is not a list of SessionSummary: %v", err)
			}
			if !reflect.DeepEqual(got, tt.wantBody) {
				t.Errorf("sessions = %+v, want %+v", got, tt.wantBody)
			}
			if strings.TrimSpace(recorder.Body.String()) == "null" {
				t.Error("an empty list must be [], not null")
			}
		})
	}
}
