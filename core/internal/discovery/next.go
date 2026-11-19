package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// next starts the sweep's next step against the sweep's baseline. When the baseline moved (an
// experiment outside the sweep changed the champion) the steps could no longer be compared:
// the sweep stops instead.
func (s *Service) next(ctx context.Context, sweep contracts.Sweep, clicks int) error {
	id := int64(sweep.Id)
	baseline, err := s.ledger.BaselineVersion(ctx, int64(sweep.SessionId))
	if err != nil {
		return err
	}
	if baseline != int64(sweep.BaselineVersionId) {
		return s.fail(ctx, id, fmt.Sprintf("The baseline moved from setup %d to %d; every step must be "+
			"measured against the same baseline. The steps so far stand.", sweep.BaselineVersionId, baseline))
	}
	share := sweep.AiHintShare
	exp, err := s.ledger.CreateSweepStep(ctx, contracts.ExperimentCreate{
		SessionId: sweep.SessionId, Parameter: sweep.Parameter, Clicks: clicks,
		Statement: fmt.Sprintf("Sweep %d: how the car responds to %s %+d (steps %v).",
			id, sweep.ParameterLabel, clicks, sweep.Planned),
		Driver: contracts.ExperimentCreateDriverAi, AiHintShare: &share, Predictions: []contracts.PredictionSpec{},
	})
	if errors.Is(err, experiments.ErrInvalid) {
		return s.fail(ctx, id, fmt.Sprintf("Step %+d cannot be driven: %v", clicks, err))
	}
	if err != nil {
		return fmt.Errorf("propose step %+d of sweep %d: %w", clicks, id, err)
	}
	if err := addStep(ctx, s.database, id, clicks, int64(exp.Id)); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "sweep proposed a step", "sweep_id", id, "experiment_id", exp.Id,
		"parameter", sweep.Parameter, "clicks", clicks)
	if _, err := s.ledger.Plan(ctx, int64(exp.Id)); err != nil {
		return err
	}
	return s.start(ctx, id, int64(exp.Id))
}

// readResponse reads a measured step's response cells once (ADR 0031): about 25 s of the
// engine's time per step. It reports true while the sweep has to wait for the engine. A step
// the engine cannot analyse (no samples, say) keeps its verdict channels only.
func (s *Service) readResponse(ctx context.Context, sweep contracts.Sweep, one *step) (bool, error) {
	if one.response != nil {
		return false, nil
	}
	request, err := s.ledger.ResponseRequest(ctx, one.experimentID)
	if err != nil {
		return true, err
	}
	cells := []contracts.ResponseEffect{}
	reply, err := s.engine.Response(ctx, request)
	switch {
	case errors.Is(err, engine.ErrRejected):
		s.logger.WarnContext(ctx, "sweep step has no response analysis", "sweep_id", sweep.Id,
			"experiment_id", one.experimentID, "error", err)
	case err != nil:
		return true, s.setState(ctx, int64(sweep.Id), contracts.SweepStateRunning, fmt.Sprintf(
			"Step %+d is measured; waiting for the engine's response analysis: %v", one.clicks, err))
	default:
		cells = expectedCells(reply.Effects)
	}
	encoded, err := json.Marshal(cells)
	if err != nil {
		return true, fmt.Errorf("encode the response of experiment %d: %w", one.experimentID, err)
	}
	_, err = s.database.ExecContext(ctx, `UPDATE sweep_step SET response = $2 WHERE experiment_id = $1`,
		one.experimentID, string(encoded))
	if err != nil {
		return true, fmt.Errorf("store the response of experiment %d: %w", one.experimentID, err)
	}
	one.response = encoded
	return false, nil
}

// conclude asks the engine to classify each observable's curve from every step and ends the
// sweep with it. Without the engine the sweep waits and asks again.
func (s *Service) conclude(ctx context.Context, sweep contracts.Sweep, steps []step) error {
	id := int64(sweep.Id)
	done := make([]measured, 0, len(steps))
	for _, one := range steps {
		exp, err := s.ledger.Get(ctx, one.experimentID)
		if err != nil {
			return err
		}
		m := measured{clicks: one.clicks, exp: exp}
		if err := json.Unmarshal(one.response, &m.cells); err != nil {
			return fmt.Errorf("decode the response of experiment %d: %w", one.experimentID, err)
		}
		done = append(done, m)
	}
	curve, err := s.engine.Curve(ctx, curveRequest(sweep.Parameter, done))
	if err != nil {
		return s.setState(ctx, id, contracts.SweepStateRunning,
			fmt.Sprintf("Every step is measured; waiting for the engine to classify the curves: %v", err))
	}
	encoded, err := json.Marshal(curve)
	if err != nil {
		return fmt.Errorf("encode the curves of sweep %d: %w", id, err)
	}
	message := fmt.Sprintf("Done: %d step(s) measured against setup %d; %d curve(s) classified.",
		len(steps), sweep.BaselineVersionId, len(curve.Curves))
	_, err = s.database.ExecContext(ctx,
		`UPDATE sweep SET curve = $2, state = 'done', message = $3, updated_at = now()
		 WHERE id = $1 AND state IN ('running', 'waiting')`, id, string(encoded), message)
	if err != nil {
		return fmt.Errorf("store the curves of sweep %d: %w", id, err)
	}
	s.logger.InfoContext(ctx, "sweep done", "sweep_id", id, "steps", len(steps), "curves", len(curve.Curves))
	return nil
}
