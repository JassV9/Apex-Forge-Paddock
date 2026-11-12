// Package features runs the jobs that have the engine compute the features of newly ingested
// corner passes (docs/adr/0006: the jobs table is the queue).
package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
)

// MaxAttempts is how often a job may fail before it is given up as failed. An engine that is
// down does not count: the job waits for it.
const MaxAttempts = 5

// Engine computes pass features. *engine.Client implements it.
type Engine interface {
	Features(ctx context.Context, request contracts.FeaturesRequest) (contracts.FeaturesResponse, error)
}

// Worker claims feature jobs one at a time.
type Worker struct {
	database *sql.DB
	engine   Engine
	logger   *slog.Logger
	poll     time.Duration
	jobs     metric.Int64Counter
}

// NewWorker builds a worker that looks for work every poll interval.
func NewWorker(database *sql.DB, eng Engine, logger *slog.Logger, meter metric.Meter, poll time.Duration) (*Worker, error) {
	jobs, err := meter.Int64Counter("paddock_jobs_total",
		metric.WithDescription("Engine jobs finished, by kind and outcome."))
	if err != nil {
		return nil, fmt.Errorf("create jobs counter: %w", err)
	}
	return &Worker{database: database, engine: eng, logger: logger, poll: poll, jobs: jobs}, nil
}

// Run works through the queue until ctx is cancelled. Jobs left "running" by a process that
// died are queued again first: core is one process, so nobody else can be running them.
func (w *Worker) Run(ctx context.Context) error {
	_, err := w.database.ExecContext(ctx,
		`UPDATE jobs SET state = 'queued', updated_at = now() WHERE state = 'running' AND kind = $1`,
		ingest.JobFeatures)
	if err != nil {
		return fmt.Errorf("requeue interrupted jobs: %w", err)
	}
	ticker := time.NewTicker(w.poll)
	defer ticker.Stop()
	for {
		for {
			worked, err := w.RunOnce(ctx)
			if err != nil {
				w.logger.WarnContext(ctx, "features job", "error", err)
			}
			if !worked || err != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type job struct {
	id       int64
	payload  []byte
	attempts int
}

// The job that has waited longest since it was last touched goes first, so one that keeps
// failing does not hold up the others.
const claimSQL = `
UPDATE jobs SET state = 'running', attempts = attempts + 1, updated_at = now()
WHERE id = (
    SELECT id FROM jobs WHERE state = 'queued' AND kind = $1
    ORDER BY updated_at, id FOR UPDATE SKIP LOCKED LIMIT 1)
RETURNING id, payload, attempts`

// RunOnce claims and runs one job. It reports whether there was one; the error is the job's.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	var claimed job
	err := w.database.QueryRowContext(ctx, claimSQL, ingest.JobFeatures).
		Scan(&claimed.id, &claimed.payload, &claimed.attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}
	failure := w.compute(ctx, claimed.payload)
	return true, errors.Join(failure, w.finish(ctx, claimed, failure))
}

// finish records how the job ended: done, back in the queue, or failed for good.
func (w *Worker) finish(ctx context.Context, claimed job, failure error) error {
	state, outcome, attempts := "done", "done", claimed.attempts
	switch {
	case failure == nil:
	case errors.Is(failure, engine.ErrUnavailable):
		state, outcome, attempts = "queued", "waiting", claimed.attempts-1
	case claimed.attempts < MaxAttempts:
		state, outcome = "queued", "retried"
	default:
		state, outcome = "failed", "failed"
	}
	var message any
	if failure != nil {
		message = failure.Error()
	}
	_, err := w.database.ExecContext(ctx,
		`UPDATE jobs SET state = $2, attempts = $3, error = $4, updated_at = now() WHERE id = $1`,
		claimed.id, state, attempts, message)
	if err != nil {
		return fmt.Errorf("finish job %d: %w", claimed.id, err)
	}
	w.jobs.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", ingest.JobFeatures), attribute.String("outcome", outcome)))
	return nil
}

func (w *Worker) compute(ctx context.Context, payload []byte) error {
	var work ingest.FeaturesJob
	if err := json.Unmarshal(payload, &work); err != nil {
		return fmt.Errorf("decode job payload: %w", err)
	}
	sessionID, request, err := buildRequest(ctx, w.database, work)
	if err != nil {
		return err
	}
	if len(request.Passes) == 0 {
		return nil
	}
	reply, err := w.engine.Features(ctx, request)
	if err != nil {
		return fmt.Errorf("session %s: %w", work.SpoolID, err)
	}
	if err := saveFeatures(ctx, w.database, sessionID, reply.Passes); err != nil {
		return err
	}
	w.logger.InfoContext(ctx, "features computed", "session_id", work.SpoolID, "passes", len(reply.Passes))
	return nil
}
