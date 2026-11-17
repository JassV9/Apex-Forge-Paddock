package experiments

import (
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestThePlansReasonSaysWhatLapsScatterAndWhenTheTargetIsOutOfReach(t *testing.T) {
	blocks := func(n int) []contracts.PlanBlock {
		list := make([]contracts.PlanBlock, n)
		for i := range list {
			list[i] = contracts.PlanBlock{Order: i + 1, LapsTarget: 3}
		}
		return list
	}
	cases := []struct {
		name string
		plan contracts.PlanResponse
		want []string
		not  []string
	}{
		{"standard plan, laps not measured",
			contracts.PlanResponse{Blocks: blocks(4), MdeDecide: 0.0157, SigmaSource: "measured",
				SigmaLapSource: contracts.PlanResponseSigmaLapSourceNone, TargetMet: true},
			[]string{"4 blocks of 3 laps", "0.0157", "lap scatter not measured"}, []string{"Short of"}},
		{"a noisy car gets more blocks",
			contracts.PlanResponse{Blocks: blocks(6), MdeDecide: 0.019, SigmaSource: "measured", SigmaLap: 0.006,
				SigmaLapSource: contracts.PlanResponseSigmaLapSourcePlanned, MdeTarget: 0.02, TargetMet: true},
			[]string{"6 blocks of 3 laps", "whole laps scatter 0.006, from planned laps"}, []string{"Short of"}},
		{"even the longest plan falls short (FH26)",
			contracts.PlanResponse{Blocks: blocks(8), MdeDecide: 0.0326, SigmaSource: "measured", SigmaLap: 0.0233,
				SigmaLapSource: contracts.PlanResponseSigmaLapSourceFree, MdeTarget: 0.0232, TargetMet: false},
			[]string{"8 blocks of 3 laps", "from free laps", "Short of the 0.0232 it was sized for"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason := planReason(c.plan, "corner_time_s")
			for _, want := range c.want {
				if !strings.Contains(reason, want) {
					t.Errorf("%q does not say %q", reason, want)
				}
			}
			for _, not := range c.not {
				if strings.Contains(reason, not) {
					t.Errorf("%q says %q", reason, not)
				}
			}
		})
	}
}

// An earlier experiment's block on the baseline: an out-lap and two push laps in 3 corners.
const baselineBlockSQL = `
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s, features, block_id,
                         block_lap, lap_in_session, setup_version_id)
SELECT $1, c.id, 900000 + lap * 10 + c.number, now() + make_interval(secs => lap * 10 + c.number),
       5.0, '{"metrics": {"corner_time_s": 5.0}}'::jsonb, b.id, lap, 40 + lap, e.baseline_version_id
FROM experiment e JOIN run_block b ON b.experiment_id = e.id AND b."order" = 1
CROSS JOIN generate_series(0, 2) AS lap
JOIN corner c ON c.number <= 3 AND c.track_id = (SELECT track_id FROM session WHERE id = $1)
WHERE e.id = $2`

func TestThePlannerGetsEachBaselinePassWithItsLap(t *testing.T) {
	f := newFixture(t, "exp_plan_laps")
	earlier := f.propose(t, -1)
	if _, err := f.service.Plan(t.Context(), int64(earlier.Id)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.ExecContext(t.Context(), baselineBlockSQL, f.sessionID, earlier.Id); err != nil {
		t.Fatal(err)
	}
	next := f.propose(t, 1)
	if _, err := f.service.Plan(t.Context(), int64(next.Id)); err != nil {
		t.Fatal(err)
	}
	laps, outLap := map[int]int{}, 0
	for _, pass := range f.engine.plans[1].BaselinePasses {
		if pass.Lap == nil {
			outLap++
			continue
		}
		if pass.Planned == nil || !*pass.Planned {
			t.Errorf("pass %+v of a block's push lap is not marked planned", pass)
		}
		laps[*pass.Lap]++
	}
	if outLap != 3 || len(laps) != 2 || laps[1] != 3 || laps[2] != 3 {
		t.Errorf("%d out-lap passes without a lap, push laps %v; want 3, and laps 1 and 2 with 3 passes each", outLap, laps)
	}
}
