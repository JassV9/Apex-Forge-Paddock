package experiments

import (
	"slices"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// runSweepStep takes a sweep step from proposed to the engine's verdict.
func (f *fixture) runSweepStep(t *testing.T, clicks int, decision contracts.EvaluateResponseDecision) contracts.Experiment {
	t.Helper()
	exp, err := f.service.CreateSweepStep(t.Context(), contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: clicks, Statement: "Sweep step.",
		Predictions: []contracts.PredictionSpec{},
	})
	if err != nil {
		t.Fatalf("CreateSweepStep: %v", err)
	}
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return f.judge(t, exp.Id, decision)
}

func TestASweepStepIsMeasuredAndNeverPromoted(t *testing.T) {
	tests := []struct {
		decision contracts.EvaluateResponseDecision
		want     []string
	}{
		{accepted, []string{"proposed", "planned", "running", "evaluating", "accepted", "reverted"}},
		{tradeoff, []string{"proposed", "planned", "running", "evaluating", "tradeoff", "reverted"}},
		{rejected, []string{"proposed", "planned", "running", "evaluating", "rejected", "reverted"}},
		{inconclusive, []string{"proposed", "planned", "running", "evaluating", "inconclusive"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.decision), func(t *testing.T) {
			f := newFixture(t, "exp_sweep_"+string(tt.decision))

			first := f.runSweepStep(t, -1, tt.decision)

			if got := path(first); !slices.Equal(got, tt.want) {
				t.Errorf("events = %v, want %v", got, tt.want)
			}
			if first.Purpose == nil || *first.Purpose != contracts.ExperimentPurposeSweep {
				t.Errorf("purpose = %v", first.Purpose)
			}
			champion := f.count(t, `SELECT count(*) FROM champion c JOIN experiment_arm a
				ON a.setup_version_id = c.setup_version_id AND a.is_baseline WHERE a.experiment_id = $1`, first.Id)
			if champion != 1 {
				t.Error("the champion must stay the sweep's baseline")
			}
			if tt.decision == inconclusive {
				return
			}
			second := f.runSweepStep(t, 1, accepted)
			if baselineArm(second) != baselineArm(first) {
				t.Errorf("the next step is measured against setup %d, want the same baseline %d",
					baselineArm(second), baselineArm(first))
			}
		})
	}
}

func baselineArm(exp contracts.Experiment) int {
	for _, arm := range exp.Arms {
		if arm.IsBaseline {
			return arm.SetupVersionId
		}
	}
	return 0
}

func TestEveryExperimentSaysWhyItWasDriven(t *testing.T) {
	f := newFixture(t, "exp_purpose")

	exp := f.propose(t, -1)

	if exp.Purpose == nil || *exp.Purpose != contracts.ExperimentPurposeImprove {
		t.Errorf("a proposal's purpose = %v, want improve", exp.Purpose)
	}
	if got := purposeOf(draft{kind: contracts.ExperimentKindRecheck}); got != contracts.ExperimentPurposeRecheck {
		t.Errorf("a re-check's purpose = %s", got)
	}
}

func TestRevertingAMeasureOnlyExperimentSaysNothingWasDecided(t *testing.T) {
	f := newFixture(t, "exp_measure_only_revert")
	step := f.runSweepStep(t, -1, inconclusive)

	reverted, err := f.service.Revert(t.Context(), int64(step.Id))
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}

	last := reverted.Events[len(reverted.Events)-1]
	if last.To != "reverted" || last.Reason != measureOnlyRevert {
		t.Errorf("last event %s: %q, want %q", last.To, last.Reason, measureOnlyRevert)
	}
}

func TestAMeasureOnlyExperimentIsAcceptedWithoutBeingPromoted(t *testing.T) {
	f := newFixture(t, "exp_measure_only")
	exp, err := f.service.Create(t.Context(), contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: -1, Statement: "Grip-hint sweep step.",
		Predictions: []contracts.PredictionSpec{}, MeasureOnly: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if exp.Purpose == nil || *exp.Purpose != contracts.ExperimentPurposeSweep {
		t.Fatalf("purpose = %v, want sweep", exp.Purpose)
	}
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	judged := f.judge(t, exp.Id, accepted)

	if got := path(judged); got[len(got)-1] != "reverted" {
		t.Errorf("events = %v, want the baseline back after acceptance", got)
	}
	champion := f.count(t, `SELECT count(*) FROM champion c JOIN experiment_arm a
		ON a.setup_version_id = c.setup_version_id AND a.is_baseline WHERE a.experiment_id = $1`, exp.Id)
	if champion != 1 {
		t.Error("the champion must stay the baseline")
	}
}

func TestTheResponseRequestCarriesTheVerdictsPasses(t *testing.T) {
	f := newFixture(t, "exp_response_request")
	exp := f.runSweepStep(t, -1, rejected)
	f.engine.decisions = nil
	if _, err := f.database.ExecContext(t.Context(),
		`UPDATE setup_parameter SET role = 'arb', axle = 'F' WHERE name = 'ARB_FRONT'`); err != nil {
		t.Fatal(err)
	}

	request, err := f.service.ResponseRequest(t.Context(), int64(exp.Id))
	if err != nil {
		t.Fatalf("ResponseRequest: %v", err)
	}

	if len(request.Passes) != len(f.engine.evaluated[0].Passes) || len(request.Corners) != 7 {
		t.Fatalf("%d passes (verdict %d), %d corners", len(request.Passes), len(f.engine.evaluated[0].Passes),
			len(request.Corners))
	}
	if len(request.Sessions) != 1 || request.Sessions[0].Session != f.sessionID || request.TrackLengthM <= 0 {
		t.Errorf("sessions %+v, length %v", request.Sessions, request.TrackLengthM)
	}
	if request.ItemRole == nil || *request.ItemRole != "arb" || *request.ItemAxle != "F" {
		t.Errorf("item %v %v", request.ItemRole, request.ItemAxle)
	}
	for _, pass := range request.Passes {
		if pass.Seq < 1 || pass.DurationS != 5 || pass.Lap == nil || pass.Covariates["fuel_l"] == 0 {
			t.Fatalf("pass %+v", pass)
		}
	}
}
