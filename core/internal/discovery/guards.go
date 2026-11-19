package discovery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// stallAfter is how long a step may go without a pass, an event or its start before the sweep
// calls it an AI failure. It is twice the time after which a lap counts as finished without a
// pass (experiments' quietAfter, 15 min), which is itself longer than the agent's lap watchdog
// (10 min) and a retry of the block.
const stallAfter = 30 * time.Minute

// hintTolerance is how far a pass's AI tyre hint may be from the car's hint times the pinned
// share: the spool writes hints to 4 decimals.
const hintTolerance = 0.0005

// A block of the experiment driven at a hint other than the pinned one, from the run context
// the agent reports (docs/adr/0028-run-context-and-ai-provenance.md): the block's lowest and
// highest hint against the session's base hint times the share. A block or session without a
// recorded hint says nothing either way.
const hintOffSQL = `
SELECT b."order", rc.ai_hint_min, rc.ai_hint_max, base.ai_tyre_hint_base
FROM run_context rc
JOIN run_block b ON b.id = rc.block_id
JOIN run_context base ON base.session_id = rc.session_id AND base.block_id IS NULL
WHERE b.experiment_id = $1 AND rc.ai_hint_min IS NOT NULL AND base.ai_tyre_hint_base IS NOT NULL
  AND (abs(rc.ai_hint_min - base.ai_tyre_hint_base * $2) > $3
       OR abs(rc.ai_hint_max - base.ai_tyre_hint_base * $2) > $3)
ORDER BY b."order" LIMIT 1`

// hintOff says, in words, which block of the experiment was driven at another AI hint than
// the sweep pins; "" when none was. Experiment C of the investigation: never mix hints.
func (s *Service) hintOff(ctx context.Context, experimentID int64, share float64) (string, error) {
	var block int
	var low, high, base float64
	err := s.database.QueryRowContext(ctx, hintOffSQL, experimentID, share, hintTolerance).
		Scan(&block, &low, &high, &base)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check the AI hint of experiment %d: %w", experimentID, err)
	}
	return fmt.Sprintf("block %d was driven at AI tyre hints %.4f to %.4f, the sweep pins %.4f (%.4f of the car's %.4f)",
		block, low, high, base*share, share, base), nil
}

// The newest sign of life of a step: its newest pass, its newest event (a start or an
// extension) or the moment the sweep added it.
const stalledSQL = `
SELECT COALESCE(GREATEST(
    (SELECT max(p.t_start + p.duration_s * interval '1 second')
     FROM corner_pass p JOIN run_block b ON b.id = p.block_id WHERE b.experiment_id = $1),
    (SELECT max(at) FROM experiment_event WHERE experiment_id = $1),
    (SELECT added_at FROM sweep_step WHERE experiment_id = $1)
) < now() - $2 * interval '1 second', false)`

// stalled reports whether nothing came from the step's experiment for stallAfter.
func (s *Service) stalled(ctx context.Context, experimentID int64) (bool, error) {
	var stalled bool
	err := s.database.QueryRowContext(ctx, stalledSQL, experimentID, stallAfter.Seconds()).Scan(&stalled)
	if err != nil {
		return false, fmt.Errorf("check whether experiment %d is still driven: %w", experimentID, err)
	}
	return stalled, nil
}
