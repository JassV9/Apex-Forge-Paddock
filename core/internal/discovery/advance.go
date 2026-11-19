package discovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Run advances every open sweep every interval until ctx is cancelled.
func (s *Service) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.AdvanceAll(ctx)
		}
	}
}

// AdvanceAll moves every open sweep one step along. A sweep whose step fails is logged and
// tried again on the next call.
func (s *Service) AdvanceAll(ctx context.Context) {
	ids, err := openSweeps(ctx, s.database)
	if err != nil {
		s.logger.ErrorContext(ctx, "list open sweeps", "error", err)
		return
	}
	for _, id := range ids {
		if err := s.advance(ctx, id); err != nil {
			s.logger.ErrorContext(ctx, "sweep step failed", "sweep_id", id, "error", err)
		}
	}
}

// advance looks at the sweep's newest step: it acts on its experiment while it is open, reads
// its response once it is closed, then starts the next step, or classifies the curves when
// every step is measured.
func (s *Service) advance(ctx context.Context, id int64) error {
	sweep, err := s.Get(ctx, id)
	if err != nil || (sweep.State != contracts.SweepStateRunning && sweep.State != contracts.SweepStateWaiting) {
		return err
	}
	steps, err := readSteps(ctx, s.database, id)
	if err != nil {
		return err
	}
	if n := len(steps); n > 0 {
		exp, err := s.ledger.Get(ctx, steps[n-1].experimentID)
		if err != nil {
			return err
		}
		if wait, err := s.tend(ctx, sweep, steps[n-1].clicks, exp); wait || err != nil {
			return err
		}
		if wait, err := s.readResponse(ctx, sweep, &steps[n-1]); wait || err != nil {
			return err
		}
	}
	if len(steps) >= len(sweep.Planned) {
		return s.conclude(ctx, sweep, steps)
	}
	return s.next(ctx, sweep, sweep.Planned[len(steps)])
}

// tend acts on the step's experiment. It reports true while the sweep has to wait for it, or
// has ended because of it.
func (s *Service) tend(ctx context.Context, sweep contracts.Sweep, clicks int, exp contracts.Experiment) (bool, error) {
	id, eid := int64(sweep.Id), int64(exp.Id)
	switch exp.State {
	case experiments.Proposed:
		_, err := s.ledger.Plan(ctx, eid)
		return true, err
	case experiments.Planned:
		return true, s.start(ctx, id, eid)
	case experiments.Running:
		return true, s.drive(ctx, sweep, clicks, exp)
	case experiments.Evaluating:
		return true, nil
	case experiments.Inconclusive:
		return true, s.settle(ctx, sweep, clicks, exp)
	case experiments.Aborted, experiments.Discarded:
		return true, s.fail(ctx, id, fmt.Sprintf(
			"Step %+d (experiment %d) was %s: the AI could not drive it or the driver stopped it. "+
				"The steps before it stand; start the sweep again to measure the rest.", clicks, exp.Id, exp.State))
	}
	if invalid(exp) { // reverted by hand after an invalid verdict
		return true, s.fail(ctx, id, invalidMessage(clicks, exp))
	}
	return false, nil // measured, and the baseline is back on the car
}

// start puts the step's experiment on the car; when another one is on it the sweep waits.
func (s *Service) start(ctx context.Context, id, experimentID int64) error {
	_, err := s.ledger.Start(ctx, experimentID)
	if errors.Is(err, experiments.ErrConflict) {
		return s.setState(ctx, id, contracts.SweepStateWaiting,
			fmt.Sprintf("Waiting to start experiment %d: %v", experimentID, err))
	}
	if err != nil {
		return err
	}
	return s.setState(ctx, id, contracts.SweepStateRunning,
		fmt.Sprintf("Experiment %d is on the car; the AI drives it.", experimentID))
}

// drive watches a step being driven: it stops the sweep when the AI was not given the pinned
// hint or has stopped driving, and asks for the verdict once every block is in.
func (s *Service) drive(ctx context.Context, sweep contracts.Sweep, clicks int, exp contracts.Experiment) error {
	id := int64(sweep.Id)
	off, err := s.hintOff(ctx, int64(exp.Id), sweep.AiHintShare)
	if err != nil {
		return err
	}
	if off != "" {
		return s.fail(ctx, id, fmt.Sprintf(
			"Step %+d (experiment %d) is invalid: %s. Abort it; the steps before it stand.", clicks, exp.Id, off))
	}
	if experiments.AllDriven(exp.Blocks) {
		_, err := s.ledger.Evaluate(ctx, int64(exp.Id))
		return err
	}
	stalled, err := s.stalled(ctx, int64(exp.Id))
	if err != nil {
		return err
	}
	if stalled {
		return s.fail(ctx, id, fmt.Sprintf(
			"The AI stopped driving step %+d: nothing from experiment %d for %s (car stuck, game closed or "+
				"a watchdog stopped the run). Abort it or let it finish, then start the sweep again.",
			clicks, exp.Id, stallAfter))
	}
	return s.setState(ctx, id, contracts.SweepStateRunning,
		fmt.Sprintf("The AI is driving step %+d of %v (experiment %d, %s).", clicks, sweep.Planned, exp.Id, progress(exp)))
}

func progress(exp contracts.Experiment) string {
	for _, block := range exp.Blocks {
		if block.LapsDone < block.LapsTarget {
			return fmt.Sprintf("block %d of %d, %d of %d laps done", block.Order, len(exp.Blocks), block.LapsDone, block.LapsTarget)
		}
	}
	return "all blocks driven"
}

// settle decides what follows an unclear step: an invalid one stops the sweep, one the engine
// says to continue is driven to its next pre-registered look (ADR 0029), and any other
// (stop_neutral, larger_perturbation, ...) is measured as it is: the baseline goes back and the
// sweep moves on. A sweep never takes a larger step than planned; the curve is the answer.
func (s *Service) settle(ctx context.Context, sweep contracts.Sweep, clicks int, exp contracts.Experiment) error {
	id, eid := int64(sweep.Id), int64(exp.Id)
	if invalid(exp) {
		reason := fmt.Sprintf("Sweep %d put the baseline back: step %+d is invalid.", id, clicks)
		if _, err := s.ledger.RevertByTraining(ctx, eid, reason); err != nil {
			return err
		}
		return s.fail(ctx, id, invalidMessage(clicks, exp))
	}
	if continues(exp) && exp.Extensions < experiments.MaxExtensions {
		_, err := s.ledger.Extend(ctx, eid)
		return err
	}
	_, err := s.ledger.RevertByTraining(ctx, eid, fmt.Sprintf(
		"Sweep %d measured step %+d and put the baseline back (%s).", id, clicks, recommendation(exp)))
	return err
}

// invalid reports whether the step's verdict cannot be trusted (ADR 0029).
func invalid(exp contracts.Experiment) bool {
	v := exp.Verdict
	return v != nil && ((v.FindingClass != nil && *v.FindingClass == contracts.VerdictSummaryFindingClassInvalid) ||
		(v.Recommendation != nil && *v.Recommendation == contracts.VerdictSummaryRecommendationStopInvalid))
}

func invalidMessage(clicks int, exp contracts.Experiment) string {
	return fmt.Sprintf("Step %+d (experiment %d) is invalid: %s The steps before it stand; fix the cause and "+
		"start the sweep again.", clicks, exp.Id, exp.Verdict.Reason)
}

// continues reports whether the engine's verdict says to drive to the next look.
func continues(exp contracts.Experiment) bool {
	return exp.Verdict != nil && exp.Verdict.Recommendation != nil &&
		*exp.Verdict.Recommendation == contracts.VerdictSummaryRecommendationContinue
}

func recommendation(exp contracts.Experiment) string {
	if exp.Verdict == nil || exp.Verdict.Recommendation == nil {
		return "no recommendation"
	}
	return string(*exp.Verdict.Recommendation)
}
