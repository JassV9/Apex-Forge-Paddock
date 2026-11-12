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

// RunPlanFile is the file the in-game agent reads its run plan from.
const RunPlanFile = "runplan.json"

// Outbox is the directory core writes for the game: runplan.json, which the agent reads, and
// a setup file per arm under setups/<car>/. It is core's own directory. Core never writes
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
	if err := os.MkdirAll(o.Dir, 0o750); err != nil {
		return fmt.Errorf("create outbox: %w", err)
	}
	temporary := filepath.Join(o.Dir, RunPlanFile+".tmp")
	if err := os.WriteFile(temporary, encoded, 0o640); err != nil { //nolint:gosec // the agent, another user's process, must read it
		return fmt.Errorf("write run plan: %w", err)
	}
	if err := os.Rename(temporary, filepath.Join(o.Dir, RunPlanFile)); err != nil {
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
	err := q.QueryRowContext(ctx, `SELECT plan_revision FROM experiment WHERE id = $1`, id).Scan(&plan.Revision)
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
