package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// RunPlanPath is where the in-game agent reads the run plan of a car: one file per car, so an
// experiment waiting on one car does not stop another car's (docs/adr/0014-ghost-driver.md).
func RunPlanPath(dir, carACID string) string {
	return filepath.Join(dir, "runplans", setups.SafeName(carACID)+".json")
}

// Outbox is the directory core writes for the game: runplans/<car>.json, which the agent
// reads, and a setup file per arm under setups/<car>/. It is core's own directory. Core never writes
// into Assetto Corsa's setups folder, so it cannot overwrite a setup the driver saved.
type Outbox struct {
	// Dir is the directory; empty turns writing off (the API and the ledger still work).
	Dir string
}

// armSetup is one arm's setup as it goes into the outbox.
type armSetup struct {
	label  string
	values setups.Values
}

// Write puts the run plan and its setup files into the outbox. The plan is written to a
// temporary file and renamed, so the agent never reads half a plan.
func (o Outbox) Write(plan contracts.RunPlan, carACID string, arms []armSetup) error {
	if o.Dir == "" {
		return nil
	}
	setupDir := filepath.Join(o.Dir, "setups", setups.SafeName(carACID))
	for _, arm := range arms {
		stem := fmt.Sprintf("paddock-e%d-%s", plan.ExperimentId, arm.label)
		description := fmt.Sprintf("Experiment %d, %s", plan.ExperimentId, arm.label)
		_, err := setups.WriteIni(setupDir, stem, carACID, arm.values, description)
		if err != nil && !errors.Is(err, setups.ErrFileExists) {
			return fmt.Errorf("write setup of %s: %w", arm.label, err)
		}
	}
	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run plan: %w", err)
	}
	path := RunPlanPath(o.Dir, carACID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create outbox: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0o640); err != nil { //nolint:gosec // the agent, another user's process, must read it
		return fmt.Errorf("write run plan: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish run plan: %w", err)
	}
	return nil
}

const runPlanSQL = `
SELECT b."order", a.label_code, b.laps_target, v."values"
FROM run_block b
JOIN experiment_arm a ON a.id = b.arm_id
JOIN setup_version v ON v.id = a.setup_version_id
WHERE b.experiment_id = $1 ORDER BY b."order"`

// readRunPlan builds the experiment's run plan as the agent reads it, and the setup of each
// arm. Blocks carry code names only: the plan does not say which setup is the baseline.
func readRunPlan(ctx context.Context, q querier, id int64) (contracts.RunPlan, []armSetup, error) {
	plan := contracts.RunPlan{V: 1, ExperimentId: int(id)}
	err := q.QueryRowContext(ctx, `SELECT plan_revision, driver FROM experiment WHERE id = $1`, id).
		Scan(&plan.Revision, &plan.Driver)
	if err != nil {
		return plan, nil, fmt.Errorf("read plan revision of experiment %d: %w", id, err)
	}
	blocks, err := queryAll(ctx, q, runPlanSQL, func(rows *sql.Rows) (contracts.RunPlanBlock, error) {
		var block contracts.RunPlanBlock
		var values []byte
		if err := rows.Scan(&block.Order, &block.Label, &block.LapsTarget, &values); err != nil {
			return block, err
		}
		return block, json.Unmarshal(values, &block.Values)
	}, id)
	if err != nil {
		return plan, nil, fmt.Errorf("read run plan of experiment %d: %w", id, err)
	}
	plan.Blocks = blocks
	var restore sql.NullInt64
	err = q.QueryRowContext(ctx,
		`SELECT max("order") FROM run_block WHERE experiment_id = $1 AND state = 'restore'`, id).Scan(&restore)
	if err != nil {
		return plan, nil, fmt.Errorf("read restore block of experiment %d: %w", id, err)
	}
	if restore.Valid {
		from := int(restore.Int64)
		plan.FromOrder = &from
	}
	seen := map[string]bool{}
	var arms []armSetup
	for _, block := range blocks {
		if !seen[block.Label] {
			seen[block.Label] = true
			arms = append(arms, armSetup{label: block.Label, values: setups.Values(block.Values)})
		}
	}
	return plan, arms, nil
}
