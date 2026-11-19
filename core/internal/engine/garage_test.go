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
	server := engineStub(t, "/sensitivity/update", `{"model_version":"m-1","priors":[],"excluded":[],
		"effects":[{"parameter":"ARB_FRONT","clicks":-1,"channel":"corner_time_s","corner_class":"all",
		"phase":"corner","track":"fn_cota/layout_f1_2026","track_name":"COTA","driver":"ai",
		"hint_bucket":"0.89","estimate":-0.03,"ci_low":-0.04,"ci_high":-0.02,"standard_error":0.005,
		"p_increase":0.99,"method":"single","experiments":1,"knowledge_level":"preliminary",
		"effect_class":"negative","n_passes":84,"nonlinear":false,"note":"","stale":false,
		"experiment_ids":[7],"engine_versions":["0.2.0"]}]}`, &got)
	se, df := 0.005, 5.0
	request := contracts.SensitivityRequest{
		Measurements: []contracts.Measurement{{
			ExperimentId: 7, Parameter: "ARB_FRONT", Clicks: -1, Channel: "corner_time_s",
			CornerClass: contracts.MeasurementCornerClassAll, Phase: "corner", Track: "fn_cota/layout_f1_2026",
			Driver: contracts.MeasurementDriverAi, HintBucket: "0.89", Estimate: -0.03,
			StandardError: &se, Df: &df, NPasses: 84,
		}},
		Transfers: []contracts.Transfer{},
		Ropes:     contracts.SensitivityRequestRopes{"corner_time_s": 0.005},
		Level:     0.9,
	}

	reply, err := New(server.URL).Sensitivity(t.Context(), request)

	if err != nil {
		t.Fatalf("Sensitivity: %v", err)
	}
	sent := got.Measurements[0]
	if len(got.Measurements) != 1 || *sent.StandardError != se || *sent.Df != df || sent.CiLow != nil ||
		got.Ropes["corner_time_s"] != 0.005 {
		t.Errorf("engine received %+v", got)
	}
	if reply.ModelVersion != "m-1" || len(reply.Effects) != 1 || reply.Effects[0].ExperimentIds[0] != 7 ||
		reply.Effects[0].KnowledgeLevel != contracts.SensitivityEffectKnowledgeLevelPreliminary {
		t.Errorf("reply = %+v", reply)
	}
}

func TestMeasurePostsTheReplayAndDecodesTheDerivation(t *testing.T) {
	var got contracts.EvaluateRequest
	server := engineStub(t, "/sensitivity/measure", `{"engine_version":"0.2.0",
		"finding_class":"insufficient_evidence","reason":"","effects":[{"channel":"corner_time_s",
		"corner_class":"all","estimate":-0.0037,"standard_error":0.0025,"df":6,"n_passes":420,
		"blocks":10,"corners":14}]}`, &got)
	request := contracts.EvaluateRequest{
		SuccessTest: contracts.SuccessTest{Channel: "corner_time_s", Rope: 0.005, Level: 0.9, Better: "lower"},
		Guardrails:  []contracts.GuardrailSpec{}, Predictions: []contracts.PredictionSpec{},
		DriverChannels: []string{}, Passes: []contracts.EvaluatePass{},
	}

	reply, err := New(server.URL).Measure(t.Context(), request)

	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	if got.SuccessTest.Channel != "corner_time_s" || reply.EngineVersion != "0.2.0" ||
		len(reply.Effects) != 1 || reply.Effects[0].Df != 6 {
		t.Errorf("engine received %+v, replied %+v", got, reply)
	}
}

func TestVersionAsksTheEnginesHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			t.Errorf("engine was called with %s %s, want GET /health", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"service":"engine","version":"0.2.0","status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	version, err := New(server.URL).Version(t.Context())

	if err != nil || version != "0.2.0" {
		t.Errorf("Version = %q, %v", version, err)
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
		Effects: []contracts.SensitivityEffect{}, Priors: []contracts.SensitivityPrior{}, Channel: "corner_time_s",
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

func TestBaseSetupPostsTheProfileAndDecodesTheChoice(t *testing.T) {
	var got contracts.BaseSetupRequest
	server := engineStub(t, "/base-setup", `{"channel":"corner_time_s","left_out":[],"measured":[],
		"changes":[{"parameter":"ARB_FRONT","label":"ARB Front","clicks":-1,"from_value":3,"to_value":2,
		"estimate":-0.0042,"ci_low":-0.0068,"ci_high":-0.0016,"n_passes":84,
		"effect_class":"inconclusive","experiment_ids":[12],"shrunk_estimate":-0.0035,
		"knowledge_level":"preliminary","p_value":0.01,"track":"fn_cota/layout_f1_2026","hint_bucket":"0.89"}],
		"prediction":{"channel":"corner_time_s","estimate":-0.0035,"raw_estimate":-0.0042,
		"ci_low":-0.0061,"ci_high":-0.0009,"level":0.9,"note":"Sum of 1 effects."}}`, &got)
	request := contracts.BaseSetupRequest{
		Effects: []contracts.SensitivityEffect{}, Priors: []contracts.SensitivityPrior{},
		Items: []contracts.BaseSetupItem{{
			Name: "ARB_FRONT", Label: "ARB Front", Min: 0, Max: 4, Step: 1, Value: 3,
		}},
		Channel: "corner_time_s", Better: contracts.BaseSetupRequestBetterLower, Rope: 0.005,
		Level: 0.9, Guardrails: []contracts.GuardrailSpec{},
	}

	reply, err := New(server.URL).BaseSetup(t.Context(), request)

	if err != nil {
		t.Fatalf("BaseSetup: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Value != 3 || got.Level != 0.9 {
		t.Errorf("engine received %+v", got)
	}
	if len(reply.Changes) != 1 || reply.Changes[0].ToValue != 2 || reply.Prediction == nil {
		t.Errorf("reply = %+v", reply)
	}
}
