package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The pass is linked to the setup version it was driven on (by the agent's fingerprint) and
// to the run block of an experiment, when the agent tagged it with one.
const insertPassSQL = `
INSERT INTO corner_pass (
    session_id, corner_id, seq, t_start, duration_s, lap_in_session, setup_hash, n_samples,
    fuel_l, tyre_core_c, tyre_surface_c, tyre_press, wear, grip, air_c, road_c, features,
    setup_version_id, block_id, block_lap
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
    (SELECT id FROM setup_version WHERE car_id = $18 AND agent_hash = $7 ORDER BY id DESC LIMIT 1),
    (SELECT id FROM run_block WHERE experiment_id = $19 AND "order" = $20),
    $21)
ON CONFLICT (session_id, corner_id, t_start) DO NOTHING`

// passFeatures is corner_pass.features as ingest writes it. The engine adds its own keys.
type passFeatures struct {
	Legacy *contracts.LegacySummary `json:"legacy,omitempty"`
}

// insertPass reports whether the row is new (false: this pass was already stored).
func insertPass(ctx context.Context, tx *sql.Tx, session sessionRef, cornerID int64, pass *contracts.SpoolPass) (bool, error) {
	features, err := json.Marshal(passFeatures{Legacy: pass.Legacy})
	if err != nil {
		return false, fmt.Errorf("encode features: %w", err)
	}
	samples := 0
	if pass.Samples != nil {
		samples = pass.Samples.N
	}
	args := []any{
		session.id, cornerID, pass.Seq, time.UnixMilli(int64(pass.TStartMs)).UTC(), pass.DurationS,
		optional(pass.LapInSession), optional(pass.SetupHash), samples,
	}
	args = append(args, conditionArgs(pass.Conditions)...)
	args = append(args, string(features), session.carID)
	args = append(args, blockArgs(pass.Block)...)
	outcome, err := tx.ExecContext(ctx, insertPassSQL, args...)
	if err != nil {
		return false, err
	}
	affected, err := outcome.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return affected == 1, nil
}

// conditionArgs are the eight condition columns, all NULL when the pass has no conditions
// (data imported from Apex Forge v0 recorded none).
func conditionArgs(c *contracts.PassConditions) []any {
	if c == nil {
		return make([]any, 8)
	}
	return []any{
		c.FuelL, c.TyreCoreC, c.TyreSurfaceC, c.TyrePressPsi, c.TyreWear, c.Grip, c.AirC, c.RoadC,
	}
}

// blockArgs are experiment id, block order and lap in the block; all NULL outside experiments.
func blockArgs(block *contracts.PassBlock) []any {
	if block == nil {
		return make([]any, 3)
	}
	return []any{block.ExperimentId, block.Order, block.Lap}
}

// optional turns a nil pointer into SQL NULL.
func optional[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
