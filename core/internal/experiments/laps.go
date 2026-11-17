package experiments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// quietAfter is how long an experiment may go without a new pass before the push lap the car
// was on counts as finished anyway: the agent stopped (car stuck three times, game closed). It
// is longer than the agent's lap watchdog (ghost LAP_TIMEOUT_S, 10 min), after which the AI
// retries the block and the retry's out-lap passes arrive within a couple of minutes.
const quietAfter = 15 * time.Minute

// lapAttempt is one drive of a push lap: the passes of one block lap in one session and game
// lap, and how many corners of the track they reached. A lap re-driven after a restart or a
// retry is another attempt of the same block lap.
type lapAttempt struct{ lap, corners int }

// finishedLaps is how many push laps of a block the car has finished (ADR 0025). The agent
// numbers the laps 1, 2, ... and tags the next lap only after the line, so every lap below the
// highest one is finished. The highest is finished when one of its attempts reached every
// corner of the track, when a later block has passes (the agent moved on only after the line),
// or when nothing arrived for quietAfter.
func finishedLaps(attempts []lapAttempt, trackCorners int, movedOn, quiet bool) int {
	top, covered := 0, false
	for _, attempt := range attempts {
		if attempt.lap > top {
			top, covered = attempt.lap, false
		}
		if attempt.lap == top && attempt.corners >= trackCorners {
			covered = true
		}
	}
	if top == 0 || covered || movedOn || quiet {
		return top
	}
	return top - 1
}

// lapEvidence is what the laps of an experiment's blocks are counted from.
type lapEvidence struct {
	attempts     map[int][]lapAttempt // block order -> push lap attempts
	trackCorners int
	quiet        bool
}

const lapAttemptsSQL = `
SELECT b."order", p.block_lap, count(DISTINCT p.corner_id)
FROM corner_pass p JOIN run_block b ON b.id = p.block_id
WHERE b.experiment_id = $1 AND p.block_lap >= 1
GROUP BY b."order", p.block_lap, p.session_id, p.lap_in_session`

const lapContextSQL = `
SELECT (SELECT count(*) FROM corner c WHERE c.track_id = h.track_id),
       COALESCE((SELECT now() - max(p.t_start + p.duration_s * interval '1 second')
                 FROM corner_pass p JOIN run_block b ON b.id = p.block_id
                 WHERE b.experiment_id = e.id) > $2 * interval '1 second', false)
FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
WHERE e.id = $1`

func readLapEvidence(ctx context.Context, q querier, id int64) (lapEvidence, error) {
	evidence := lapEvidence{attempts: map[int][]lapAttempt{}}
	err := q.QueryRowContext(ctx, lapContextSQL, id, quietAfter.Seconds()).
		Scan(&evidence.trackCorners, &evidence.quiet)
	if err != nil {
		return evidence, fmt.Errorf("read corners and last pass of experiment %d: %w", id, err)
	}
	type blockAttempt struct {
		order   int
		attempt lapAttempt
	}
	rows, err := queryAll(ctx, q, lapAttemptsSQL, func(rows *sql.Rows) (blockAttempt, error) {
		var row blockAttempt
		return row, rows.Scan(&row.order, &row.attempt.lap, &row.attempt.corners)
	}, id)
	if err != nil {
		return evidence, fmt.Errorf("read laps of experiment %d: %w", id, err)
	}
	for _, row := range rows {
		evidence.attempts[row.order] = append(evidence.attempts[row.order], row.attempt)
	}
	return evidence, nil
}

// countLaps fills in the finished push laps of every block, in plan order.
func countLaps(blocks []contracts.ExperimentBlock, evidence lapEvidence) {
	for i := range blocks {
		movedOn := false
		for _, later := range blocks[i+1:] {
			movedOn = movedOn || later.Passes > 0
		}
		blocks[i].LapsDone = finishedLaps(evidence.attempts[blocks[i].Order], evidence.trackCorners,
			movedOn, evidence.quiet)
	}
}
