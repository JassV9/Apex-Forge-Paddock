// Package telemetry sets up core's OpenTelemetry metrics and exposes them to Prometheus.
package telemetry

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Metrics is a meter provider and the HTTP handler that serves what it has measured.
type Metrics struct {
	Provider metric.MeterProvider
	Handler  http.Handler // GET /metrics in the Prometheus text format
}

// NewMetrics builds a meter provider with its own Prometheus registry: nothing is registered
// globally, so tests and several instances never share state.
func NewMetrics() (*Metrics, error) {
	registry := prometheus.NewRegistry()
	exporter, err := otelprometheus.New(
		otelprometheus.WithRegisterer(registry),
		otelprometheus.WithoutScopeInfo(),
	)
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	return &Metrics{
		Provider: provider,
		Handler:  promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}, nil
}
