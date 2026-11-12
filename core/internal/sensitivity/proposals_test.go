package sensitivity

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

func TestProposalsRankTheDiagnosedChangesAgainstTheCarsProfile(t *testing.T) {
	f := newFixture(t, "sensitivity_proposals")
	f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03)

	got, err := f.service.Proposals(t.Context(), f.sessionID)

	if err != nil {
		t.Fatalf("Proposals: %v", err)
	}
	if got.Stage != contracts.ProposalsStageColdStart || len(got.Ranked) != 1 {
		t.Errorf("proposals = %+v, want the engine's answer", got)
	}
	request := f.engine.proposed[0]
	wantCandidates := []contracts.ProposeCandidate{
		{Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front",
			Hypotheses: []string{"mid_understeer.slow", "entry_understeer.slow"}},
		{Parameter: "ARB_REAR", Label: "ARB Rear", Clicks: 1, Why: "stiffer rear",
			Hypotheses: []string{"entry_understeer.slow"}},
	}
	if !reflect.DeepEqual(request.Candidates, wantCandidates) {
		t.Errorf("candidates = %+v", request.Candidates)
	}
	if len(request.Effects) != 1 || request.Effects[0].Parameter != "ARB_FRONT" || request.Experiments != 1 {
		t.Errorf("effects = %+v with %d experiments, want the car's profile", request.Effects, request.Experiments)
	}
	if request.Channel != "corner_time_s" || request.Better != contracts.ProposeRequestBetterLower ||
		request.Rope != 0.005 || request.LapsPerBlock != experiments.DefaultLapsPerBlock {
		t.Errorf("request = %+v, want the test every experiment is judged by", request)
	}
	if request.Corners != 7 || request.Sigma != assumedSigma {
		t.Errorf("%d corners, sigma %v; want the track's 7 and the assumed scatter", request.Corners, request.Sigma)
	}
}

func TestProposalsUseTheScatterOfTheNewestPlan(t *testing.T) {
	f := newFixture(t, "sensitivity_sigma")
	f.exec(t, `UPDATE experiment SET mde = '{"sigma": 0.019}' WHERE id = $1`,
		f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03))

	if _, err := f.service.Proposals(t.Context(), f.sessionID); err != nil {
		t.Fatalf("Proposals: %v", err)
	}

	if got := f.engine.proposed[0].Sigma; got != 0.019 {
		t.Errorf("sigma = %v, want the 0.019 the plan measured", got)
	}
}

func TestProposalsThatCannotBeMade(t *testing.T) {
	down := fmt.Errorf("connection refused: %w", engine.ErrUnavailable)
	tests := []struct {
		name      string
		sessionID int64
		diagnosis error
		engine    error
		want      error
	}{
		{name: "no such session", sessionID: 99999, want: experiments.ErrNotFound},
		{name: "not diagnosed yet", diagnosis: experiments.ErrNotFound, want: experiments.ErrNotFound},
		{name: "engine down", engine: down, want: engine.ErrUnavailable},
	}
	f := newFixture(t, "sensitivity_refused")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.diagnoses.failure, f.engine.failure = tt.diagnosis, tt.engine
			sessionID := tt.sessionID
			if sessionID == 0 {
				sessionID = f.sessionID
			}

			_, err := f.service.Proposals(t.Context(), sessionID)

			if !errors.Is(err, tt.want) {
				t.Errorf("Proposals error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestScorecardCountsPredictionsThatCameTrue(t *testing.T) {
	f := newFixture(t, "sensitivity_scorecard")
	f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03) // human, 1 of 1
	extended := f.add(t, entry{parameter: "ARB_REAR", clicks: 1, source: "rules", state: "reverted"})
	f.verdict(t, extended, "engine", 2, 2) // judged, then extended and judged again
	f.verdict(t, extended, "engine", 0, 2)
	f.verdict(t, extended, "driver", 0, 2) // the driver's call is not a second score
	f.add(t, entry{parameter: "ARB_FRONT", clicks: 1, state: "proposed"})

	got, err := f.service.Scorecard(t.Context())

	if err != nil {
		t.Fatalf("Scorecard: %v", err)
	}
	want := contracts.Scorecard{
		Experiments: 3, Judged: 2, PredictionsTotal: 3, PredictionsHit: 1,
		BySource: []contracts.SourceScore{
			{Source: contracts.SourceScoreSourceHuman, Experiments: 2, PredictionsTotal: 1, PredictionsHit: 1},
			{Source: contracts.SourceScoreSourceRules, Experiments: 1, PredictionsTotal: 2, PredictionsHit: 0},
		},
		ByState: []contracts.StateCount{
			{State: "accepted", Count: 1}, {State: "proposed", Count: 1}, {State: "reverted", Count: 1},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scorecard = %+v\nwant %+v", got, want)
	}
}

func TestAnEmptyLedgerHasAnEmptyScorecard(t *testing.T) {
	f := newFixture(t, "sensitivity_scorecard_empty")

	got, err := f.service.Scorecard(t.Context())

	if err != nil {
		t.Fatalf("Scorecard: %v", err)
	}
	if got.Experiments != 0 || got.BySource == nil || got.ByState == nil {
		t.Errorf("scorecard = %+v, want zeros and empty lists", got)
	}
}
