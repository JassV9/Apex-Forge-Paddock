package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

type fakeDatabase struct{ err error }

func (f fakeDatabase) PingContext(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		database   fakeDatabase
		wantStatus contracts.HealthStatus
	}{
		{name: "database reachable", database: fakeDatabase{}, wantStatus: contracts.HealthStatusOk},
		{
			name:       "database unreachable",
			database:   fakeDatabase{err: errors.New("connection refused")},
			wantStatus: contracts.HealthStatusDegraded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			handler := NewHandler(logger, "1.2.3", tt.database)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status code = %d, want 200", recorder.Code)
			}
			// Decoding into the generated type also validates the body against the contract.
			var got contracts.Health
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("body does not match the Health contract: %v", err)
			}
			want := contracts.Health{
				Service: contracts.HealthServiceCore,
				Version: "1.2.3",
				Status:  tt.wantStatus,
			}
			if got != want {
				t.Errorf("health = %+v, want %+v", got, want)
			}
		})
	}
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		wantCode int
	}{
		{name: "unknown path", method: http.MethodGet, path: "/nope", wantCode: http.StatusNotFound},
		{
			name:     "health is read-only",
			method:   http.MethodPost,
			path:     "/health",
			wantCode: http.StatusMethodNotAllowed,
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewHandler(logger, "test", fakeDatabase{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			if recorder.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", recorder.Code, tt.wantCode)
			}
		})
	}
}
