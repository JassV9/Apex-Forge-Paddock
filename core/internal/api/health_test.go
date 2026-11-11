package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

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
			deps := testDeps()
			deps.Database = tt.database

			recorder := get(t, NewHandler(deps), http.MethodGet, "/health")

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
