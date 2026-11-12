package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// planRequest is what the engine needs to say what a run plan can detect: the track's
// corners and how much the primary channel scattered on earlier passes of the baseline.
func planRequest(ctx context.Context, q querier, exp record, lapsPerBlock int) (contracts.PlanRequest, error) {
	request := contracts.PlanRequest{SuccessTest: exp.test, LapsPerBlock: lapsPerBlock}
	corners, err := queryAll(ctx, q, `SELECT number FROM corner WHERE track_id = $1 ORDER BY number`,
		func(rows *sql.Rows) (int, error) {
			var number int
			return number, rows.Scan(&number)
		}, exp.trackID)
	if err != nil {
		return request, fmt.Errorf("read corners of track %d: %w", exp.trackID, err)
	}
	request.Corners = corners
	request.BaselinePasses, err = queryAll(ctx, q,
		`SELECT c.number, (p.features->'metrics'->>$4)::float8
		 FROM corner_pass p
		 JOIN corner c ON c.id = p.corner_id
		 JOIN session s ON s.id = p.session_id
		 WHERE s.car_id = $1 AND s.track_id = $2 AND p.setup_version_id = $3 AND p.valid
		   AND p.features->'metrics'->>$4 IS NOT NULL`,
		func(rows *sql.Rows) (contracts.PlanPass, error) {
			var pass contracts.PlanPass
			return pass, rows.Scan(&pass.Corner, &pass.Value)
		}, exp.carID, exp.trackID, exp.baselineVersion, exp.test.Channel)
	if err != nil {
		return request, fmt.Errorf("read baseline passes of experiment %d: %w", exp.id, err)
	}
	return request, nil
}

// savePlan stores the engine's run plan: its blocks and what it can answer.
func savePlan(ctx context.Context, tx *sql.Tx, id int64, plan contracts.PlanResponse) error {
	summary, err := json.Marshal(contracts.PlanSummary{
		Sigma: plan.Sigma, SigmaSource: contracts.PlanSummarySigmaSource(plan.SigmaSource),
		PassesPerArm: plan.PassesPerArm, MdeDetect: plan.MdeDetect, MdeDecide: plan.MdeDecide,
	})
	if err != nil {
		return fmt.Errorf("encode plan summary: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE experiment SET mde = $2 WHERE id = $1`, id, string(summary)); err != nil {
		return fmt.Errorf("save plan of experiment %d: %w", id, err)
	}
	for _, block := range plan.Blocks {
		baseline := block.Arm == contracts.PlanResponseBlocksElemArmBaseline
		if err := addBlock(ctx, tx, id, baseline, block.LapsTarget, "queued"); err != nil {
			return err
		}
	}
	return nil
}

// addBlock appends a block to the experiment's run plan, on the baseline or on the variant.
func addBlock(ctx context.Context, tx *sql.Tx, id int64, baseline bool, laps int, state string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO run_block (experiment_id, arm_id, "order", laps_target, state)
		 SELECT $1, a.id,
		        (SELECT COALESCE(max("order"), 0) + 1 FROM run_block WHERE experiment_id = $1),
		        $3, $4
		 FROM experiment_arm a WHERE a.experiment_id = $1 AND a.is_baseline = $2`,
		id, baseline, laps, state)
	if err != nil {
		return fmt.Errorf("add block to experiment %d: %w", id, err)
	}
	return nil
}

// addRestoreBlock queues the baseline to go back on the car: a block with no laps to drive.
// The run plan changes, so its revision goes up.
func addRestoreBlock(ctx context.Context, tx *sql.Tx, id int64) error {
	if err := addBlock(ctx, tx, id, true, 0, "restore"); err != nil {
		return err
	}
	return bumpRevision(ctx, tx, id)
}

// bumpRevision marks the run plan as changed; the agent reloads a plan with a new revision.
func bumpRevision(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE experiment SET plan_revision = plan_revision + 1 WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("bump plan revision of experiment %d: %w", id, err)
	}
	return nil
}

// extend adds one more baseline block and one more variant block, as long as the first.
func extend(ctx context.Context, tx *sql.Tx, exp record) error {
	if exp.extensions >= MaxExtensions {
		return fmt.Errorf("experiment %d was already extended %d times: %w", exp.id, MaxExtensions, ErrConflict)
	}
	var laps int
	err := tx.QueryRowContext(ctx,
		`SELECT laps_target FROM run_block WHERE experiment_id = $1 AND "order" = 1`, exp.id).Scan(&laps)
	if err != nil {
		return fmt.Errorf("read block length of experiment %d: %w", exp.id, err)
	}
	for _, baseline := range []bool{true, false} {
		if err := addBlock(ctx, tx, exp.id, baseline, laps, "queued"); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE experiment SET extensions = extensions + 1 WHERE id = $1`, exp.id)
	if err != nil {
		return fmt.Errorf("count extension of experiment %d: %w", exp.id, err)
	}
	return bumpRevision(ctx, tx, exp.id)
}

// otherOccupying is the id of another experiment that has its setups on the car, or 0.
func otherOccupying(ctx context.Context, q querier, id int64) (int64, error) {
	states := make([]string, len(occupying))
	for i, state := range occupying {
		states[i] = string(state)
	}
	var other sql.NullInt64
	err := q.QueryRowContext(ctx,
		`SELECT min(id) FROM experiment WHERE id <> $1 AND state = ANY($2)`, id, states).Scan(&other)
	if err != nil {
		return 0, fmt.Errorf("look for a running experiment: %w", err)
	}
	return other.Int64, nil
}
