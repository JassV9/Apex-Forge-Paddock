package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// lapsPerBlockCode keeps laps of different blocks apart in the engine's lap numbers.
const lapsPerBlockCode = 1000

// Push laps only: lap 0 of a block is the out-lap, on cold tyres, and is never compared.
const blockPassesSQL = `
SELECT c.number, a.is_baseline, b."order", p.block_lap, p.features->>'speed_class',
       p.features->'metrics', p.fuel_l,
       (SELECT avg(core) FROM unnest(p.tyre_core_c) AS core),
       EXTRACT(EPOCH FROM p.t_start - s.started_at)::float8
FROM corner_pass p
JOIN run_block b ON b.id = p.block_id
JOIN experiment_arm a ON a.id = b.arm_id
JOIN corner c ON c.id = p.corner_id
JOIN session s ON s.id = p.session_id
WHERE b.experiment_id = $1 AND p.valid AND p.block_lap >= 1
ORDER BY p.t_start, p.id`

// blockPasses are the experiment's comparable passes, and how many of them still wait for
// the engine to compute their features.
func blockPasses(ctx context.Context, q querier, id int64) ([]contracts.EvaluatePass, int, error) {
	pending := 0
	passes, err := queryAll(ctx, q, blockPassesSQL, func(rows *sql.Rows) (contracts.EvaluatePass, error) {
		pass := contracts.EvaluatePass{Covariates: contracts.EvaluatePassCovariates{}}
		var baseline bool
		var lap int
		var speedClass sql.NullString
		var metrics []byte
		var fuel, tyreCore sql.NullFloat64
		var elapsed float64
		err := rows.Scan(&pass.Corner, &baseline, &pass.Block, &lap, &speedClass, &metrics,
			&fuel, &tyreCore, &elapsed)
		if err != nil {
			return pass, err
		}
		if metrics == nil {
			pending++
			return pass, nil
		}
		pass.Arm = contracts.EvaluatePassArmVariant
		if baseline {
			pass.Arm = contracts.EvaluatePassArmBaseline
		}
		lapCode := pass.Block*lapsPerBlockCode + lap
		pass.Lap = &lapCode
		if speedClass.Valid {
			class := contracts.EvaluatePassSpeedClass(speedClass.String)
			pass.SpeedClass = &class
		}
		addCovariates(pass.Covariates, fuel, tyreCore, elapsed)
		return pass, json.Unmarshal(metrics, &pass.Metrics)
	}, id)
	if err != nil {
		return nil, 0, fmt.Errorf("read passes of experiment %d: %w", id, err)
	}
	return passes, pending, nil
}

// addCovariates fills in the conditions the engine adjusts for. One that was not recorded is
// left out; the engine only uses covariates every pass has.
func addCovariates(covariates contracts.EvaluatePassCovariates, fuel, tyreCore sql.NullFloat64, elapsed float64) {
	covariates["time_in_session_s"] = elapsed
	if fuel.Valid {
		covariates["fuel_l"] = fuel.Float64
	}
	if tyreCore.Valid {
		covariates["tyre_core_c"] = tyreCore.Float64
	}
}

// saveResults replaces the experiment's measured effects: an extended experiment is judged
// again on all its passes, and only the latest numbers are the result.
func saveResults(ctx context.Context, tx *sql.Tx, id int64, results []contracts.ChannelResult) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM result WHERE experiment_id = $1`, id); err != nil {
		return fmt.Errorf("clear results of experiment %d: %w", id, err)
	}
	for _, r := range results {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO result (experiment_id, channel, role, corner_class, estimate, ci_low,
			                     ci_high, n_passes, effect_class)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, r.Channel, string(r.Role), string(r.CornerClass), r.Estimate, r.CiLow, r.CiHigh,
			r.NPasses, string(r.EffectClass))
		if err != nil {
			return fmt.Errorf("save result on %s: %w", r.Channel, err)
		}
	}
	return nil
}

// verdict is one decision about an experiment, by the engine or by the driver.
type verdict struct {
	decision  contracts.VerdictSummaryDecision
	reason    string
	hits      int
	total     int
	decidedBy contracts.VerdictSummaryDecidedBy
}

// addVerdict appends a verdict. Verdicts are never changed: the driver's call on a trade-off
// is a new row after the engine's.
func addVerdict(ctx context.Context, tx *sql.Tx, id int64, v verdict) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO verdict (experiment_id, decision, reason, prediction_hits, prediction_total,
		                      decided_by, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, clock_timestamp())`,
		id, string(v.decision), v.reason, v.hits, v.total, string(v.decidedBy))
	if err != nil {
		return fmt.Errorf("save verdict of experiment %d: %w", id, err)
	}
	return nil
}

// driverVerdict records a decision the driver made; the prediction score of the engine's
// last verdict is carried over, so it stays on the latest row.
func driverVerdict(ctx context.Context, tx *sql.Tx, id int64, decision contracts.VerdictSummaryDecision, reason string) error {
	last, err := readVerdict(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("read last verdict of experiment %d: %w", id, err)
	}
	v := verdict{decision: decision, reason: reason, decidedBy: contracts.VerdictSummaryDecidedByDriver}
	if last != nil {
		v.hits, v.total = last.PredictionHits, last.PredictionTotal
	}
	return addVerdict(ctx, tx, id, v)
}
