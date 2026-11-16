package passes_test

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/passes"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// The sample spool: three passes over corners 1 to 3 of a synthetic track, one per chunk.
const sampleSpool = "../ingest/testdata/spool"

type fixture struct {
	database  *sql.DB
	service   *passes.Service
	sessionID int64
}

// newFixture ingests the sample spool as core would, into a fresh schema and data directory.
func newFixture(t *testing.T, schema string) *fixture {
	t.Helper()
	database := testdb.Open(t, schema)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dataDir := t.TempDir()
	cfg := ingest.Config{SpoolDir: sampleSpool, DataDir: dataDir, PollInterval: time.Second}
	ingester, err := ingest.New(cfg, ingest.NewPostgresStore(database), logger, noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("ingest.New: %v", err)
	}
	if _, err := ingester.ScanOnce(t.Context()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	f := &fixture{database: database, service: passes.NewService(database, dataDir)}
	if err := database.QueryRowContext(t.Context(), `SELECT id FROM session`).Scan(&f.sessionID); err != nil {
		t.Fatalf("find the sample session: %v", err)
	}
	return f
}

func TestPassesReadsWhatIngestStored(t *testing.T) {
	f := newFixture(t, "passes_list")
	_, err := f.database.ExecContext(t.Context(), `
		UPDATE corner_pass SET features = '{"flags": ["flat-out corner"], "speed_class": "fast",
		    "metrics": {"v_min_kmh": 151.5, "brake_peak": null}}' WHERE seq = 1;
		UPDATE corner_pass SET valid = false, invalid_reason = 'off track' WHERE seq = 3`)
	if err != nil {
		t.Fatalf("stand in for the engine: %v", err)
	}

	got, err := f.service.Passes(t.Context(), f.sessionID)

	if err != nil {
		t.Fatalf("Passes: %v", err)
	}
	if len(got.Passes) != 3 || len(got.Corners) != 3 || got.SessionId != int(f.sessionID) {
		t.Fatalf("%d passes over %d corners, want 3 over 3", len(got.Passes), len(got.Corners))
	}
	if got.TrackLengthM != 2949.98 {
		t.Errorf("track length = %v, want the session line's 2949.98", got.TrackLengthM)
	}
	first := got.Passes[0]
	if first.Seq != 1 || first.Corner != 1 || first.Lap == nil || *first.Lap != 0 || first.NSamples != 545 {
		t.Errorf("first pass = %+v", first)
	}
	if first.Conditions == nil || first.Conditions.FuelL != 42.5 || len(first.Conditions.TyreCoreC) != 4 {
		t.Errorf("conditions = %+v", first.Conditions)
	}
	if len(first.Metrics) != 1 || first.Metrics["v_min_kmh"] != 151.5 || first.SpeedClass == nil || len(first.Flags) != 1 {
		t.Errorf("features = %v %v %v; a null metric must be left out", first.Metrics, first.SpeedClass, first.Flags)
	}
	if second := got.Passes[1]; len(second.Metrics) != 0 || second.Flags == nil || second.SpeedClass != nil {
		t.Errorf("a pass without features = %+v", second)
	}
	if third := got.Corners[2]; third.Name != "T3" || third.ValidPasses != 0 || third.BestS != nil {
		t.Errorf("corner of the invalid pass = %+v", third)
	}
}

func TestTraceReadsThePassFromItsPart(t *testing.T) {
	f := newFixture(t, "passes_trace")

	full, err := f.service.Trace(t.Context(), f.sessionID, 3, passes.MaxPoints)
	if err != nil {
		t.Fatalf("Trace: %v", err)
	}
	thin, err := f.service.Trace(t.Context(), f.sessionID, 3, 100)
	if err != nil {
		t.Fatalf("Trace thinned: %v", err)
	}

	if full.Corner != 3 || full.NRecorded != 434 || full.N != 434 || full.Step != 1 || len(full.Columns.V) != 434 {
		t.Errorf("full trace: corner %d, %d of %d, step %d", full.Corner, full.N, full.NRecorded, full.Step)
	}
	if thin.N > 100 || thin.NRecorded != 434 || thin.Step != 5 || len(thin.Axles.RearSlipDeg) != thin.N {
		t.Errorf("thinned trace: %d of %d, step %d", thin.N, thin.NRecorded, thin.Step)
	}
	if full.Columns.T[0] != 0.0109 || thin.Columns.T[thin.N-1] != full.Columns.T[full.N-1] {
		t.Errorf("trace starts at %v and ends at %v", full.Columns.T[0], thin.Columns.T[thin.N-1])
	}
}

func TestNotFound(t *testing.T) {
	f := newFixture(t, "passes_missing")
	if _, err := f.database.ExecContext(t.Context(), `UPDATE corner_pass SET seq = 9 WHERE seq = 2`); err != nil {
		t.Fatalf("renumber a pass: %v", err)
	}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "passes of no session", call: func() error { _, err := f.service.Passes(t.Context(), 999); return err }},
		{name: "samples of no session", call: func() error { _, err := f.service.Trace(t.Context(), 999, 1, 100); return err }},
		{name: "no such pass", call: func() error { _, err := f.service.Trace(t.Context(), f.sessionID, 4, 100); return err }},
		{name: "a pass without samples", call: func() error { _, err := f.service.Trace(t.Context(), f.sessionID, 9, 100); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, passes.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}
