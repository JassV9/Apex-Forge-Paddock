package experiments

import (
	"errors"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// A stored diagnosis with one hypothesis, as the engine writes it.
const storedDiagnosis = `{
  "evidence": [{"id": "front_slip_mid_deg.slow", "metric": "front_slip_mid_deg",
                "corner_class": "slow", "value": 7.7, "passes": 32}],
  "hypotheses": [{
    "key": "mid_understeer.slow", "statement": "Understeer mid-corner in slow corners.",
    "phase": "mid", "corner_class": "slow", "score": 3.4,
    "evidence": ["front_slip_mid_deg.slow"],
    "alternatives": [{"kind": "driver_input", "statement": "Steering past the front tyre's peak.",
                      "supported": true, "evidence": ["front_slip_mid_deg.slow"]}],
    "candidates": [], "not_testable": []}],
  "explanation": {"provider": "none", "summary": "Understeer.", "ranking": [], "rejected": ""}
}`

func (f *fixture) proposeFrom(t *testing.T, key *string) (contracts.Experiment, error) {
	t.Helper()
	return f.service.Create(t.Context(), contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: -1,
		Statement: "A softer front bar cures slow-corner understeer.", HypothesisKey: key,
	})
}

func TestAHypothesisFromTheDiagnosisGoesIntoTheLedgerWithItsEvidence(t *testing.T) {
	f := newFixture(t, "exp_origin")
	if _, err := f.database.ExecContext(t.Context(),
		`INSERT INTO diagnosis (session_id, body) VALUES ($1, $2)`, f.sessionID, storedDiagnosis); err != nil {
		t.Fatalf("store diagnosis: %v", err)
	}
	key := "mid_understeer.slow"

	exp, err := f.proposeFrom(t, &key)

	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if exp.Source != contracts.ExperimentSourceRules {
		t.Errorf("source = %s, want rules", exp.Source)
	}
	var phase, cornerClass, evidence, alternative string
	var supported bool
	err = f.database.QueryRowContext(t.Context(),
		`SELECT h.phase, h.corner_class, h.evidence->>0, h.alternatives->0->>'kind',
		        (h.alternatives->0->>'supported')::boolean
		 FROM hypothesis h JOIN experiment e ON e.hypothesis_id = h.id WHERE e.id = $1`, exp.Id).
		Scan(&phase, &cornerClass, &evidence, &alternative, &supported)
	if err != nil {
		t.Fatalf("read hypothesis: %v", err)
	}
	if phase != "mid" || cornerClass != "slow" || evidence != "front_slip_mid_deg.slow" {
		t.Errorf("hypothesis is %s/%s on %s", phase, cornerClass, evidence)
	}
	if alternative != "driver_input" || !supported {
		t.Errorf("alternative %s supported %v, want the driver-input alternative kept", alternative, supported)
	}
}

func TestAHypothesisWithoutAKeyIsTheHumansOwn(t *testing.T) {
	f := newFixture(t, "exp_origin_human")

	exp, err := f.proposeFrom(t, nil)

	if err != nil || exp.Source != contracts.ExperimentSourceHuman {
		t.Errorf("source = %s, %v; want human", exp.Source, err)
	}
}

func TestAKeyThatNoDiagnosisHasIsRefused(t *testing.T) {
	f := newFixture(t, "exp_origin_unknown")
	key := "mid_understeer.slow"

	_, noDiagnosis := f.proposeFrom(t, &key)
	if _, err := f.database.ExecContext(t.Context(),
		`INSERT INTO diagnosis (session_id, body) VALUES ($1, $2)`, f.sessionID, storedDiagnosis); err != nil {
		t.Fatalf("store diagnosis: %v", err)
	}
	other := "exit_oversteer.all"
	_, noSuchKey := f.proposeFrom(t, &other)

	if !errors.Is(noDiagnosis, ErrInvalid) || !errors.Is(noSuchKey, ErrInvalid) {
		t.Errorf("errors = %v and %v, want ErrInvalid twice", noDiagnosis, noSuchKey)
	}
	if stored := f.count(t, `SELECT count(*) FROM experiment`); stored != 0 {
		t.Errorf("%d refused experiments were stored", stored)
	}
}
