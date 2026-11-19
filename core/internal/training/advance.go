package training

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Run advances every open program every interval until ctx is cancelled.
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

// AdvanceAll moves every open program one step along. A program that fails is logged and
// tried again on the next call.
func (s *Service) AdvanceAll(ctx context.Context) {
	ids, err := openPrograms(ctx, s.database)
	if err != nil {
		s.logger.ErrorContext(ctx, "list open training programs", "error", err)
		return
	}
	for _, id := range ids {
		if err := s.advance(ctx, id); err != nil {
			s.logger.ErrorContext(ctx, "training step failed", "program_id", id, "error", err)
		}
	}
}

// advance looks at the program's newest experiment: it acts on it while it is open, and once
// it is closed starts the next one, or ends the program.
func (s *Service) advance(ctx context.Context, id int64) error {
	program, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if n := len(program.Steps); n > 0 {
		exp, err := s.ledger.Get(ctx, int64(program.Steps[n-1].ExperimentId))
		if err != nil {
			return err
		}
		if open, err := s.tend(ctx, id, exp); open || err != nil {
			return err
		}
	}
	if len(program.Steps) >= program.Target {
		return s.finish(ctx, id, fmt.Sprintf("Done: drove %d experiment(s).", len(program.Steps)))
	}
	return s.next(ctx, program)
}

// tend acts on the newest experiment. It reports true while that experiment is still open,
// so the program waits for it.
func (s *Service) tend(ctx context.Context, id int64, exp contracts.Experiment) (bool, error) {
	eid := int64(exp.Id)
	switch exp.State {
	case experiments.Running:
		if !experiments.AllDriven(exp.Blocks) {
			return true, s.setState(ctx, id, contracts.TrainingProgramStateRunning,
				fmt.Sprintf("The AI is driving experiment %d (%s).", exp.Id, progress(exp)))
		}
		_, err := s.ledger.Evaluate(ctx, eid)
		return true, err
	case experiments.Evaluating:
		return true, nil
	case experiments.Inconclusive:
		if continues(exp) && exp.Extensions < experiments.MaxExtensions {
			_, err := s.ledger.Extend(ctx, eid)
			return true, err
		}
		_, err := s.ledger.RevertByTraining(ctx, eid, fmt.Sprintf(
			"Training program %d kept the baseline: still unclear after %d extension(s).", id, exp.Extensions))
		return true, err
	case experiments.Rejected:
		_, err := s.ledger.RevertByTraining(ctx, eid, fmt.Sprintf(
			"Training program %d put the baseline back after the change was rejected.", id))
		return true, err
	case experiments.Tradeoff:
		return true, s.setState(ctx, id, contracts.TrainingProgramStateWaiting,
			fmt.Sprintf("Experiment %d is a trade-off: your call, in the dashboard or the game.", exp.Id))
	case experiments.Proposed:
		_, err := s.ledger.Plan(ctx, eid)
		return true, err
	case experiments.Planned:
		return true, s.start(ctx, id, eid)
	}
	return false, nil // accepted, reverted, aborted or discarded: closed
}

func progress(exp contracts.Experiment) string {
	for _, block := range exp.Blocks {
		if block.LapsDone < block.LapsTarget {
			return fmt.Sprintf("block %d of %d, %d of %d laps done", block.Order, len(exp.Blocks), block.LapsDone, block.LapsTarget)
		}
	}
	return "all blocks driven"
}

// continues reports whether the engine's verdict on the experiment says to drive it to the
// next pre-registered look: the effect could still clear the margin, and the look spends its
// share of the error (docs/adr/0029-block-level-inference-and-finding-classes.md). A verdict
// judged before ADR 0029 recommends nothing, and the baseline stays.
func continues(exp contracts.Experiment) bool {
	return exp.Verdict != nil && exp.Verdict.Recommendation != nil &&
		*exp.Verdict.Recommendation == contracts.VerdictSummaryRecommendationContinue
}

// start puts the experiment on the car; when another one is on it the program waits.
func (s *Service) start(ctx context.Context, id, experimentID int64) error {
	_, err := s.ledger.Start(ctx, experimentID)
	if errors.Is(err, experiments.ErrConflict) {
		return s.setState(ctx, id, contracts.TrainingProgramStateWaiting,
			fmt.Sprintf("Waiting to start experiment %d: %v", experimentID, err))
	}
	if err != nil {
		return err
	}
	return s.setState(ctx, id, contracts.TrainingProgramStateRunning,
		fmt.Sprintf("Experiment %d is on the car; the AI drives it.", experimentID))
}

func (s *Service) finish(ctx context.Context, id int64, message string) error {
	s.logger.InfoContext(ctx, "training finished", "program_id", id, "message", message)
	return s.setState(ctx, id, contracts.TrainingProgramStateDone, message)
}
