package training

import (
	"context"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// next starts the program's next experiment: the best-ranked change of the session's
// diagnosis on an item the program has not tested yet. The ranking already leaves out what
// this car measured as negligible or worse; an item tested in this program is not tried again,
// so the AI spends its laps on what is still unknown.
func (s *Service) next(ctx context.Context, program contracts.TrainingProgram) error {
	proposals, err := s.ranker.Proposals(ctx, int64(program.SessionId))
	if err != nil {
		return fmt.Errorf("rank experiments for program %d: %w", program.Id, err)
	}
	proposal, found := pick(proposals.Ranked, program.Steps)
	if !found {
		return s.finish(ctx, int64(program.Id), fmt.Sprintf(
			"Done: nothing left to test from session %d's diagnosis after %d experiment(s).",
			program.SessionId, len(program.Steps)))
	}
	exp, err := s.ledger.Create(ctx, createRequest(program.SessionId, proposal))
	if err != nil {
		return fmt.Errorf("propose %s for program %d: %w", proposal.Parameter, program.Id, err)
	}
	if err := addStep(ctx, s.database, int64(program.Id), int64(exp.Id)); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "training proposed an experiment", "program_id", program.Id,
		"experiment_id", exp.Id, "parameter", proposal.Parameter, "clicks", proposal.Clicks)
	if _, err := s.ledger.Plan(ctx, int64(exp.Id)); err != nil {
		return err
	}
	return s.start(ctx, int64(program.Id), int64(exp.Id))
}

// pick returns the first ranked proposal whose item no step has tested.
func pick(ranked []contracts.Proposal, steps []contracts.TrainingStep) (contracts.Proposal, bool) {
	tested := map[string]bool{}
	for _, step := range steps {
		tested[step.Parameter] = true
	}
	for _, proposal := range ranked {
		if !tested[proposal.Parameter] {
			return proposal, true
		}
	}
	return contracts.Proposal{}, false
}

// createRequest is the experiment a proposal asks for, with the AI driving. A prediction is
// written down only when the whole interval of the expected gain is an improvement, as the
// in-game desk and the propose form do.
func createRequest(sessionID int, proposal contracts.Proposal) contracts.ExperimentCreate {
	request := contracts.ExperimentCreate{
		SessionId: sessionID, Parameter: proposal.Parameter, Clicks: proposal.Clicks,
		Statement:   "Training: " + proposal.Why + ".",
		Driver:      contracts.ExperimentCreateDriverAi,
		Predictions: []contracts.PredictionSpec{},
	}
	if proposal.GainLow > 0 && proposal.GainHigh >= proposal.GainLow {
		request.Predictions = append(request.Predictions, contracts.PredictionSpec{
			Channel: "corner_time_s", Direction: contracts.PredictionSpecDirectionDecrease,
			Low: proposal.GainLow, High: proposal.GainHigh,
		})
	}
	if len(proposal.Hypotheses) > 0 {
		key := proposal.Hypotheses[0]
		request.HypothesisKey = &key
	}
	return request
}
