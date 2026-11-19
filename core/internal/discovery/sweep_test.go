package discovery

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

const (
	insufficient = contracts.VerdictSummaryFindingClassInsufficientEvidence
	neutral      = contracts.VerdictSummaryFindingClassNeutral
	stopNeutral  = contracts.VerdictSummaryRecommendationStopNeutral
	cont         = contracts.VerdictSummaryRecommendationContinue
	larger       = contracts.VerdictSummaryRecommendationLargerPerturbation
)

// measure drives the newest step and has the engine find it neutral.
func (f *fixture) measure(id int) {
	f.t.Helper()
	ctx := context.Background()
	exp := newest(f.read(id))
	f.ledger.drive(exp)
	f.service.AdvanceAll(ctx) // evaluate
	f.ledger.judge(exp, neutral, stopNeutral)
	f.service.AdvanceAll(ctx) // revert: the baseline goes back
	f.service.AdvanceAll(ctx) // read the response, start the next step or classify
}

func TestASweepDrivesEveryStepAgainstTheBaselineAndClassifiesTheCurve(t *testing.T) {
	f := newFixture(t, "discovery_sweep")

	sweep := f.sweep("ARB_REAR", -1, 1, 2, 3)

	if !slices.Equal(sweep.Planned, []int{-1, 1, 2}) || len(sweep.Skipped) != 1 ||
		!strings.Contains(sweep.Skipped[0], "beyond the maximum 5: clipped to +2, already a step") {
		t.Fatalf("planned %v, skipped %q: +3 from 3 is past the maximum 5", sweep.Planned, sweep.Skipped)
	}
	for range sweep.Planned {
		f.measure(sweep.Id)
	}

	want := []string{"create 1", "plan 1", "start 1", "evaluate 1", "revert 1", "response 1",
		"create 2", "plan 2", "start 2", "evaluate 2", "revert 2", "response 2",
		"create 3", "plan 3", "start 3", "evaluate 3", "revert 3", "response 3"}
	if !slices.Equal(f.ledger.actions, want) {
		t.Errorf("actions %v\nwant    %v", f.ledger.actions, want)
	}
	for i, request := range f.ledger.created {
		if request.Parameter != "ARB_REAR" || request.Clicks != sweep.Planned[i] ||
			request.Driver != contracts.ExperimentCreateDriverAi || *request.AiHintShare != 0.93 {
			t.Errorf("step %d created %+v", i, request)
		}
	}
	if !strings.HasPrefix(f.ledger.reasons[0], "Sweep 1 measured step -1 and put the baseline back") {
		t.Errorf("the ledger must say the sweep decided: %q", f.ledger.reasons)
	}
	sweep = f.read(sweep.Id)
	if sweep.State != contracts.SweepStateDone || sweep.Curve == nil || len(sweep.Curve.Curves) != 1 {
		t.Fatalf("sweep %s: %q, curve %+v", sweep.State, sweep.Message, sweep.Curve)
	}
	checkCurveRequest(t, f.engine.curves)
}

// checkCurveRequest checks the engine was asked once, with time over all corners at every
// step (at the step's look) and only the response cell the item's row expects.
func checkCurveRequest(t *testing.T, curves []contracts.DiscoveryCurveRequest) {
	t.Helper()
	if len(curves) != 1 || len(curves[0].Observables) != 2 {
		t.Fatalf("curve requests %+v", curves)
	}
	time, slip := curves[0].Observables[0], curves[0].Observables[1]
	if time.Metric != "corner_time_s" || len(time.Steps) != 3 || *time.Steps[0].Look != 1 || *time.Margin != 0.005 {
		t.Errorf("time %+v", time)
	}
	if slip.Metric != "rear_slip_deg" || slip.Source != contracts.CurveObservableSourceResponse || len(slip.Steps) != 3 {
		t.Errorf("rear slip %+v", slip)
	}
}

func TestAStepGoesOnToTheNextLookOnlyWhenTheEngineSaysContinue(t *testing.T) {
	f := newFixture(t, "discovery_extend")
	sweep := f.sweep("ARB_REAR", 1, 2)
	first := newest(sweep)
	ctx := context.Background()

	f.ledger.drive(first)
	f.service.AdvanceAll(ctx)
	f.ledger.judge(first, insufficient, cont)
	f.service.AdvanceAll(ctx) // extend
	f.ledger.drive(first)
	f.service.AdvanceAll(ctx)
	f.ledger.judge(first, insufficient, larger)
	f.service.AdvanceAll(ctx) // a sweep measures, it does not chase: revert and move on
	f.service.AdvanceAll(ctx)

	want := []string{"create 1", "plan 1", "start 1", "evaluate 1", "extend 1", "evaluate 1", "revert 1",
		"response 1", "create 2", "plan 2", "start 2"}
	if !slices.Equal(f.ledger.actions, want) {
		t.Errorf("actions %v\nwant    %v", f.ledger.actions, want)
	}
	if f.ledger.created[1].Clicks != 2 {
		t.Errorf("the second step is %+d, want +2 as planned (never a larger one)", f.ledger.created[1].Clicks)
	}
}

func TestALinkedGroupIsSweptAsOneSetting(t *testing.T) {
	f := newFixture(t, "discovery_linked")

	sweep := f.sweep("WING_9", -1, 1, 2)

	if sweep.Parameter != "WING_1" || f.ledger.created[0].Parameter != "WING_1" {
		t.Errorf("swept %s, created %+v: the rear wing group goes under WING_1", sweep.Parameter, f.ledger.created)
	}
	if !slices.Equal(sweep.Planned, []int{-1, 1}) {
		t.Errorf("planned %v, want -1 and +1 from 1 in 0..2 (+2 clipped to +1, already a step)", sweep.Planned)
	}
}

func TestASweepNeedsAPinnedHintAndAFreeCarWithAVerifiedBaseline(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(f *fixture)
		request contracts.SweepCreate
		want    error
	}{
		{"no hint", nil, contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{1}}, experiments.ErrInvalid},
		{"a hint out of range", nil, contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{1}, AiHintShare: 1.6},
			experiments.ErrInvalid},
		{"no such item", nil, contracts.SweepCreate{Parameter: "ARB_FRONT", Steps: []int{1}, AiHintShare: 0.93},
			experiments.ErrInvalid},
		{"no possible step", nil, contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{0}, AiHintShare: 0.93},
			experiments.ErrInvalid},
		{"an experiment on the car", func(f *fixture) { f.experimentRow(experiments.Running, "ai") },
			contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{1}, AiHintShare: 0.93}, experiments.ErrConflict},
		{"a training program on the car", func(f *fixture) {
			f.exec(`INSERT INTO training_program (session_id, car_id, track_id, target) VALUES ($1, $2, $3, 2)`,
				f.session, f.car, f.track)
		}, contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{1}, AiHintShare: 0.93}, experiments.ErrConflict},
		{"a baseline never read back", func(f *fixture) { f.exec(`UPDATE setup_version SET agent_hash = NULL`) },
			contracts.SweepCreate{Parameter: "ARB_REAR", Steps: []int{1}, AiHintShare: 0.93}, experiments.ErrConflict},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, "discovery_refuse_"+string(rune('a'+i)))
			if tt.prepare != nil {
				tt.prepare(f)
			}
			_, err := f.service.Start(context.Background(), f.session, tt.request)
			if !errors.Is(err, tt.want) || len(f.ledger.created) != 0 {
				t.Errorf("err = %v, want %v; created %d", err, tt.want, len(f.ledger.created))
			}
		})
	}
}

func TestOneSweepPerCarAndStop(t *testing.T) {
	f := newFixture(t, "discovery_stop")
	sweep := f.sweep("ARB_REAR", 1)
	ctx := context.Background()

	_, err := f.service.Start(ctx, f.session, contracts.SweepCreate{Parameter: "WING_1", Steps: []int{1}, AiHintShare: 0.93})
	if !errors.Is(err, experiments.ErrConflict) {
		t.Errorf("a second sweep on the car: %v", err)
	}
	stopped, err := f.service.Stop(ctx, int64(sweep.Id))
	if err != nil || stopped.State != contracts.SweepStateStopped {
		t.Fatalf("stop: %v, %s", err, stopped.State)
	}
	if _, err := f.service.Stop(ctx, int64(sweep.Id)); !errors.Is(err, experiments.ErrConflict) {
		t.Errorf("stopping twice: %v", err)
	}
	if _, err := f.service.Stop(ctx, 999); !errors.Is(err, experiments.ErrNotFound) {
		t.Errorf("stopping no sweep: %v", err)
	}
	list, err := f.service.List(ctx)
	if err != nil || len(list) != 1 || list[0].Steps[0].Clicks != 1 {
		t.Errorf("list: %v %+v", err, list)
	}
}

func TestASweepWaitsWhileAnotherExperimentIsOnTheCar(t *testing.T) {
	f := newFixture(t, "discovery_wait")
	f.ledger.occupied = true

	sweep := f.sweep("ARB_REAR", 1)

	if sweep.State != contracts.SweepStateWaiting || !strings.Contains(sweep.Message, "Waiting to start") {
		t.Errorf("sweep %s: %q", sweep.State, sweep.Message)
	}
}
