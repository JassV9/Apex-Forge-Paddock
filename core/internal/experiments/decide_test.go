package experiments

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const (
	accepted     = contracts.EvaluateResponseDecisionAccepted
	rejected     = contracts.EvaluateResponseDecisionRejected
	tradeoff     = contracts.EvaluateResponseDecisionTradeoff
	inconclusive = contracts.EvaluateResponseDecisionInconclusive
)

func readPlan(t *testing.T, f *fixture) contracts.RunPlan {
	t.Helper()
	var plan contracts.RunPlan
	content, err := os.ReadFile(filepath.Join(f.outbox, RunPlanFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &plan); err != nil {
		t.Fatalf("runplan.json: %v", err)
	}
	return plan
}

func TestAcceptedExperimentKeepsItsResultsAndBecomesChampion(t *testing.T) {
	f := newFixture(t, "exp_accepted")

	exp := f.run(t, -1, accepted)

	if got := path(exp); !slices.Equal(got, []string{"proposed", "planned", "running", "evaluating", "accepted"}) {
		t.Errorf("events = %v", got)
	}
	if exp.Verdict == nil || exp.Verdict.Decision != contracts.VerdictSummaryDecisionAccepted ||
		exp.Verdict.DecidedBy != contracts.VerdictSummaryDecidedByEngine || exp.Verdict.PredictionHits != 1 {
		t.Errorf("verdict = %+v", exp.Verdict)
	}
	if len(exp.Results) != 1 || exp.Results[0].Estimate != -0.03 {
		t.Errorf("results = %+v", exp.Results)
	}
	for _, block := range exp.Blocks {
		if block.LapsDone != DefaultLapsPerBlock || block.Passes != 7*(DefaultLapsPerBlock+1) {
			t.Errorf("block %d: %d laps, %d passes", block.Order, block.LapsDone, block.Passes)
		}
	}
	champion := f.count(t, `SELECT count(*) FROM champion c JOIN experiment_arm a
		ON a.setup_version_id = c.setup_version_id AND NOT a.is_baseline WHERE a.experiment_id = $1`, exp.Id)
	if champion != 1 {
		t.Error("the accepted variant is not the champion")
	}
	if plan := readPlan(t, f); len(plan.Blocks) != 4 {
		t.Errorf("run plan has %d blocks: the variant should simply stay on the car", len(plan.Blocks))
	}
}

func TestOnlyPushLapsWithFeaturesReachTheEngine(t *testing.T) {
	f := newFixture(t, "exp_passes")

	exp := f.run(t, -1, accepted)

	sent := f.engine.evaluated[0]
	if len(sent.Passes) != 4*DefaultLapsPerBlock*7 {
		t.Fatalf("%d passes sent, want 4 blocks x %d laps x 7 corners (no out-laps)", len(sent.Passes), DefaultLapsPerBlock)
	}
	base := baselineLabel(t, exp)
	arms := map[int]string{}
	for _, block := range exp.Blocks {
		arms[block.Order] = block.Label
	}
	for _, pass := range sent.Passes {
		onBaseline := pass.Arm == contracts.EvaluatePassArmBaseline
		if onBaseline != (arms[pass.Block] == base) || pass.Lap == nil || *pass.Lap/lapsPerBlockCode != pass.Block {
			t.Fatalf("pass %+v does not match its block", pass)
		}
		if pass.Covariates["tyre_core_c"] != 81.5 || pass.Covariates["fuel_l"] == 0 || pass.Metrics["corner_time_s"] == nil {
			t.Fatalf("pass conditions = %+v, metrics %+v", pass.Covariates, pass.Metrics)
		}
	}
	if len(sent.Predictions) != 1 || len(sent.Guardrails) != 2 || len(sent.DriverChannels) != 3 {
		t.Errorf("engine got %d predictions, %d guardrails, %d driver channels",
			len(sent.Predictions), len(sent.Guardrails), len(sent.DriverChannels))
	}
}

func TestEvaluateWaitsForFeatures(t *testing.T) {
	f := newFixture(t, "exp_pending")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Evaluate(t.Context(), id); !errors.Is(err, ErrConflict) {
		t.Errorf("evaluating a planned experiment: err = %v, want ErrConflict", err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	f.drive(t, exp.Id)
	_, err := f.database.ExecContext(t.Context(),
		`UPDATE corner_pass SET features = '{}' WHERE block_lap = 2 AND block_id IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.service.Evaluate(t.Context(), id)

	if !errors.Is(err, ErrConflict) || len(f.engine.evaluated) != 0 {
		t.Errorf("err = %v after %d engine calls, want ErrConflict and none", err, len(f.engine.evaluated))
	}
}

func TestRejectedExperimentPutsTheBaselineBackOnTheCar(t *testing.T) {
	f := newFixture(t, "exp_rejected")

	exp := f.run(t, -1, rejected)

	want := []string{"proposed", "planned", "running", "evaluating", "rejected", "reverted"}
	if got := path(exp); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if exp.Verdict.Decision != contracts.VerdictSummaryDecisionRejected {
		t.Errorf("verdict = %+v: the engine's rejection must stay on record", exp.Verdict)
	}
	plan := readPlan(t, f)
	last := plan.Blocks[len(plan.Blocks)-1]
	if plan.Revision != 2 || len(plan.Blocks) != 5 || last.LapsTarget != 0 ||
		last.Label != baselineLabel(t, exp) || last.Values["ARB_FRONT"] != 3 {
		t.Errorf("revision %d, %d blocks, last block %+v", plan.Revision, len(plan.Blocks), last)
	}
	if champion, _ := championValue(t, f); champion != 3 {
		t.Errorf("champion ARB_FRONT %v: a rejected variant became champion", champion)
	}
}

func TestInconclusiveExperimentIsExtendedTwiceAndThenReverts(t *testing.T) {
	f := newFixture(t, "exp_extend")
	exp := f.run(t, -1, inconclusive)
	id := int64(exp.Id)
	if exp.State != Inconclusive {
		t.Fatalf("state %s", exp.State)
	}

	for extension := 1; extension <= MaxExtensions; extension++ {
		extended, err := f.service.Extend(t.Context(), id)
		if err != nil || extended.State != Running || extended.Extensions != extension {
			t.Fatalf("extension %d: state %s, %d extensions, err %v", extension, extended.State, extended.Extensions, err)
		}
		if blocks := len(extended.Blocks); blocks != 4+2*extension {
			t.Fatalf("extension %d: %d blocks", extension, blocks)
		}
		exp = f.judge(t, exp.Id, inconclusive)
	}

	if exp.State != Reverted {
		t.Fatalf("state %s after the last extension, want reverted: ties go to the baseline", exp.State)
	}
	if judged := f.engine.evaluated[2].Passes; len(judged) != 8*DefaultLapsPerBlock*7 {
		t.Errorf("the last verdict was on %d passes, want all 8 blocks", len(judged))
	}
	if _, err := f.service.Extend(t.Context(), id); !errors.Is(err, ErrConflict) {
		t.Errorf("extending a reverted experiment: err = %v, want ErrConflict", err)
	}
	plan := readPlan(t, f)
	if last := plan.Blocks[len(plan.Blocks)-1]; len(plan.Blocks) != 9 || last.LapsTarget != 0 {
		t.Errorf("%d blocks, last %+v: want the baseline restored after block 8", len(plan.Blocks), last)
	}
}

func TestDriverCanKeepTheBaselineOnAnInconclusiveResult(t *testing.T) {
	f := newFixture(t, "exp_revert")
	exp := f.run(t, -1, inconclusive)

	reverted, err := f.service.Revert(t.Context(), int64(exp.Id))

	if err != nil || reverted.State != Reverted {
		t.Fatalf("state %s, err %v", reverted.State, err)
	}
	verdict := reverted.Verdict
	if verdict.Decision != contracts.VerdictSummaryDecisionReverted ||
		verdict.DecidedBy != contracts.VerdictSummaryDecidedByDriver || verdict.PredictionTotal != 1 {
		t.Errorf("verdict = %+v", verdict)
	}
	if _, err := f.service.Revert(t.Context(), int64(exp.Id)); !errors.Is(err, ErrConflict) {
		t.Errorf("reverting twice: err = %v, want ErrConflict", err)
	}
}

func TestDriverDecidesATradeoff(t *testing.T) {
	cases := map[string]struct {
		accept   bool
		state    State
		champion float64 // ARB_FRONT of the champion afterwards; the baseline has 3
	}{
		"accepts":  {true, Accepted, 2},
		"declines": {false, Reverted, 3},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, "exp_tradeoff_"+name)
			exp := f.run(t, -1, tradeoff)
			if exp.State != Tradeoff {
				t.Fatalf("state %s", exp.State)
			}

			decided, err := f.service.Decide(t.Context(), int64(exp.Id), tc.accept)

			if err != nil || decided.State != tc.state {
				t.Fatalf("state %s, err %v", decided.State, err)
			}
			if decided.Verdict.DecidedBy != contracts.VerdictSummaryDecidedByDriver {
				t.Errorf("verdict = %+v", decided.Verdict)
			}
			if got, _ := championValue(t, f); got != tc.champion {
				t.Errorf("champion ARB_FRONT %v, want %v", got, tc.champion)
			}
			if _, err := f.service.Decide(t.Context(), int64(exp.Id), true); !errors.Is(err, ErrConflict) {
				t.Errorf("deciding twice: err = %v, want ErrConflict", err)
			}
		})
	}
}

func TestAbortAndDiscardKeepTheExperimentInTheLedger(t *testing.T) {
	f := newFixture(t, "exp_abort")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	if _, err := f.service.Abort(t.Context(), id); !errors.Is(err, ErrConflict) {
		t.Errorf("aborting a proposal: err = %v, want ErrConflict", err)
	}
	discarded, err := f.service.Discard(t.Context(), id)
	if err != nil || discarded.State != Discarded {
		t.Fatalf("Discard: state %s, err %v", discarded.State, err)
	}

	second := f.propose(t, 1)
	id = int64(second.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Discard(t.Context(), id); !errors.Is(err, ErrConflict) {
		t.Errorf("discarding a running experiment: err = %v, want ErrConflict", err)
	}
	aborted, err := f.service.Abort(t.Context(), id)
	if err != nil || aborted.State != Aborted || aborted.Verdict.Decision != contracts.VerdictSummaryDecisionAborted {
		t.Fatalf("Abort: state %s, verdict %+v, err %v", aborted.State, aborted.Verdict, err)
	}
	if plan := readPlan(t, f); plan.Blocks[len(plan.Blocks)-1].LapsTarget != 0 {
		t.Error("the baseline is not queued to go back on the car")
	}
	if list, _ := f.service.List(t.Context()); len(list) != 2 || list[0].Id != second.Id {
		t.Errorf("ledger = %d experiments, newest %d", len(list), list[0].Id)
	}
}
