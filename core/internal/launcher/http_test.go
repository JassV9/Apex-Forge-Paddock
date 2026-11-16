package launcher

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const launchBody = `{"car_id":"ks_ferrari_488_gt3","track_id":"fn_cota","layout":"gp_paddock"}`

func TestHandler(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		host        string
		origin      string
		contentType string
		body        string
		running     bool
		wantCode    int
		wantCORS    bool
	}{
		{name: "content", method: "GET", path: "/content", wantCode: 200},
		{name: "status from the dashboard", method: "GET", path: "/status", origin: "http://localhost:5173", wantCode: 200, wantCORS: true},
		{name: "preflight", method: "OPTIONS", path: "/launch", origin: "http://127.0.0.1:5173", wantCode: 204, wantCORS: true},
		{name: "other website", method: "POST", path: "/launch", origin: "https://example.com", contentType: "application/json", body: launchBody, wantCode: 403},
		{name: "rebound host name", method: "GET", path: "/status", host: "evil.example:8090", wantCode: 403},
		{name: "form post", method: "POST", path: "/launch", contentType: "text/plain", body: launchBody, wantCode: 415},
		{name: "missing field", method: "POST", path: "/launch", contentType: "application/json", body: `{"car_id":"x"}`, wantCode: 422},
		{name: "unknown field", method: "POST", path: "/launch", contentType: "application/json", body: `{"car_id":"x","track_id":"y","layout":"","skin":"red"}`, wantCode: 422},
		{name: "game running", method: "POST", path: "/launch", contentType: "application/json", body: launchBody, running: true, wantCode: 409},
		{name: "launch", method: "POST", path: "/launch", origin: "http://localhost:5173", contentType: "application/json; charset=utf-8", body: launchBody, wantCode: 202, wantCORS: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestService(newFixture(t), &fakeProcesses{running: tt.running}, &fakeOpener{})
			defer service.Wait()
			request := httptest.NewRequestWithContext(t.Context(), tt.method, "http://127.0.0.1:8090"+tt.path, strings.NewReader(tt.body))
			if tt.host != "" {
				request.Host = tt.host
			}
			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}
			if tt.contentType != "" {
				request.Header.Set("Content-Type", tt.contentType)
			}
			recorder := httptest.NewRecorder()
			NewHandler(service, slog.New(slog.DiscardHandler)).ServeHTTP(recorder, request)

			if recorder.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d: %s", recorder.Code, tt.wantCode, recorder.Body)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Origin") != ""; got != tt.wantCORS {
				t.Errorf("CORS header present = %v, want %v", got, tt.wantCORS)
			}
		})
	}
}

func TestStatusBody(t *testing.T) {
	service := newTestService(newFixture(t), &fakeProcesses{running: true}, &fakeOpener{})
	recorder := httptest.NewRecorder()
	NewHandler(service, slog.New(slog.DiscardHandler)).
		ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost:8090/status", nil))

	var status contracts.LauncherStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.GameRunning || status.Version != "test" || status.Service != contracts.LauncherStatusServiceLauncher {
		t.Errorf("status = %+v", status)
	}
}
