package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const headSQL = `
SELECT e.id, e.kind, e.state, h.statement, h.source, c.name, t.name, COALESCE(sp.name, ''),
       COALESCE(sp.label, ''), e.clicks, e.success_test, e.guardrails, e.mde, e.extensions,
       e.created_at
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN car c ON c.id = h.car_id
JOIN track t ON t.id = h.track_id
LEFT JOIN setup_parameter sp ON sp.id = e.parameter_id
WHERE e.id = $1`

// readExperiment is the whole record of one experiment, as the API returns it.
func readExperiment(ctx context.Context, q querier, id int64) (contracts.Experiment, error) {
	exp, err := readHead(ctx, q, id)
	if err != nil {
		return contracts.Experiment{}, err
	}
	steps := []func() error{
		func() (err error) { exp.Arms, err = readArms(ctx, q, id, exp.Parameter); return },
		func() (err error) { exp.Blocks, err = readBlocks(ctx, q, id); return },
		func() (err error) { exp.Predictions, err = readPredictions(ctx, q, id); return },
		func() (err error) { exp.Results, err = readResults(ctx, q, id); return },
		func() (err error) { exp.Events, err = readEvents(ctx, q, id); return },
		func() (err error) { exp.Verdict, err = readVerdict(ctx, q, id); return },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return contracts.Experiment{}, fmt.Errorf("read experiment %d: %w", id, err)
		}
	}
	return exp, nil
}

func readHead(ctx context.Context, q querier, id int64) (contracts.Experiment, error) {
	var exp contracts.Experiment
	var test, guardrails, plan []byte
	var createdAt time.Time
	err := q.QueryRowContext(ctx, headSQL, id).Scan(&exp.Id, &exp.Kind, &exp.State, &exp.Statement,
		&exp.Source, &exp.CarName, &exp.TrackName, &exp.Parameter, &exp.ParameterLabel, &exp.Clicks,
		&test, &guardrails, &plan, &exp.Extensions, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return exp, fmt.Errorf("experiment %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return exp, fmt.Errorf("read experiment %d: %w", id, err)
	}
	exp.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	if err := json.Unmarshal(test, &exp.SuccessTest); err != nil {
		return exp, fmt.Errorf("decode success test of experiment %d: %w", id, err)
	}
	if err := json.Unmarshal(guardrails, &exp.Guardrails); err != nil {
		return exp, fmt.Errorf("decode guardrails of experiment %d: %w", id, err)
	}
	if plan != nil {
		exp.Plan = &contracts.PlanSummary{}
		if err := json.Unmarshal(plan, exp.Plan); err != nil {
			return exp, fmt.Errorf("decode plan of experiment %d: %w", id, err)
		}
	}
	return exp, nil
}

// Arms are listed by code name, so their order says nothing about which is the baseline.
func readArms(ctx context.Context, q querier, id int64, parameter string) ([]contracts.ExperimentArm, error) {
	return queryAll(ctx, q,
		`SELECT a.label_code, a.is_baseline, a.setup_version_id,
		        COALESCE((v."values"->>$2)::float8, 0)
		 FROM experiment_arm a JOIN setup_version v ON v.id = a.setup_version_id
		 WHERE a.experiment_id = $1 ORDER BY a.label_code`,
		func(rows *sql.Rows) (contracts.ExperimentArm, error) {
			var arm contracts.ExperimentArm
			return arm, rows.Scan(&arm.Label, &arm.IsBaseline, &arm.SetupVersionId, &arm.Value)
		}, id, parameter)
}

// A block's laps are counted from the passes tagged with it; lap 0 is the out-lap.
func readBlocks(ctx context.Context, q querier, id int64) ([]contracts.ExperimentBlock, error) {
	return queryAll(ctx, q,
		`SELECT b."order", a.label_code, b.laps_target, COALESCE(max(p.block_lap), 0), count(p.id)
		 FROM run_block b
		 JOIN experiment_arm a ON a.id = b.arm_id
		 LEFT JOIN corner_pass p ON p.block_id = b.id
		 WHERE b.experiment_id = $1 GROUP BY b.id, a.label_code ORDER BY b."order"`,
		func(rows *sql.Rows) (contracts.ExperimentBlock, error) {
			var block contracts.ExperimentBlock
			return block, rows.Scan(&block.Order, &block.Label, &block.LapsTarget, &block.LapsDone, &block.Passes)
		}, id)
}

func readPredictions(ctx context.Context, q querier, id int64) ([]contracts.PredictionSpec, error) {
	return queryAll(ctx, q,
		`SELECT channel, direction, low, high FROM prediction WHERE experiment_id = $1 ORDER BY id`,
		func(rows *sql.Rows) (contracts.PredictionSpec, error) {
			var prediction contracts.PredictionSpec
			return prediction, rows.Scan(&prediction.Channel, &prediction.Direction, &prediction.Low, &prediction.High)
		}, id)
}

func readResults(ctx context.Context, q querier, id int64) ([]contracts.ExperimentResult, error) {
	return queryAll(ctx, q,
		`SELECT channel, role, corner_class, estimate, ci_low, ci_high, n_passes, effect_class
		 FROM result WHERE experiment_id = $1 ORDER BY id`,
		func(rows *sql.Rows) (contracts.ExperimentResult, error) {
			var r contracts.ExperimentResult
			return r, rows.Scan(&r.Channel, &r.Role, &r.CornerClass, &r.Estimate, &r.CiLow,
				&r.CiHigh, &r.NPasses, &r.EffectClass)
		}, id)
}

func readEvents(ctx context.Context, q querier, id int64) ([]contracts.ExperimentEvent, error) {
	return queryAll(ctx, q,
		`SELECT from_state, to_state, reason, at FROM experiment_event
		 WHERE experiment_id = $1 ORDER BY id`,
		func(rows *sql.Rows) (contracts.ExperimentEvent, error) {
			var event contracts.ExperimentEvent
			var at time.Time
			err := rows.Scan(&event.From, &event.To, &event.Reason, &at)
			event.At = at.UTC().Format(time.RFC3339)
			return event, err
		}, id)
}

// readVerdict is the latest verdict: an extended experiment is judged more than once, and the
// driver's decision on a trade-off comes after the engine's.
func readVerdict(ctx context.Context, q querier, id int64) (*contracts.VerdictSummary, error) {
	var verdict contracts.VerdictSummary
	var decidedAt time.Time
	err := q.QueryRowContext(ctx,
		`SELECT decision, reason, decided_by, decided_at, prediction_hits, prediction_total
		 FROM verdict WHERE experiment_id = $1 ORDER BY id DESC LIMIT 1`, id).
		Scan(&verdict.Decision, &verdict.Reason, &verdict.DecidedBy, &decidedAt,
			&verdict.PredictionHits, &verdict.PredictionTotal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // no verdict yet is not an error
	}
	if err != nil {
		return nil, err
	}
	verdict.DecidedAt = decidedAt.UTC().Format(time.RFC3339)
	return &verdict, nil
}

// listIDs is every experiment, newest first.
func listIDs(ctx context.Context, q querier) ([]int64, error) {
	return queryAll(ctx, q, `SELECT id FROM experiment ORDER BY id DESC`,
		func(rows *sql.Rows) (int64, error) {
			var id int64
			return id, rows.Scan(&id)
		})
}
