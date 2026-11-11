package telemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCountersAppearInThePrometheusOutput(t *testing.T) {
	metrics, err := NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	counter, err := metrics.Provider.Meter("test").Int64Counter("paddock.ingest.lines")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}
	counter.Add(t.Context(), 3)

	recorder := httptest.NewRecorder()
	metrics.Handler.ServeHTTP(recorder,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))

	found := false
	for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
		if strings.HasPrefix(line, "paddock_ingest_lines_total") && strings.HasSuffix(line, " 3") {
			found = true
		}
	}
	if recorder.Code != http.StatusOK || !found {
		t.Errorf("GET /metrics = %d, no paddock_ingest_lines_total 3 in:\n%s", recorder.Code, recorder.Body.String())
	}
}

func TestInstancesDoNotShareState(t *testing.T) {
	first, err := NewMetrics()
	if err != nil {
		t.Fatalf("first NewMetrics: %v", err)
	}
	if _, err := NewMetrics(); err != nil {
		t.Fatalf("second NewMetrics: %v", err)
	}
	if first.Provider == nil || first.Handler == nil {
		t.Error("NewMetrics returned an incomplete value")
	}
}
