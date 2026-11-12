package features

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

const sampleSpool = "../ingest/testdata/spool"

// fakeEngine answers every pass with one metric, and calls pass 2 off track.
type fakeEngine struct {
	failure  error
	requests []contracts.FeaturesRequest
}

func (f *fakeEngine) Features(_ context.Context, request contracts.FeaturesRequest) (contracts.FeaturesResponse, error) {
	f.requests = append(f.requests, request)
	if f.failure != nil {
		return contracts.FeaturesResponse{}, f.failure
	}
	var reply contracts.FeaturesResponse
	for _, pass := range request.Passes {
		duration := pass.DurationS
		features := contracts.PassFeatures{
			Seq: pass.Seq, Valid: true, SpeedClass: "medium", Flags: []string{},
			Metrics: contracts.FeaturesResponsePassesElemMetrics{"corner_time_s": &duration, "brake_dist_m": nil},
		}
		if pass.Seq == 2 {
			reason := "off track"
			features.Valid, features.InvalidReason = false, &reason
		}
		reply.Passes = append(reply.Passes, features)
	}
	return reply, nil
}

func ingested(t *testing.T, schema string) *sql.DB {
	t.Helper()
	database := testdb.Open(t, schema)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := ingest.Config{SpoolDir: sampleSpool, DataDir: t.TempDir(), PollInterval: time.Second}
	service, err := ingest.New(cfg, ingest.NewPostgresStore(database), logger, noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("ingest.New: %v", err)
	}
	if _, err := service.ScanOnce(t.Context()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	return database
}

func worker(t *testing.T, database *sql.DB, eng Engine) *Worker {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w, err := NewWorker(database, eng, logger, noop.NewMeterProvider().Meter("test"), time.Millisecond)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	return w
}

func drain(t *testing.T, w *Worker) (jobs int, last error) {
	t.Helper()
	for range 50 {
		worked, err := w.RunOnce(t.Context())
		if !worked {
			return jobs, last
		}
		jobs, last = jobs+1, err
	}
	t.Fatal("the queue never emptied")
	return 0, nil
}

func count(t *testing.T, database *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(t.Context(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestQueuedJobsPutTheEnginesFeaturesOnThePasses(t *testing.T) {
	database := ingested(t, "features_done")
	eng := &fakeEngine{}

	jobs, err := drain(t, worker(t, database, eng))

	if err != nil || jobs == 0 {
		t.Fatalf("drain = %d jobs, err %v", jobs, err)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM jobs WHERE state = 'done'`:                                      jobs,
		`SELECT count(*) FROM jobs WHERE state <> 'done'`:                                     0,
		`SELECT count(*) FROM corner_pass WHERE features->'metrics' ? 'corner_time_s'`:        3,
		`SELECT count(*) FROM corner_pass WHERE features->'metrics'->'brake_dist_m' = 'null'`: 3,
		`SELECT count(*) FROM corner_pass WHERE features->>'speed_class' = 'medium'`:          3,
		`SELECT count(*) FROM corner_pass WHERE valid`:                                        2,
		`SELECT count(*) FROM corner_pass WHERE NOT valid AND invalid_reason = 'off track'`:   1,
	} {
		if got := count(t, database, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	first := eng.requests[0]
	if first.ParquetDir == nil || len(first.Corners) == 0 || first.TrackLengthM <= 0 {
		t.Errorf("engine was asked with %+v", first)
	}
	if first.DrivenAxle != contracts.FeaturesRequestDrivenAxleRear {
		t.Errorf("driven axle = %s, want rear for an unknown drivetrain", first.DrivenAxle)
	}
}

func TestJobsWaitForAnEngineThatIsDown(t *testing.T) {
	database := ingested(t, "features_waiting")
	eng := &fakeEngine{failure: fmt.Errorf("connection refused: %w", engine.ErrUnavailable)}
	w := worker(t, database, eng)
	queued := count(t, database, `SELECT count(*) FROM jobs`)

	for range 3 * MaxAttempts {
		if _, err := w.RunOnce(t.Context()); err == nil {
			t.Fatal("RunOnce reported no error while the engine is down")
		}
	}

	if got := count(t, database, `SELECT count(*) FROM jobs WHERE state = 'queued' AND attempts = 0`); got != queued {
		t.Errorf("%d of %d jobs still wait with no attempt counted", got, queued)
	}
}

func TestAJobTheEngineRefusesIsGivenUpAfterMaxAttempts(t *testing.T) {
	database := ingested(t, "features_failed")
	eng := &fakeEngine{failure: fmt.Errorf("422: %w", engine.ErrRejected)}

	jobs, err := drain(t, worker(t, database, eng))

	if err == nil {
		t.Fatal("drain reported no error")
	}
	queued := count(t, database, `SELECT count(*) FROM jobs`)
	if jobs != queued*MaxAttempts {
		t.Errorf("%d attempts for %d jobs, want %d each", jobs, queued, MaxAttempts)
	}
	failed := count(t, database, `SELECT count(*) FROM jobs WHERE state = 'failed' AND error LIKE '%422%'`)
	if failed != queued {
		t.Errorf("%d of %d jobs are failed with the engine's answer", failed, queued)
	}
}

func TestRunQueuesInterruptedJobsAgainAndStopsWithItsContext(t *testing.T) {
	database := ingested(t, "features_run")
	if _, err := database.ExecContext(t.Context(), `UPDATE jobs SET state = 'running'`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- worker(t, database, &fakeEngine{}).Run(ctx) }()

	deadline := time.Now().Add(10 * time.Second)
	for count(t, database, `SELECT count(*) FROM jobs WHERE state <> 'done'`) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("interrupted jobs were not finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run: %v", err)
	}
}
