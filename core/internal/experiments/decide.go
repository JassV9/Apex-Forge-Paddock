package experiments

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Evaluate sends the experiment's push-lap passes to the engine and applies its verdict.
// The decision is the engine's, from the numbers and the test fixed before driving; core
// only carries out what follows from it.
func (s *Service) Evaluate(ctx context.Context, id int64) (contracts.Experiment, error) {
	exp, err := loadRecord(ctx, s.database, id, false)
	if err != nil {
		return contracts.Experiment{}, err
	}
	if exp.state != Running {
		return contracts.Experiment{}, fmt.Errorf("experiment %d is %s, not running: %w", id, exp.state, ErrConflict)
	}
	passes, pending, err := blockPasses(ctx, s.database, id)
	if err != nil {
		return contracts.Experiment{}, err
	}
	if pending > 0 {
		return contracts.Experiment{}, fmt.Errorf(
			"experiment %d: the features of %d passes are not computed yet: %w", id, pending, ErrConflict)
	}
	predictions, err := readPredictions(ctx, s.database, id)
	if err != nil {
		return contracts.Experiment{}, fmt.Errorf("read predictions of experiment %d: %w", id, err)
	}
	reply, err := s.engine.Evaluate(ctx, contracts.EvaluateRequest{
		SuccessTest: exp.test, Guardrails: exp.guardrails, Predictions: predictions,
		DriverChannels: driverChannels, Passes: passes,
	})
	if err != nil {
		return contracts.Experiment{}, fmt.Errorf("evaluate experiment %d: %w", id, err)
	}
	return s.act(ctx, id, true, func(tx *sql.Tx, current record) error {
		return applyVerdict(ctx, tx, current, reply)
	})
}

// applyVerdict stores the engine's results and verdict and moves the experiment on.
func applyVerdict(ctx context.Context, tx *sql.Tx, exp record, reply contracts.EvaluateResponse) error {
	judged := fmt.Sprintf("%d passes judged, %d left out.", reply.PassesUsed, reply.PassesExcluded)
	if err := move(ctx, tx, exp.id, exp.state, Evaluating, judged); err != nil {
		return err
	}
	if err := saveResults(ctx, tx, exp.id, reply.Results); err != nil {
		return err
	}
	err := addVerdict(ctx, tx, exp.id, verdict{
		decision: contracts.VerdictSummaryDecision(reply.Decision), reason: reply.Reason,
		hits: reply.PredictionHits, total: reply.PredictionTotal,
		decidedBy: contracts.VerdictSummaryDecidedByEngine,
	})
	if err != nil {
		return err
	}
	decision := State(reply.Decision)
	if err := move(ctx, tx, exp.id, Evaluating, decision, reply.Reason); err != nil {
		return err
	}
	return settle(ctx, tx, exp, decision)
}

// settle carries out what follows from a verdict without asking anyone: an accepted variant
// becomes the champion, a rejected one comes off the car, and an experiment that is still
// unclear after its last extension goes to the baseline. A trade-off waits for the driver.
func settle(ctx context.Context, tx *sql.Tx, exp record, decision State) error {
	switch {
	case decision == Accepted:
		return promote(ctx, tx, exp)
	case decision == Rejected:
		return restore(ctx, tx, exp, Rejected, "The baseline goes back on the car.")
	case decision == Inconclusive && exp.extensions >= MaxExtensions:
		reason := fmt.Sprintf("Still unclear after %d extensions: ties go to the baseline.", MaxExtensions)
		return restore(ctx, tx, exp, Inconclusive, reason)
	}
	return nil
}

// restore reverts the experiment: the baseline is queued to go back on the car and, where
// the variant was the champion, the champion goes back too.
func restore(ctx context.Context, tx *sql.Tx, exp record, from State, reason string) error {
	if err := move(ctx, tx, exp.id, from, Reverted, reason); err != nil {
		return err
	}
	if err := dethrone(ctx, tx, exp, from); err != nil {
		return err
	}
	return addRestoreBlock(ctx, tx, exp.id)
}

// Decide is the driver's call on a trade-off: keep the variant, or go back to the baseline.
func (s *Service) Decide(ctx context.Context, id int64, accept bool) (contracts.Experiment, error) {
	return s.act(ctx, id, true, func(tx *sql.Tx, exp record) error {
		if exp.state != Tradeoff {
			return fmt.Errorf("experiment %d is %s, not a trade-off: %w", id, exp.state, ErrConflict)
		}
		if !accept {
			reason := "The driver declined the trade-off."
			if err := restore(ctx, tx, exp, Tradeoff, reason); err != nil {
				return err
			}
			return driverVerdict(ctx, tx, id, contracts.VerdictSummaryDecisionReverted, reason)
		}
		reason := "The driver accepted the trade-off."
		if err := move(ctx, tx, id, Tradeoff, Accepted, reason); err != nil {
			return err
		}
		if err := promote(ctx, tx, exp); err != nil {
			return err
		}
		return driverVerdict(ctx, tx, id, contracts.VerdictSummaryDecisionAccepted, reason)
	})
}

// Extend adds two more blocks to an inconclusive experiment, at most MaxExtensions times.
func (s *Service) Extend(ctx context.Context, id int64) (contracts.Experiment, error) {
	return s.act(ctx, id, true, func(tx *sql.Tx, exp record) error {
		reason := fmt.Sprintf("Extended (%d of %d): two more blocks.", exp.extensions+1, MaxExtensions)
		if err := move(ctx, tx, id, exp.state, Running, reason); err != nil {
			return err
		}
		return extend(ctx, tx, exp)
	})
}

// Revert goes back to the baseline: giving up on an inconclusive experiment, declining a
// trade-off, or rolling back an accepted experiment whose variant is the current champion.
func (s *Service) Revert(ctx context.Context, id int64) (contracts.Experiment, error) {
	return s.act(ctx, id, true, func(tx *sql.Tx, exp record) error {
		reason := "The driver kept the baseline."
		if exp.state == Accepted {
			other, err := otherOccupying(ctx, tx, id)
			if err != nil {
				return err
			}
			if other != 0 {
				return fmt.Errorf("experiment %d is on the car; finish it first: %w", other, ErrConflict)
			}
			reason = "Rolled back by the driver: the baseline is the champion again."
		}
		if err := restore(ctx, tx, exp, exp.state, reason); err != nil {
			return err
		}
		return driverVerdict(ctx, tx, id, contracts.VerdictSummaryDecisionReverted, reason)
	})
}

// Abort stops a running experiment (damage, conditions out of the window, no time left).
// What was driven is kept; the baseline is queued to go back on the car.
func (s *Service) Abort(ctx context.Context, id int64) (contracts.Experiment, error) {
	return s.act(ctx, id, true, func(tx *sql.Tx, exp record) error {
		reason := "Aborted by the driver."
		if err := move(ctx, tx, id, exp.state, Aborted, reason); err != nil {
			return err
		}
		if err := addRestoreBlock(ctx, tx, id); err != nil {
			return err
		}
		return driverVerdict(ctx, tx, id, contracts.VerdictSummaryDecisionAborted, reason)
	})
}

// Discard drops an experiment before it is driven. It stays in the ledger as discarded.
func (s *Service) Discard(ctx context.Context, id int64) (contracts.Experiment, error) {
	return s.act(ctx, id, false, func(tx *sql.Tx, exp record) error {
		return move(ctx, tx, id, exp.state, Discarded, "Discarded before driving.")
	})
}
