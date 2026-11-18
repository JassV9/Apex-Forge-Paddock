package ingest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The run context of a session and its blocks (docs/adr/0028-run-context-and-ai-provenance.md).
// A session line is stored once; a block's row gathers the hints of its passes as they arrive.

const insertSessionContextSQL = `
INSERT INTO run_context (
    session_id, csp_version, csp_build, cars_count, ai_tyre_hint_base, ai_learned_share,
    ai_pinned_share, ai_applied_hint, ai_extra_grip, ai_aero_hint, ai_brake_hint, ai_level,
    ai_aggression
) SELECT id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13 FROM session WHERE spool_id = $1
ON CONFLICT (session_id, block_id) DO NOTHING`

// The pinned share is the experiment's when the block's first pass arrived: it is fixed at
// proposal, so later passes cannot see another.
const upsertBlockContextSQL = `
INSERT INTO run_context (session_id, block_id, ai_pinned_share, ai_hint_min, ai_hint_max, passes)
SELECT $1, b.id, e.ai_hint_share, $4, $4, 1
FROM run_block b JOIN experiment e ON e.id = b.experiment_id
WHERE b.experiment_id = $2 AND b."order" = $3
ON CONFLICT (session_id, block_id) DO UPDATE SET
    ai_hint_min = LEAST(run_context.ai_hint_min, EXCLUDED.ai_hint_min),
    ai_hint_max = GREATEST(run_context.ai_hint_max, EXCLUDED.ai_hint_max),
    passes = run_context.passes + 1`

// saveSessionContext stores what the session line says it was driven under; a session line
// recorded before the agent sent it stores nothing.
func saveSessionContext(ctx context.Context, tx *sql.Tx, s *contracts.SpoolSession) error {
	if s.Context == nil {
		return nil
	}
	c, ai := s.Context, s.Context.Ai
	_, err := tx.ExecContext(ctx, insertSessionContextSQL,
		s.SessionId, optional(c.CspVersion), optional(c.CspBuild), c.CarsCount, ai.TyreHintBase,
		ai.LearnedShare, optional(ai.PinnedShare), optional(ai.AppliedHint), optional(ai.ExtraGrip),
		optional(ai.AeroHint), optional(ai.BrakeHint), optional(ai.Level), optional(ai.Aggression))
	if err != nil {
		return fmt.Errorf("save run context of session %s: %w", s.SessionId, err)
	}
	return nil
}

// saveBlockContext counts a newly stored pass towards its block's run context. A pass of a
// block core does not know is stored without one, as its block_id is NULL.
func saveBlockContext(ctx context.Context, tx *sql.Tx, session sessionRef, pass *contracts.SpoolPass) error {
	if pass.Block == nil {
		return nil
	}
	var hint *float64
	if pass.Context != nil {
		hint = pass.Context.AiHint
	}
	_, err := tx.ExecContext(ctx, upsertBlockContextSQL,
		session.id, pass.Block.ExperimentId, pass.Block.Order, optional(hint))
	if err != nil {
		return fmt.Errorf("save run context of block %d: %w", pass.Block.Order, err)
	}
	return nil
}

// passContextArgs are the corner_pass columns of the pass's context: ai_hint, damage_max,
// reset_counter, abs_share, tc_share, drs_share, clock; all NULL on a line without one.
func passContextArgs(c *contracts.PassContext) []any {
	if c == nil {
		return make([]any, 7)
	}
	return []any{
		optional(c.AiHint), c.DamageMax, c.Resets, c.AbsShare, c.TcShare, c.DrsShare, string(c.Clock),
	}
}
