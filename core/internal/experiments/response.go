package experiments

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The track and the changed item of an experiment, for the response analysis.
const responseContextSQL = `
SELECT t.length_m, t.corner_map, COALESCE(sp.role, ''), COALESCE(sp.axle, '')
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN track t ON t.id = h.track_id
LEFT JOIN setup_parameter sp ON sp.id = e.parameter_id
WHERE e.id = $1`

// ResponseRequest is what the engine's vehicle-response analysis (POST /response,
// docs/adr/0031-corner-phases-speed-regimes-and-response.md) needs to say how the experiment's
// variant changed the car: its push-lap passes on the arm's setup and from its own driver,
// with the covariates and lap codes the verdict uses, the corner map and where each session's
// samples are. An off-track pass is sent as not valid: never compared, but its samples may
// complete the previous pass's traction phase.
func (s *Service) ResponseRequest(ctx context.Context, id int64) (contracts.ResponseRequest, error) {
	request, err := responseContext(ctx, s.database, id)
	if err != nil {
		return request, err
	}
	rows, err := readPushPasses(ctx, s.database, id)
	if err != nil {
		return request, fmt.Errorf("read passes of experiment %d: %w", id, err)
	}
	var sessions []int
	request.Passes, sessions = responsePasses(rows)
	request.Sessions, err = parquetDirs(ctx, s.database, sessions)
	return request, err
}

func responseContext(ctx context.Context, q querier, id int64) (contracts.ResponseRequest, error) {
	request := contracts.ResponseRequest{Passes: []contracts.ResponsePass{}}
	var cornerMap []byte
	var role, axle string
	err := q.QueryRowContext(ctx, responseContextSQL, id).Scan(&request.TrackLengthM, &cornerMap, &role, &axle)
	if err != nil {
		return request, fmt.Errorf("read track of experiment %d: %w", id, err)
	}
	var corners []contracts.SpoolCorner
	if err := json.Unmarshal(cornerMap, &corners); err != nil {
		return request, fmt.Errorf("decode corner map of experiment %d: %w", id, err)
	}
	for _, corner := range corners {
		request.Corners = append(request.Corners, contracts.ResponseCorner{
			Number: corner.Number, ZoneStartM: corner.ZoneStartM, StartM: corner.StartM,
			EndM: corner.EndM, ZoneEndM: corner.ZoneEndM,
		})
	}
	if role != "" {
		request.ItemRole = &role
	}
	if axle != "" {
		request.ItemAxle = &axle
	}
	return request, nil
}

// responsePasses are the passes the verdict would weigh, plus the off-track ones as not valid,
// and the sessions they came from in order.
func responsePasses(rows []pushPass) ([]contracts.ResponsePass, []int) {
	var kept []pushPass
	for _, row := range rows {
		if row.onArmSetup && slices.Contains(sources[row.driver], row.source) {
			kept = append(kept, row)
		}
	}
	passes := []contracts.ResponsePass{}
	if len(kept) == 0 {
		return passes, nil
	}
	sessions := sessionOrder(kept)
	laps := newLapCodes()
	for _, row := range kept {
		arm := contracts.ResponseRequestPassesElemArmVariant
		if row.baseline {
			arm = contracts.ResponseRequestPassesElemArmBaseline
		}
		lap := laps.of(row.block, row.session, row.lap)
		passes = append(passes, contracts.ResponsePass{
			Session: row.session, Seq: row.seq, Corner: row.corner, Arm: arm, Block: row.block,
			Lap: &lap, TStartS: row.start, DurationS: row.duration, Valid: row.valid,
			Covariates: contracts.ResponseRequestPassesElemCovariates(covariates(row, kept[0].start, sessions)),
		})
	}
	return passes, sessions
}

func parquetDirs(ctx context.Context, q querier, sessions []int) ([]contracts.ResponseSession, error) {
	dirs := []contracts.ResponseSession{}
	for _, session := range sessions {
		var dir string
		err := q.QueryRowContext(ctx, `SELECT parquet_uri FROM session WHERE id = $1`, session).Scan(&dir)
		if err != nil {
			return nil, fmt.Errorf("read samples directory of session %d: %w", session, err)
		}
		if dir != "" {
			dirs = append(dirs, contracts.ResponseSession{Session: session, ParquetDir: dir})
		}
	}
	return dirs, nil
}
