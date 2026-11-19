package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// blockOnTheCar gives the experiment a first block with a run context: the session's base
// hint 1.1 and the hints the block's passes were driven with.
func (f *fixture) blockOnTheCar(experiment int64, low, high float64) {
	f.t.Helper()
	arm := f.id(`INSERT INTO experiment_arm (experiment_id, setup_version_id, label_code, is_baseline)
	             VALUES ($1, $2, 'Setup C', true) RETURNING id`, experiment, f.setup)
	block := f.id(`INSERT INTO run_block (experiment_id, arm_id, "order", laps_target) VALUES ($1, $2, 1, 3)
	               RETURNING id`, experiment, arm)
	f.exec(`INSERT INTO run_context (session_id, ai_tyre_hint_base) VALUES ($1, 1.1)
	        ON CONFLICT (session_id, block_id) DO NOTHING`, f.session)
	f.exec(`INSERT INTO run_context (session_id, block_id, ai_pinned_share, ai_hint_min, ai_hint_max, passes)
	        VALUES ($1, $2, 0.93, $3, $4, 12)`, f.session, block, low, high)
}

func TestASweepStopsAStepItCannotTrustAndSaysWhy(t *testing.T) {
	invalid := contracts.VerdictSummaryFindingClassInvalid
	tests := []struct {
		name    string
		prepare func(f *fixture, step int64)
		want    string
	}{
		{"an invalid finding", func(f *fixture, step int64) {
			f.ledger.drive(step)
			f.service.AdvanceAll(context.Background())
			f.ledger.judge(step, invalid, contracts.VerdictSummaryRecommendationStopInvalid)
		}, "is invalid: The engine says so."},
		{"a step that was aborted", func(f *fixture, step int64) {
			f.ledger.states[step].State = experiments.Aborted
		}, "was aborted"},
		{"the AI stopped driving", func(f *fixture, step int64) {
			f.exec(`UPDATE sweep_step SET added_at = now() - interval '1 hour' WHERE experiment_id = $1`, step)
		}, "The AI stopped driving step +1"},
		{"a block at another hint", func(f *fixture, step int64) {
			f.blockOnTheCar(step, 1.023, 1.1)
		}, "driven at AI tyre hints 1.0230 to 1.1000, the sweep pins 1.0230"},
		{"the baseline moved", func(f *fixture, step int64) {
			f.ledger.drive(step)
			f.service.AdvanceAll(context.Background())
			f.ledger.judge(step, neutral, stopNeutral)
			f.service.AdvanceAll(context.Background())
			f.ledger.baseline = f.setup + 1
		}, "The baseline moved"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, "discovery_fail_"+string(rune('a'+i)))
			sweep := f.sweep("ARB_REAR", 1, 2)
			tt.prepare(f, newest(sweep))

			f.service.AdvanceAll(context.Background())

			sweep = f.read(sweep.Id)
			if sweep.State != contracts.SweepStateFailed || !strings.Contains(sweep.Message, tt.want) {
				t.Errorf("sweep %s: %q, want it to say %q", sweep.State, sweep.Message, tt.want)
			}
			if len(f.ledger.created) != 1 {
				t.Errorf("%d steps created: a failed sweep starts nothing", len(f.ledger.created))
			}
		})
	}
}

func TestAnInvalidStepIsPutBackBeforeTheSweepStops(t *testing.T) {
	f := newFixture(t, "discovery_invalid_revert")
	sweep := f.sweep("ARB_REAR", 1, 2)
	step := newest(sweep)
	f.ledger.drive(step)
	f.service.AdvanceAll(context.Background())
	f.ledger.judge(step, contracts.VerdictSummaryFindingClassInvalid, "")

	f.service.AdvanceAll(context.Background())

	if f.ledger.states[step].State != experiments.Reverted || !strings.Contains(f.ledger.reasons[0], "is invalid") {
		t.Errorf("step %s, reasons %q", f.ledger.states[step].State, f.ledger.reasons)
	}
}

func TestBlocksDrivenAtThePinnedHintPass(t *testing.T) {
	f := newFixture(t, "discovery_hint_ok")
	sweep := f.sweep("ARB_REAR", 1)
	f.blockOnTheCar(newest(sweep), 1.023, 1.0232)

	f.service.AdvanceAll(context.Background())

	sweep = f.read(sweep.Id)
	if sweep.State != contracts.SweepStateRunning || !strings.Contains(sweep.Message, "The AI is driving step +1") {
		t.Errorf("sweep %s: %q", sweep.State, sweep.Message)
	}
}

func TestTheCurvesWaitForTheEngine(t *testing.T) {
	f := newFixture(t, "discovery_engine_down")
	f.engine.failure = errors.New("engine unavailable")
	sweep := f.sweep("ARB_REAR", 1)

	f.measure(sweep.Id)

	sweep = f.read(sweep.Id)
	if sweep.State != contracts.SweepStateRunning || !strings.Contains(sweep.Message, "waiting for the engine") {
		t.Errorf("sweep %s: %q", sweep.State, sweep.Message)
	}
	f.engine.failure = nil
	f.service.AdvanceAll(context.Background())
	if sweep = f.read(sweep.Id); sweep.State != contracts.SweepStateDone || f.engine.responses != 1 {
		t.Errorf("sweep %s after the engine came back, %d response reads", sweep.State, f.engine.responses)
	}
}
