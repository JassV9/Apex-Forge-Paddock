package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const insertPassSQL = `
INSERT INTO corner_pass (
    session_id, corner_id, seq, t_start, duration_s, lap_in_session, setup_hash, n_samples,
    fuel_l, tyre_core_c, tyre_surface_c, tyre_press, wear, grip, air_c, road_c, features
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (session_id, corner_id, t_start) DO NOTHING`

// passFeatures is corner_pass.features as ingest writes it. The engine adds its own keys.
type passFeatures struct {
	Legacy *contracts.LegacySummary `json:"legacy,omitempty"`
}

// insertPass reports whether the row is new (false: this pass was already stored).
func insertPass(ctx context.Context, tx *sql.Tx, sessionID, cornerID int64, pass *contracts.SpoolPass) (bool, error) {
	features, err := json.Marshal(passFeatures{Legacy: pass.Legacy})
	if err != nil {
		return false, fmt.Errorf("encode features: %w", err)
	}
	samples := 0
	if pass.Samples != nil {
		samples = pass.Samples.N
	}
	args := []any{
		sessionID, cornerID, pass.Seq, time.UnixMilli(int64(pass.TStartMs)).UTC(), pass.DurationS,
		optional(pass.LapInSession), optional(pass.SetupHash), samples,
	}
	args = append(args, conditionArgs(pass.Conditions)...)
	args = append(args, string(features))
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

// optional turns a nil pointer into SQL NULL.
func optional[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
