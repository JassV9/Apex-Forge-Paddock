package engine

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// engineStub answers one path with a fixed body and keeps what it was sent.
func engineStub(t *testing.T, path, reply string, sent any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != path {
			t.Errorf("engine was called with %s %s, want POST %s", r.Method, r.URL.Path, path)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, sent); err != nil {
			t.Errorf("request body %s: %v", body, err)
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSensitivityPostsTheMeasurementsAndDecodesTheProfile(t *testing.T) {
	var got contracts.SensitivityRequest
	server := engineStub(t, "/sensitivity/update", `{"model_version":"m-1","effects":[{
		"parameter":"ARB_FRONT","clicks":-1,"channel":"corner_time_s","corner_class":"all",
		"estimate":-0.03,"ci_low":-0.04,"ci_high":-0.02,"n_passes":84,"effect_class":"negative",
		"nonlinear":false,"note":"","is_prior":false,"prior_source":"","experiment_ids":[7]}]}`, &got)
	request := contracts.SensitivityRequest{
		Measurements: []contracts.Measurement{{
			ExperimentId: 7, Parameter: "ARB_FRONT", Clicks: -1, Channel: "corner_time_s",
			CornerClass: contracts.MeasurementCornerClassAll, Estimate: -0.03, CiLow: -0.04, CiHigh: -0.02, NPasses: 84,
		}},
		Transfers: []contracts.Transfer{},
		Ropes:     contracts.SensitivityRequestRopes{"corner_time_s": 0.005},
		Level:     0.9,
	}

	reply, err := New(server.URL).Sensitivity(t.Context(), request)

	if err != nil {
		t.Fatalf("Sensitivity: %v", err)
	}
	if len(got.Measurements) != 1 || got.Measurements[0] != request.Measurements[0] || got.Ropes["corner_time_s"] != 0.005 {
		t.Errorf("engine received %+v", got)
	}
	if reply.ModelVersion != "m-1" || len(reply.Effects) != 1 || reply.Effects[0].ExperimentIds[0] != 7 {
		t.Errorf("reply = %+v", reply)
	}
}

func TestProposePostsTheCandidatesAndDecodesTheRanking(t *testing.T) {
	var got contracts.ProposeRequest
	server := engineStub(t, "/propose", `{"stage":"learning","skipped":[],"ranked":[{
		"parameter":"ARB_FRONT","label":"ARB Front","clicks":-1,"why":"softer front",
		"hypotheses":["mid_understeer.slow"],"basis":"measured","prior_source":"",
		"expected_gain":0.02,"gain_low":0.01,"gain_high":0.04,"laps_needed":16,"value_per_lap":0.00125}]}`, &got)
	request := contracts.ProposeRequest{
		Candidates: []contracts.ProposeCandidate{{
			Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front",
			Hypotheses: []string{"mid_understeer.slow"},
		}},
		Effects: []contracts.SensitivityEffect{}, Channel: "corner_time_s",
		Better: contracts.ProposeRequestBetterLower, Rope: 0.005, Sigma: 0.028, Corners: 7,
		LapsPerBlock: 3, Experiments: 6,
	}

	reply, err := New(server.URL).Propose(t.Context(), request)

	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if len(got.Candidates) != 1 || got.Corners != 7 || got.Experiments != 6 {
		t.Errorf("engine received %+v", got)
	}
	if reply.Stage != contracts.ProposalsStageLearning || reply.Ranked[0].Basis != contracts.ProposalBasisMeasured {
		t.Errorf("reply = %+v", reply)
	}
}
