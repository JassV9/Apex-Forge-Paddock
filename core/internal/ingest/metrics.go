package ingest

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// metrics are ingest's OpenTelemetry instruments. Exposed to Prometheus as
// paddock_ingest_lines_total, paddock_ingest_rejects_total and paddock_ingest_lag_seconds.
type metrics struct {
	lines   metric.Int64Counter
	rejects metric.Int64Counter
	lag     metric.Float64Histogram
}

func newMetrics(meter metric.Meter) (*metrics, error) {
	lines, err := meter.Int64Counter("paddock.ingest.lines",
		metric.WithDescription("Spool lines read, by outcome."))
	if err != nil {
		return nil, fmt.Errorf("create lines counter: %w", err)
	}
	rejects, err := meter.Int64Counter("paddock.ingest.rejects",
		metric.WithDescription("Spool lines not ingested, by reason."))
	if err != nil {
		return nil, fmt.Errorf("create rejects counter: %w", err)
	}
	lag, err := meter.Float64Histogram("paddock.ingest.lag", metric.WithUnit("s"),
		metric.WithDescription("Time from a spool file being written to it being ingested."))
	if err != nil {
		return nil, fmt.Errorf("create lag histogram: %w", err)
	}
	return &metrics{lines: lines, rejects: rejects, lag: lag}, nil
}

func (m *metrics) record(ctx context.Context, chunk Chunk, result ChunkResult, lag time.Duration) {
	outcome := func(name string) metric.MeasurementOption {
		return metric.WithAttributes(attribute.String("outcome", name))
	}
	rejected := 0
	for _, counts := range []map[string]int{chunk.Rejects, result.Rejects} {
		for reason, count := range counts {
			rejected += count
			m.rejects.Add(ctx, int64(count), metric.WithAttributes(attribute.String("reason", reason)))
		}
	}
	m.lines.Add(ctx, int64(chunk.Lines-rejected-result.Duplicates), outcome("ingested"))
	m.lines.Add(ctx, int64(result.Duplicates), outcome("duplicate"))
	m.lines.Add(ctx, int64(rejected), outcome("rejected"))
	m.lag.Record(ctx, lag.Seconds())
}
