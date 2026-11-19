package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

const sweepSQL = `
SELECT w.id, w.session_id, c.name, t.name, w.parameter, w.label, w.baseline_version_id,
       w.ai_hint_share, w.requested, w.planned, w.skipped, w.state, w.message, w.created_at, w.curve
FROM sweep w JOIN car c ON c.id = w.car_id JOIN track t ON t.id = w.track_id`

// Get returns one sweep with its steps and, once classified, its curves.
func (s *Service) Get(ctx context.Context, id int64) (contracts.Sweep, error) {
	sweep, err := scanSweep(s.database.QueryRowContext(ctx, sweepSQL+` WHERE w.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sweep, fmt.Errorf("sweep %d: %w", id, experiments.ErrNotFound)
	}
	if err != nil {
		return sweep, fmt.Errorf("read sweep %d: %w", id, err)
	}
	return sweep, s.fillSteps(ctx, &sweep)
}

// List returns every sweep, newest first.
func (s *Service) List(ctx context.Context) ([]contracts.Sweep, error) {
	rows, err := s.database.QueryContext(ctx, sweepSQL+` ORDER BY w.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list sweeps: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	sweeps := []contracts.Sweep{}
	for rows.Next() {
		sweep, err := scanSweep(rows)
		if err != nil {
			return nil, fmt.Errorf("read sweep: %w", err)
		}
		sweeps = append(sweeps, sweep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sweeps: %w", err)
	}
	for i := range sweeps {
		if err := s.fillSteps(ctx, &sweeps[i]); err != nil {
			return nil, err
		}
	}
	return sweeps, nil
}

type scanner interface{ Scan(dest ...any) error }

func scanSweep(row scanner) (contracts.Sweep, error) {
	var w contracts.Sweep
	var state string
	var requested, planned, skipped, curve []byte
	err := row.Scan(&w.Id, &w.SessionId, &w.CarName, &w.TrackName, &w.Parameter, &w.ParameterLabel,
		&w.BaselineVersionId, &w.AiHintShare, &requested, &planned, &skipped, &state, &w.Message,
		&w.CreatedAt, &curve)
	if err != nil {
		return w, err
	}
	w.State = contracts.SweepState(state)
	w.Steps = []contracts.SweepStep{}
	err = errors.Join(json.Unmarshal(requested, &w.Requested), json.Unmarshal(planned, &w.Planned),
		json.Unmarshal(skipped, &w.Skipped))
	if curve != nil {
		w.Curve = &contracts.DiscoveryCurve{}
		err = errors.Join(err, json.Unmarshal(curve, w.Curve))
	}
	return w, err
}

// step is one step as the sweep stores it: its experiment, and the response cells once read.
type step struct {
	clicks       int
	experimentID int64
	response     []byte // NULL until read
}

// readSteps lists a sweep's steps, oldest first.
func readSteps(ctx context.Context, q *sql.DB, sweepID int64) ([]step, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT clicks, experiment_id, response FROM sweep_step WHERE sweep_id = $1
		 ORDER BY added_at, experiment_id`, sweepID)
	if err != nil {
		return nil, fmt.Errorf("read steps of sweep %d: %w", sweepID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var steps []step
	for rows.Next() {
		var one step
		if err := rows.Scan(&one.clicks, &one.experimentID, &one.response); err != nil {
			return nil, fmt.Errorf("read step of sweep %d: %w", sweepID, err)
		}
		steps = append(steps, one)
	}
	return steps, rows.Err()
}

func (s *Service) fillSteps(ctx context.Context, sweep *contracts.Sweep) error {
	steps, err := readSteps(ctx, s.database, int64(sweep.Id))
	if err != nil {
		return err
	}
	for _, one := range steps {
		exp, err := s.ledger.Get(ctx, one.experimentID)
		if err != nil {
			return fmt.Errorf("read experiment %d of sweep %d: %w", one.experimentID, sweep.Id, err)
		}
		out := contracts.SweepStep{Clicks: one.clicks, ExperimentId: exp.Id, State: string(exp.State)}
		if exp.Verdict != nil && exp.Verdict.FindingClass != nil {
			finding := string(*exp.Verdict.FindingClass)
			out.FindingClass = &finding
		}
		if exp.Verdict != nil && exp.Verdict.Recommendation != nil {
			next := string(*exp.Verdict.Recommendation)
			out.Recommendation = &next
		}
		sweep.Steps = append(sweep.Steps, out)
	}
	return nil
}

// openSweeps lists the sweeps still running or waiting.
func openSweeps(ctx context.Context, q *sql.DB) ([]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM sweep WHERE state IN ('running', 'waiting') ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list open sweeps: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read open sweep: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// setState records where a sweep is; the message says why in words. An ended sweep stays ended.
func (s *Service) setState(ctx context.Context, id int64, state contracts.SweepState, message string) error {
	_, err := s.database.ExecContext(ctx,
		`UPDATE sweep SET state = $2, message = $3, updated_at = now()
		 WHERE id = $1 AND (state <> $2 OR message <> $3) AND state IN ('running', 'waiting')`,
		id, string(state), message)
	if err != nil {
		return fmt.Errorf("record state of sweep %d: %w", id, err)
	}
	return nil
}

// fail ends a sweep the sweep itself had to stop, and says why.
func (s *Service) fail(ctx context.Context, id int64, message string) error {
	s.logger.WarnContext(ctx, "sweep failed", "sweep_id", id, "message", message)
	return s.setState(ctx, id, contracts.SweepStateFailed, message)
}

func addStep(ctx context.Context, q *sql.DB, sweepID int64, clicks int, experimentID int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO sweep_step (sweep_id, clicks, experiment_id) VALUES ($1, $2, $3)`,
		sweepID, clicks, experimentID)
	if err != nil {
		return fmt.Errorf("add experiment %d to sweep %d: %w", experimentID, sweepID, err)
	}
	return nil
}
