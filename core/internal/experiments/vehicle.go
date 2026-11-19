package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// vehiclePasses are the push passes the engine measures the car's response on
// (docs/adr/0034-vehicle-response-in-verdicts.md): those on the arm's setup and from the
// experiment's driver, off-track ones included, since a pass that went wide may still carry the
// previous pass's traction. Lap codes and covariates are those of the compared passes.
func vehiclePasses(rows []pushPass, laps *lapCodes, start float64, sessions []int) []contracts.VehiclePass {
	passes := []contracts.VehiclePass{}
	for _, row := range rows {
		lap := laps.of(row.block, row.session, row.lap)
		pass := contracts.VehiclePass{Session: row.session, Seq: row.seq, Corner: row.corner,
			Arm: contracts.VehiclePassArmVariant, Block: row.block, Lap: &lap, TStartS: row.start,
			DurationS: row.duration, Valid: row.valid,
			Covariates: contracts.VehiclePassCovariates(covariates(row, start, sessions))}
		if row.baseline {
			pass.Arm = contracts.VehiclePassArmBaseline
		}
		passes = append(passes, pass)
	}
	return passes
}

// The experiment's track, and the changed item's role and axle in the catalog (empty for an
// experiment that moves no single item).
const vehicleTrackSQL = `
SELECT t.length_m, t.corner_map, COALESCE(sp.role, ''), COALESCE(sp.axle, '')
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN track t ON t.id = h.track_id
LEFT JOIN setup_parameter sp ON sp.id = e.parameter_id
WHERE e.id = $1`

// vehicleInput is the response context of an evaluate request, or nil when the experiment moves
// no single item (nothing is expected to move) or no pass has raw samples to read.
func vehicleInput(ctx context.Context, q querier, id int64, passes []contracts.VehiclePass) (*contracts.VehicleResponseInput, error) {
	var input contracts.VehicleResponseInput
	var cornerMap []byte
	var role, axle string
	err := q.QueryRowContext(ctx, vehicleTrackSQL, id).Scan(&input.TrackLengthM, &cornerMap, &role, &axle)
	if err != nil {
		return nil, fmt.Errorf("read track of experiment %d: %w", id, err)
	}
	if role == "" || len(passes) == 0 {
		return nil, nil
	}
	input.ItemRole, input.ItemAxle, input.Passes = &role, &axle, passes
	if input.Corners, err = vehicleCorners(cornerMap); err != nil {
		return nil, fmt.Errorf("corner map of experiment %d: %w", id, err)
	}
	if input.Sessions, err = vehicleSessions(ctx, q, passes); err != nil {
		return nil, fmt.Errorf("sessions of experiment %d: %w", id, err)
	}
	if len(input.Sessions) == 0 {
		return nil, nil
	}
	return &input, nil
}

func vehicleCorners(cornerMap []byte) ([]contracts.VehicleCorner, error) {
	var corners []contracts.SpoolCorner
	if err := json.Unmarshal(cornerMap, &corners); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out := []contracts.VehicleCorner{}
	for _, corner := range corners {
		out = append(out, contracts.VehicleCorner{Number: corner.Number, ZoneStartM: corner.ZoneStartM,
			StartM: corner.StartM, EndM: corner.EndM, ZoneEndM: corner.ZoneEndM})
	}
	return out, nil
}

// vehicleSessions are the sessions of the passes whose raw samples were written to Parquet.
func vehicleSessions(ctx context.Context, q querier, passes []contracts.VehiclePass) ([]contracts.VehicleSession, error) {
	ids := []int64{}
	for _, pass := range passes {
		if !slices.Contains(ids, int64(pass.Session)) {
			ids = append(ids, int64(pass.Session))
		}
	}
	return queryAll(ctx, q,
		`SELECT id, parquet_uri FROM session
		 WHERE id = ANY($1) AND parquet_uri <> ''
		   AND EXISTS (SELECT 1 FROM corner_pass p WHERE p.session_id = session.id AND p.n_samples > 0)
		 ORDER BY id`,
		func(rows *sql.Rows) (contracts.VehicleSession, error) {
			var s contracts.VehicleSession
			return s, rows.Scan(&s.Session, &s.ParquetDir)
		}, ids)
}
