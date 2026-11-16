package experiments

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// proposeBase proposes a base setup on the sample session: ARB_FRONT 3 -> 2, ARB_REAR 1 -> 0.
func (f *fixture) proposeBase(t *testing.T) contracts.Experiment {
	t.Helper()
	baseline, err := f.service.BaselineVersion(t.Context(), int64(f.sessionID))
	if err != nil {
		t.Fatalf("BaselineVersion: %v", err)
	}
	exp, err := f.service.CreateBase(t.Context(), baseRequest(f, baseline))
	if err != nil {
		t.Fatalf("CreateBase: %v", err)
	}
	return exp
}

func baseRequest(f *fixture, baseline int64) BaseRequest {
	return BaseRequest{
		SessionID: int64(f.sessionID), BaselineVersion: baseline,
		Changes:   map[string]int{"ARB_FRONT": -1, "ARB_REAR": -1},
		Statement: "Base setup: 2 measured changes on Mazda MX-5 Cup",
		Evidence:  []string{"ARB_FRONT -1: experiments 1", "ARB_REAR -1: experiments 2"},
		Prediction: contracts.PredictionSpec{
			Channel: "corner_time_s", Direction: contracts.PredictionSpecDirectionDecrease, Low: 0.002, High: 0.012,
		},
		Driver: contracts.ExperimentCreateDriverAi,
	}
}

func TestABaseSetupIsProposedAsItsOwnExperimentWithItsPrediction(t *testing.T) {
	f := newFixture(t, "base_proposed")

	exp := f.proposeBase(t)

	if exp.Kind != contracts.ExperimentKindBase || exp.State != Proposed || exp.Parameter != "" {
		t.Fatalf("experiment = %s, %s, parameter %q", exp.Kind, exp.State, exp.Parameter)
	}
	if exp.Driver != contracts.ExperimentDriverAi || exp.Source != contracts.ExperimentSourceRules {
		t.Errorf("driver %s, source %s", exp.Driver, exp.Source)
	}
	if len(exp.Predictions) != 1 || exp.Predictions[0].Low != 0.002 || exp.Predictions[0].High != 0.012 {
		t.Errorf("predictions = %+v, want the engine's, written before driving", exp.Predictions)
	}
	var arms int
	var front, rear float64
	err := f.database.QueryRowContext(t.Context(),
		`SELECT count(*), (v."values"->>'ARB_FRONT')::float8, (v."values"->>'ARB_REAR')::float8
		 FROM experiment_arm a JOIN setup_version v ON v.id = a.setup_version_id
		 WHERE a.experiment_id = $1 AND NOT a.is_baseline AND v.created_by = $2
		 GROUP BY v.id`, exp.Id, "base setup, experiment "+strconv.Itoa(exp.Id)).Scan(&arms, &front, &rear)
	if err != nil || arms != 1 || front != 2 || rear != 0 {
		t.Errorf("variant: %d arm, ARB_FRONT %v, ARB_REAR %v, err %v; want both changes on a labelled version", arms, front, rear, err)
	}
}

func TestABaseSetupIsRefusedWhenItCannotBeDriven(t *testing.T) {
	f := newFixture(t, "base_refused")
	baseline, err := f.service.BaselineVersion(t.Context(), int64(f.sessionID))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*BaseRequest)
		want   error
	}{
		{"no change", func(r *BaseRequest) { r.Changes = map[string]int{} }, ErrInvalid},
		{"out of range", func(r *BaseRequest) { r.Changes = map[string]int{"ARB_REAR": 1} }, ErrInvalid},
		{"baseline moved", func(r *BaseRequest) { r.BaselineVersion = baseline + 100 }, ErrConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest(f, baseline)
			test.change(&request)

			_, err := f.service.CreateBase(t.Context(), request)

			if !errors.Is(err, test.want) {
				t.Errorf("err = %v, want %v", err, test.want)
			}
		})
	}
	if n := f.count(t, `SELECT count(*) FROM experiment`); n != 0 {
		t.Errorf("%d experiments stored by refused requests", n)
	}
}

// runBase takes a base setup from proposed to the engine's verdict.
func (f *fixture) runBase(t *testing.T, decision contracts.EvaluateResponseDecision) contracts.Experiment {
	t.Helper()
	exp := f.proposeBase(t)
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return f.judge(t, exp.Id, decision)
}

func TestAnAcceptedBaseSetupIsTheNewChampion(t *testing.T) {
	tests := []struct {
		name      string
		decision  contracts.EvaluateResponseDecision
		wantState State
		wantFront float64 // ARB_FRONT of the champion afterwards
	}{
		{"accepted: the base is the champion", accepted, Accepted, 2},
		{"rejected: the baseline stays", rejected, Reverted, 3},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t, "base_champion_"+strconv.Itoa(i))

			exp := f.runBase(t, test.decision)

			if exp.State != test.wantState {
				t.Fatalf("state %s, want %s", exp.State, test.wantState)
			}
			if champion, original := championValue(t, f); champion != test.wantFront || original != 3 {
				t.Errorf("champion ARB_FRONT %v, original %v; want %v and 3", champion, original, test.wantFront)
			}
			next := f.propose(t, 1) // the next one-item experiment starts from the champion
			for _, arm := range next.Arms {
				if arm.IsBaseline && arm.Value != test.wantFront {
					t.Errorf("next baseline ARB_FRONT %v, want %v", arm.Value, test.wantFront)
				}
			}
		})
	}
}

func TestABaseSetupsFilesSayTheyBelongToOne(t *testing.T) {
	f := newFixture(t, "base_outbox")
	exp := f.proposeBase(t)
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatal(err)
	}

	files, err := filepath.Glob(filepath.Join(f.outbox, "setups", "*", "paddock-e"+strconv.Itoa(exp.Id)+"-base-*.ini"))
	if err != nil || len(files) != 2 {
		t.Fatalf("setup files %v, err %v: want one per arm", files, err)
	}
	for _, file := range files {
		body, err := os.ReadFile(file) //nolint:gosec // a file the test just made
		if err != nil || !strings.Contains(string(body), "(base setup)") {
			t.Errorf("%s: %v, %q", file, err, body)
		}
	}
}
