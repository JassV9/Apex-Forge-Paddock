package experiments

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// RecheckEvery is how many accepted experiments may pile up before the champion is driven
// against the original baseline again. Each accepted step was measured against the step
// before it; only the re-check shows whether the steps still add up (docs/design.md section F:
// this is what would have caught v0's setups drifting from v2 to v9).
const RecheckEvery = 3

const recheckStatement = "Re-check: the current setup is still faster than the original baseline."

// anchor makes sure the experiment is measured against the champion before its setups go on
// the car. The first experiment of a car and track makes its baseline the champion and the
// original. Later ones must start from the champion (a re-check must end on it); a proposal
// that the ledger has moved past is refused.
func anchor(ctx context.Context, tx *sql.Tx, exp record) error {
	expected := exp.baselineVersion
	if exp.kind == contracts.ExperimentKindRecheck {
		expected = exp.variantVersion
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO champion (car_id, track_id, setup_version_id, original_version_id, accepted_since_check)
		 VALUES ($1, $2, $3, $3, 0) ON CONFLICT (car_id, track_id) DO NOTHING`,
		exp.carID, exp.trackID, expected)
	if err != nil {
		return fmt.Errorf("anchor champion: %w", err)
	}
	var champion int64
	err = tx.QueryRowContext(ctx,
		`SELECT setup_version_id FROM champion WHERE car_id = $1 AND track_id = $2 FOR UPDATE`,
		exp.carID, exp.trackID).Scan(&champion)
	if err != nil {
		return fmt.Errorf("read champion: %w", err)
	}
	if champion != expected {
		return fmt.Errorf("experiment %d was proposed against a setup that is no longer the champion; "+
			"discard it and propose it again: %w", exp.id, ErrConflict)
	}
	return nil
}

// promote makes the experiment's variant the champion of its car and track. A re-check that
// is accepted confirms the champion instead. When RecheckEvery experiments were accepted
// since the last check, a re-check is proposed.
func promote(ctx context.Context, tx *sql.Tx, exp record) error {
	if exp.kind == contracts.ExperimentKindRecheck {
		_, err := tx.ExecContext(ctx,
			`UPDATE champion SET accepted_since_check = 0 WHERE car_id = $1 AND track_id = $2`,
			exp.carID, exp.trackID)
		if err != nil {
			return fmt.Errorf("confirm champion: %w", err)
		}
		return nil
	}
	var accepted int
	var original int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO champion (car_id, track_id, setup_version_id, original_version_id, accepted_since_check)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (car_id, track_id) DO UPDATE SET
		     setup_version_id = EXCLUDED.setup_version_id,
		     accepted_since_check = champion.accepted_since_check + 1
		 RETURNING accepted_since_check, original_version_id`,
		exp.carID, exp.trackID, exp.variantVersion, exp.baselineVersion).Scan(&accepted, &original)
	if err != nil {
		return fmt.Errorf("promote setup %d to champion: %w", exp.variantVersion, err)
	}
	if accepted < RecheckEvery || original == exp.variantVersion {
		return nil
	}
	return proposeRecheck(ctx, tx, exp, original)
}

// proposeRecheck queues the champion against the original baseline, unless a re-check for
// this car and track is already open.
func proposeRecheck(ctx context.Context, tx *sql.Tx, exp record, original int64) error {
	var open int
	err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
		 WHERE h.car_id = $1 AND h.track_id = $2 AND e.kind = 'recheck'
		   AND e.state NOT IN ('accepted', 'reverted', 'aborted', 'discarded')`,
		exp.carID, exp.trackID).Scan(&open)
	if err != nil {
		return fmt.Errorf("look for an open re-check: %w", err)
	}
	if open > 0 {
		return nil
	}
	baselineLabel, variantLabel := randomLabels()
	_, err = insertDraft(ctx, tx, draft{
		carID: exp.carID, trackID: exp.trackID, kind: contracts.ExperimentKindRecheck,
		baselineVersion: original, variantVersion: exp.variantVersion,
		statement: recheckStatement, source: "rules",
		predictions: []contracts.PredictionSpec{},
	}, baselineLabel, variantLabel)
	return err
}

// dethrone handles the champion when an experiment is reverted. A reverted re-check means
// the accepted steps did not add up: the original baseline is the champion again. A reverted
// accepted experiment is a rollback, and only the current champion can be rolled back.
func dethrone(ctx context.Context, tx *sql.Tx, exp record, from State) error {
	if exp.kind == contracts.ExperimentKindRecheck {
		_, err := tx.ExecContext(ctx,
			`UPDATE champion SET setup_version_id = original_version_id, accepted_since_check = 0
			 WHERE car_id = $1 AND track_id = $2`, exp.carID, exp.trackID)
		if err != nil {
			return fmt.Errorf("return champion to the original baseline: %w", err)
		}
		return nil
	}
	if from != Accepted {
		return nil // the variant never became champion
	}
	outcome, err := tx.ExecContext(ctx,
		`UPDATE champion SET setup_version_id = $3,
		     accepted_since_check = GREATEST(accepted_since_check - 1, 0)
		 WHERE car_id = $1 AND track_id = $2 AND setup_version_id = $4`,
		exp.carID, exp.trackID, exp.baselineVersion, exp.variantVersion)
	if err != nil {
		return fmt.Errorf("roll back champion: %w", err)
	}
	changed, err := outcome.RowsAffected()
	if err != nil {
		return fmt.Errorf("roll back champion: %w", err)
	}
	if changed == 0 {
		return fmt.Errorf("experiment %d: its variant is not the current champion: %w", exp.id, ErrConflict)
	}
	return nil
}
