package experiments

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// slipUp is a response finding the engine measured, and one it could not estimate.
func slipUp() []contracts.ResponseFinding {
	estimate, low, high := 0.12, 0.08, 0.16
	return []contracts.ResponseFinding{
		{Metric: "rear_slip_deg", Phase: contracts.ResponseFindingPhaseMid, Regime: contracts.ResponseFindingRegimeAll,
			Expected: contracts.ResponseFindingExpectedDirect, Finding: contracts.ResponseFindingFindingIncrease,
			Estimate: &estimate, CiLow: &low, CiHigh: &high, Margin: 0.05, NPasses: 168, Corners: 7},
		{Metric: "yaw_gain", Phase: contracts.ResponseFindingPhaseExit, Regime: contracts.ResponseFindingRegimeSlow,
			Expected: contracts.ResponseFindingExpectedProxy, Finding: contracts.ResponseFindingFindingInsufficient,
			Margin: 0.005, NPasses: 12, Corners: 2},
	}
}

func TestTheEngineIsToldWhereTheRawSamplesAreInTheSameRequest(t *testing.T) {
	f := newFixture(t, "exp_vehicle_request")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	f.drive(t, exp.Id)
	spoil(t, f, id)
	f.engine.decisions = append(f.engine.decisions, inconclusive)

	if _, err := f.service.Evaluate(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	sent := f.engine.evaluated[0].Response
	if sent == nil || sent.ItemRole == nil || *sent.ItemRole != "arb" || *sent.ItemAxle != "F" ||
		len(sent.Corners) != 7 || sent.TrackLengthM <= 0 || len(sent.Sessions) != 1 {
		t.Fatalf("response context %+v", sent)
	}
	// Off-track passes are sent (they may carry the previous pass's traction), never compared.
	if len(sent.Passes) != 4*DefaultLapsPerBlock*7-2 {
		t.Errorf("%d passes for the response, want every push pass but the 2 on another setup or driver", len(sent.Passes))
	}
	compared := map[[3]int]bool{}
	for _, pass := range f.engine.evaluated[0].Passes {
		compared[[3]int{pass.Block, pass.Corner, *pass.Lap}] = true
	}
	invalid, matched := 0, 0
	for _, pass := range sent.Passes {
		if pass.Lap == nil || *pass.Lap/lapsPerBlockCode != pass.Block || pass.DurationS <= 0 || pass.Seq < 1 {
			t.Fatalf("pass %+v", pass)
		}
		if !pass.Valid {
			invalid++
		} else if compared[[3]int{pass.Block, pass.Corner, *pass.Lap}] {
			matched++
		}
	}
	if invalid != 2 || matched != len(f.engine.evaluated[0].Passes) {
		t.Errorf("%d off-track passes sent (want 2), %d share a compared pass's lap code", invalid, matched)
	}
}

func TestResponseFindingsAreKeptWithTheirVerdictApartFromTheResults(t *testing.T) {
	f := newFixture(t, "exp_vehicle_rows")
	f.engine.responses = [][]contracts.ResponseFinding{slipUp()}
	first := f.run(t, -1, inconclusive)

	if len(first.Responses) != 1 || len(first.Results) != 1 || first.Results[0].Role != contracts.ExperimentResultRolePrimary {
		t.Fatalf("responses %+v, results %+v", first.Responses, first.Results)
	}
	got := first.Responses[0]
	if got.Metric != "rear_slip_deg" || got.Phase != contracts.ExperimentResponsePhaseMid ||
		got.Regime != contracts.ExperimentResponseRegimeAll || got.Finding != contracts.ExperimentResponseFindingIncrease ||
		got.Expected != contracts.ExperimentResponseExpectedDirect || got.Margin != 0.05 || got.CiLow != 0.08 {
		t.Errorf("stored response %+v", got)
	}
	id := int64(first.Id)
	if n := f.count(t, `SELECT count(*) FROM result WHERE experiment_id = $1 AND role = 'response'
	                    AND effect_class = 'positive' AND engine_version = 'test-engine'`, id); n != 1 {
		t.Errorf("%d response rows, want 1", n)
	}
	if _, err := f.service.Extend(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	second := f.judge(t, first.Id, inconclusive)

	if len(second.Responses) != 0 || len(second.Results) != 1 {
		t.Errorf("the latest verdict measured no response, yet shows %+v", second.Responses)
	}
	if n := f.count(t, `SELECT count(*) FROM result WHERE experiment_id = $1 AND role = 'response'`, id); n != 1 {
		t.Errorf("%d response rows after a second verdict, want the first verdict's 1 kept", n)
	}
}

func TestAResultFromBeforeResponsesStillReadsAndAResponseNeedsItsFinding(t *testing.T) {
	f := newFixture(t, "exp_vehicle_old")
	exp := f.propose(t, -1)
	insert := `INSERT INTO result (experiment_id, channel, role, corner_class, estimate, ci_low, ci_high,
	                               n_passes, effect_class)
	           VALUES ($1, $2, $3, 'all', -0.01, -0.02, 0.0, 84, 'inconclusive')`
	if _, err := f.database.ExecContext(t.Context(), insert, exp.Id, "corner_time_s", "primary"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.ExecContext(t.Context(), insert, exp.Id, "rear_slip_deg", "response"); err == nil {
		t.Error("a response row without its finding, expectation and margin was stored")
	}

	read, err := f.service.Get(t.Context(), int64(exp.Id))

	if err != nil || len(read.Results) != 1 || len(read.Responses) != 0 {
		t.Errorf("err %v, results %+v, responses %+v", err, read.Results, read.Responses)
	}
}
