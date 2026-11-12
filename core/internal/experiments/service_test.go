package experiments

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
)

func TestCreateProposesOneKnobAgainstTheSessionsSetup(t *testing.T) {
	f := newFixture(t, "exp_create")

	exp := f.propose(t, -1)

	if exp.State != Proposed || exp.Kind != contracts.ExperimentKindOfat {
		t.Errorf("state %s, kind %s", exp.State, exp.Kind)
	}
	if exp.Parameter != "ARB_FRONT" || exp.ParameterLabel != "ARB Front" || exp.Clicks != -1 {
		t.Errorf("parameter %s (%s), %d clicks", exp.Parameter, exp.ParameterLabel, exp.Clicks)
	}
	if exp.CarName != "Mazda MX-5 Cup" || len(exp.Predictions) != 1 || len(exp.Guardrails) != 2 {
		t.Errorf("car %q, %d predictions, %d guardrails", exp.CarName, len(exp.Predictions), len(exp.Guardrails))
	}
	if exp.SuccessTest != defaultTest {
		t.Errorf("success test = %+v", exp.SuccessTest)
	}
	if got := path(exp); !slices.Equal(got, []string{"proposed"}) {
		t.Errorf("events = %v", got)
	}
	values := map[bool]float64{}
	for _, arm := range exp.Arms {
		values[arm.IsBaseline] = arm.Value
		if !strings.HasPrefix(arm.Label, "Setup ") || strings.ContainsAny(arm.Label[6:], "ABIO") {
			t.Errorf("code name %q", arm.Label)
		}
	}
	if len(exp.Arms) != 2 || exp.Arms[0].Label >= exp.Arms[1].Label {
		t.Fatalf("arms = %+v, want two in code-name order", exp.Arms)
	}
	if values[true] != 3 || values[false] != 2 {
		t.Errorf("baseline %v, variant %v, want 3 and 2", values[true], values[false])
	}
	lineage := f.count(t, `SELECT count(*) FROM setup_version v JOIN setup_version parent ON parent.id = v.parent_id
		WHERE v.created_by = $1 AND parent.created_by = 'agent'`, fmt.Sprintf("experiment %d", exp.Id))
	if lineage != 1 {
		t.Errorf("variant setup with the baseline as parent: %d, want 1", lineage)
	}
}

func TestCreateRefusesWhatCouldNotBeDriven(t *testing.T) {
	f := newFixture(t, "exp_invalid")
	valid := contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: 1, Statement: "Stiffer front bar.",
	}
	cases := map[string]func(*contracts.ExperimentCreate){
		"no change":       func(r *contracts.ExperimentCreate) { r.Clicks = 0 },
		"out of range":    func(r *contracts.ExperimentCreate) { r.Clicks = 2 },
		"unknown item":    func(r *contracts.ExperimentCreate) { r.Parameter = "FLUX_CAPACITOR" },
		"unknown session": func(r *contracts.ExperimentCreate) { r.SessionId = 999 },
		"no statement":    func(r *contracts.ExperimentCreate) { r.Statement = "  " },
		"upside-down range": func(r *contracts.ExperimentCreate) {
			r.Predictions = []contracts.PredictionSpec{{Channel: "corner_time_s", Low: 2, High: 1}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			request := valid
			change(&request)
			if _, err := f.service.Create(t.Context(), request); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
	if stored := f.count(t, `SELECT count(*) FROM experiment`); stored != 0 {
		t.Errorf("%d refused experiments were stored", stored)
	}
}

func TestPlanStoresFourBlocksAndWhatTheyCanAnswer(t *testing.T) {
	f := newFixture(t, "exp_plan")
	exp := f.propose(t, -1)

	planned, err := f.service.Plan(t.Context(), int64(exp.Id))

	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if planned.State != Planned || planned.Plan == nil || planned.Plan.MdeDecide != 0.03 {
		t.Fatalf("state %s, plan %+v", planned.State, planned.Plan)
	}
	base := baselineLabel(t, planned)
	for i, block := range planned.Blocks {
		onBaseline := block.Label == base
		if block.Order != i+1 || block.LapsTarget != DefaultLapsPerBlock || onBaseline != (i%2 == 0) {
			t.Errorf("block %d = %+v (baseline is %s)", i+1, block, base)
		}
	}
	asked := f.engine.plans[0]
	if len(planned.Blocks) != 4 || len(asked.Corners) != 7 || asked.BaselinePasses == nil {
		t.Errorf("%d blocks; engine was asked about %d corners, passes %v",
			len(planned.Blocks), len(asked.Corners), asked.BaselinePasses)
	}
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); !errors.Is(err, ErrConflict) {
		t.Errorf("planning twice: err = %v, want ErrConflict", err)
	}
}

func TestPlanLeavesTheExperimentProposedWhenTheEngineIsDown(t *testing.T) {
	f := newFixture(t, "exp_plan_down")
	exp := f.propose(t, -1)
	f.engine.failure = engine.ErrUnavailable

	_, err := f.service.Plan(t.Context(), int64(exp.Id))

	if !errors.Is(err, engine.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if after, _ := f.service.Get(t.Context(), int64(exp.Id)); after.State != Proposed || len(after.Blocks) != 0 {
		t.Errorf("state %s with %d blocks", after.State, len(after.Blocks))
	}
}

func TestStartWritesTheRunPlanAndBothSetupsForTheAgent(t *testing.T) {
	f := newFixture(t, "exp_start")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	running, err := f.service.Start(t.Context(), id)

	if err != nil || running.State != Running {
		t.Fatalf("Start: state %s, err %v", running.State, err)
	}
	var plan contracts.RunPlan
	content, err := os.ReadFile(filepath.Join(f.outbox, RunPlanFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &plan); err != nil {
		t.Fatalf("runplan.json is not a RunPlan: %v", err)
	}
	if plan.ExperimentId != exp.Id || plan.Revision != 1 || len(plan.Blocks) != 4 {
		t.Fatalf("run plan = experiment %d, revision %d, %d blocks", plan.ExperimentId, plan.Revision, len(plan.Blocks))
	}
	base := baselineLabel(t, running)
	for _, block := range plan.Blocks {
		want := 2.0
		if block.Label == base {
			want = 3.0
		}
		if block.Values["ARB_FRONT"] != want || len(block.Values) != 35 {
			t.Errorf("block %d (%s): ARB_FRONT %v of %d values, want %v", block.Order, block.Label,
				block.Values["ARB_FRONT"], len(block.Values), want)
		}
	}
	if strings.Contains(string(content), "baseline") {
		t.Error("the run plan says which setup is the baseline")
	}
	files, _ := filepath.Glob(filepath.Join(f.outbox, "setups", "ks_mazda_mx5_cup", "*.ini"))
	if len(files) != 2 {
		t.Errorf("setup files = %v, want one per arm", files)
	}
}

func TestOnlyOneExperimentAtATimeIsOnTheCar(t *testing.T) {
	f := newFixture(t, "exp_one_at_a_time")
	first, second := f.propose(t, -1), f.propose(t, 1)
	for _, exp := range []contracts.Experiment{first, second} {
		if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.Start(t.Context(), int64(first.Id)); err != nil {
		t.Fatal(err)
	}

	_, err := f.service.Start(t.Context(), int64(second.Id))

	if !errors.Is(err, ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
	if _, err := f.service.Abort(t.Context(), int64(first.Id)); err != nil {
		t.Fatal(err)
	}
	if started, err := f.service.Start(t.Context(), int64(second.Id)); err != nil || started.State != Running {
		t.Errorf("after the first was aborted: state %s, err %v", started.State, err)
	}
}

func TestUnknownExperimentsAreNotFound(t *testing.T) {
	f := newFixture(t, "exp_missing")
	if _, err := f.service.Get(t.Context(), 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: err = %v, want ErrNotFound", err)
	}
	if _, err := f.service.Start(t.Context(), 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("Start: err = %v, want ErrNotFound", err)
	}
	if list, err := f.service.List(t.Context()); err != nil || list == nil || len(list) != 0 {
		t.Errorf("List = %v, %v, want an empty list", list, err)
	}
}
