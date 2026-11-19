package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// lapsPerBlockCode keeps laps of different blocks apart in the engine's lap numbers.
const lapsPerBlockCode = 1000

// lapKey identifies a lap that was driven: a block continued in a new session (the game was
// restarted) has more than one lap 1, and passes of different laps do not share their errors.
type lapKey struct{ block, session, lap int }

// lapCodes numbers the laps of each block 1, 2, ... in the order the passes arrive.
type lapCodes struct {
	codes    map[lapKey]int
	perBlock map[int]int
}

func newLapCodes() *lapCodes {
	return &lapCodes{codes: map[lapKey]int{}, perBlock: map[int]int{}}
}

func (l *lapCodes) of(block, session, lap int) int {
	key := lapKey{block, session, lap}
	if _, known := l.codes[key]; !known {
		l.perBlock[block]++
		l.codes[key] = block*lapsPerBlockCode + l.perBlock[block]
	}
	return l.codes[key]
}

// sources are the session sources whose passes count for an experiment's driver: the human's
// own sessions (and recorded demo sessions), or the AI's.
var sources = map[string][]string{"human": {"human", "demo"}, "ai": {"ai", "ghost"}}

// pushPass is one push-lap pass of an experiment's blocks, as the ledger has it.
type pushPass struct {
	corner, block, session, lap int
	baseline, valid, onArmSetup bool
	source, driver              string
	speedClass                  sql.NullString
	metrics                     []byte
	fuel, tyreCore, ersStart    sql.NullFloat64
	start                       float64 // seconds since the epoch
}

// Push laps only: lap 0 of a block is the out-lap, on cold tyres, and is never compared.
const pushPassesSQL = `
SELECT c.number, a.is_baseline, b."order", p.session_id, p.block_lap, p.features->>'speed_class',
       p.features->'metrics', p.fuel_l,
       (SELECT avg(core) FROM unnest(p.tyre_core_c) AS core),
       EXTRACT(EPOCH FROM p.t_start)::float8, p.ers_charge_start, p.valid,
       p.setup_version_id IS NOT DISTINCT FROM a.setup_version_id, s.source, e.driver
FROM corner_pass p
JOIN run_block b ON b.id = p.block_id
JOIN experiment e ON e.id = b.experiment_id
JOIN experiment_arm a ON a.id = b.arm_id
JOIN corner c ON c.id = p.corner_id
JOIN session s ON s.id = p.session_id
WHERE b.experiment_id = $1 AND p.block_lap >= 1
ORDER BY p.t_start, p.id`

func readPushPasses(ctx context.Context, q querier, id int64) ([]pushPass, error) {
	return queryAll(ctx, q, pushPassesSQL, func(rows *sql.Rows) (pushPass, error) {
		var p pushPass
		return p, rows.Scan(&p.corner, &p.baseline, &p.block, &p.session, &p.lap, &p.speedClass,
			&p.metrics, &p.fuel, &p.tyreCore, &p.start, &p.ersStart, &p.valid, &p.onArmSetup,
			&p.source, &p.driver)
	}, id)
}

// comparison is what the engine is sent about an experiment's passes (ADR 0029): the valid
// passes on the arm's setup and from the experiment's own driver, the passes left out for either
// reason, and per block how many push-lap passes went off track. pending counts the compared
// passes whose features the engine has not computed yet.
type comparison struct {
	passes   []contracts.EvaluatePass
	pending  int
	excluded contracts.ExcludedPasses
	offtrack []contracts.OfftrackBlock
}

// compare sorts the push passes. A pass on another setup is never compared, whatever else is
// true of it; nor is one from a session someone else drove. An off-track pass is counted against
// its block, not compared.
func compare(rows []pushPass) (comparison, error) {
	var out comparison
	blocks := map[int]*contracts.OfftrackBlock{}
	var kept []pushPass
	for _, row := range rows {
		switch {
		case !row.onArmSetup:
			out.excluded.SetupMismatch++
		case !slices.Contains(sources[row.driver], row.source):
			out.excluded.SourceMismatch++
		default:
			countOfftrack(blocks, row)
			if row.valid {
				kept = append(kept, row)
			}
		}
	}
	var err error
	out.passes, out.pending, err = evaluatePasses(kept)
	for _, order := range slices.Sorted(maps.Keys(blocks)) {
		out.offtrack = append(out.offtrack, *blocks[order])
	}
	return out, err
}

func countOfftrack(blocks map[int]*contracts.OfftrackBlock, row pushPass) {
	block, known := blocks[row.block]
	if !known {
		arm := contracts.OfftrackBlockArmVariant
		if row.baseline {
			arm = contracts.OfftrackBlockArmBaseline
		}
		block = &contracts.OfftrackBlock{Block: row.block, Arm: arm}
		blocks[row.block] = block
	}
	block.Passes++
	if !row.valid {
		block.Offtrack++
	}
}

// evaluatePasses turns compared passes into the engine's, with lap codes, time since the first
// of them and a session term for every game session after the first.
func evaluatePasses(rows []pushPass) ([]contracts.EvaluatePass, int, error) {
	passes, pending := []contracts.EvaluatePass{}, 0
	if len(rows) == 0 {
		return passes, 0, nil
	}
	sessions := sessionOrder(rows)
	laps := newLapCodes()
	for _, row := range rows {
		if row.metrics == nil {
			pending++
			continue
		}
		pass := contracts.EvaluatePass{Corner: row.corner, Block: row.block, Arm: contracts.EvaluatePassArmVariant,
			Covariates: covariates(row, rows[0].start, sessions)}
		if row.baseline {
			pass.Arm = contracts.EvaluatePassArmBaseline
		}
		lapCode := laps.of(row.block, row.session, row.lap)
		pass.Lap = &lapCode
		if row.speedClass.Valid {
			class := contracts.EvaluatePassSpeedClass(row.speedClass.String)
			pass.SpeedClass = &class
		}
		if err := json.Unmarshal(row.metrics, &pass.Metrics); err != nil {
			return nil, 0, fmt.Errorf("metrics of a pass at corner %d: %w", row.corner, err)
		}
		passes = append(passes, pass)
	}
	return passes, pending, nil
}

// sessionOrder lists the game sessions in the order the experiment's passes first came from them.
func sessionOrder(rows []pushPass) []int {
	var order []int
	for _, row := range rows {
		if !slices.Contains(order, row.session) {
			order = append(order, row.session)
		}
	}
	return order
}

// covariates are the conditions the engine adjusts for. One that was not recorded is left out;
// the engine only uses covariates every pass has. A car without a hybrid battery has no battery
// charge (docs/adr/0024-ers-state-per-pass.md). Time runs from the experiment's first compared
// pass, and session_<n> is 1 on the n-th game session's passes: a restart resets the game, not
// the experiment's clock (docs/adr/0029-block-level-inference-and-finding-classes.md).
func covariates(row pushPass, start float64, sessions []int) contracts.EvaluatePassCovariates {
	out := contracts.EvaluatePassCovariates{"time_since_start_s": row.start - start}
	if row.fuel.Valid {
		out["fuel_l"] = row.fuel.Float64
	}
	if row.tyreCore.Valid {
		out["tyre_core_c"] = row.tyreCore.Float64
	}
	if row.ersStart.Valid {
		out["ers_charge_start"] = row.ersStart.Float64
	}
	for i, session := range sessions[1:] {
		term := 0.0
		if row.session == session {
			term = 1
		}
		out[fmt.Sprintf("session_%d", i+2)] = term
	}
	return out
}

// readComparison is the experiment's comparison as the ledger has it now.
func readComparison(ctx context.Context, q querier, id int64) (comparison, error) {
	rows, err := readPushPasses(ctx, q, id)
	if err != nil {
		return comparison{}, fmt.Errorf("read passes of experiment %d: %w", id, err)
	}
	out, err := compare(rows)
	if err != nil {
		return comparison{}, fmt.Errorf("passes of experiment %d: %w", id, err)
	}
	return out, nil
}
