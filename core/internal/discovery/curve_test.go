package discovery

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestTheCurveRequestGathersEachObservableAcrossTheSteps(t *testing.T) {
	look, margin := 1, 0.005
	step := func(clicks int, slip *float64) measured {
		exp := contracts.Experiment{
			SuccessTest: contracts.SuccessTest{Level: 0.9},
			Guardrails:  []contracts.GuardrailSpec{{Channel: "rear_slip_peak_deg", Tolerance: 0.5}},
			Verdict:     &contracts.VerdictSummary{Look: &look, Margin: &margin},
			Results: []contracts.ExperimentResult{
				{Channel: "corner_time_s", Role: contracts.ExperimentResultRolePrimary,
					CornerClass: contracts.ExperimentResultCornerClassAll, Estimate: 0.001},
				{Channel: "corner_time_s", Role: contracts.ExperimentResultRolePrimary,
					CornerClass: contracts.ExperimentResultCornerClassSlow, Estimate: 0.002},
				{Channel: "rear_slip_peak_deg", Role: contracts.ExperimentResultRoleGuardrail,
					CornerClass: contracts.ExperimentResultCornerClassAll, Estimate: 0.1},
			},
		}
		low, high := 0.0, 0.2
		cell := contracts.ResponseEffect{Metric: "rear_slip_deg", Phase: "exit", Regime: "slow",
			Estimate: slip, CiLow: &low, CiHigh: &high, Expected: contracts.ResponseResponseEffectsElemExpectedDirect}
		return measured{clicks: clicks, exp: exp, cells: []contracts.ResponseEffect{cell}}
	}
	slip := 0.1

	request := curveRequest("ARB_REAR", []measured{step(-1, &slip), step(1, nil), step(2, &slip)})

	if request.Parameter != "ARB_REAR" || len(request.Observables) != 4 {
		t.Fatalf("observables %+v", request.Observables)
	}
	all, slow, guardrail, cell := request.Observables[0], request.Observables[1], request.Observables[2], request.Observables[3]
	if len(all.Steps) != 3 || all.Steps[1].Clicks != 1 || all.Steps[0].Look == nil || *all.Margin != 0.005 {
		t.Errorf("time over all corners %+v", all)
	}
	if slow.Regime != contracts.CurveObservableRegimeSlow || slow.Steps[0].Look != nil {
		t.Errorf("a speed class is at the test's level, not the look's: %+v", slow)
	}
	if *guardrail.Margin != 0.5 || *guardrail.Role != "guardrail" {
		t.Errorf("guardrail %+v", guardrail)
	}
	if cell.Phase != contracts.CurveObservablePhaseExit || len(cell.Steps) != 2 || cell.Margin != nil {
		t.Errorf("a cell without an estimate at a step has no point there: %+v", cell)
	}
}

func TestOnlyTheExpectedCellsAreKept(t *testing.T) {
	cells := expectedCells([]contracts.ResponseEffect{
		{Metric: "a", Expected: contracts.ResponseResponseEffectsElemExpectedDirect},
		{Metric: "b", Expected: contracts.ResponseResponseEffectsElemExpectedNone},
		{Metric: "c", Expected: contracts.ResponseResponseEffectsElemExpectedProxy},
	})
	if len(cells) != 2 || cells[0].Metric != "a" || cells[1].Metric != "c" {
		t.Errorf("kept %+v", cells)
	}
}
