package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

type fakeDatabase struct{ err error }

func (f fakeDatabase) PingContext(context.Context) error { return f.err }

type fakeSessions struct {
	list []contracts.SessionSummary
	err  error
}

func (f fakeSessions) ListSessions(context.Context) ([]contracts.SessionSummary, error) {
	return f.list, f.err
}

// testDeps are working dependencies; a test overrides the one it is about.
func testDeps() Deps {
	return Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:  "1.2.3",
		Database: fakeDatabase{},
		Sessions: fakeSessions{},
	}
}

func get(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, path, nil))
	return recorder
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		{name: "unknown path", method: http.MethodGet, path: "/nope", wantCode: http.StatusNotFound},
		{name: "health is read-only", method: http.MethodPost, path: "/health", wantCode: http.StatusMethodNotAllowed},
		{name: "no metrics route without a handler", method: http.MethodGet, path: "/metrics", wantCode: http.StatusNotFound},
	}
	handler := NewHandler(testDeps())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := get(t, handler, tt.method, tt.path).Code; got != tt.wantCode {
				t.Errorf("status code = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestMetricsRouteServesTheGivenHandler(t *testing.T) {
	deps := testDeps()
	deps.Metrics = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("paddock_up 1\n"))
	})

	recorder := get(t, NewHandler(deps), http.MethodGet, "/metrics")

	if recorder.Code != http.StatusOK || recorder.Body.String() != "paddock_up 1\n" {
		t.Errorf("GET /metrics = %d %q", recorder.Code, recorder.Body.String())
	}
}
