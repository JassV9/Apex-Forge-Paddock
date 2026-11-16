package sensitivity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// fakeLedger serves the baseline's items and keeps the base setups it was asked to propose.
type fakeLedger struct {
	created []experiments.BaseRequest
}

func (l *fakeLedger) BaselineVersion(context.Context, int64) (int64, error) { return 41, nil }

func (l *fakeLedger) SetupItems(context.Context, int64) ([]contracts.SetupItem, error) {
	return []contracts.SetupItem{
		{Name: "ARB_FRONT", Label: "ARB Front", Min: 0, Max: 4, Step: 1, Value: 3},
		{Name: "CAMBER_LF", Label: "Camber LF", Min: -38, Max: 8, Step: 1, Value: -30},
	}, nil
}

func (l *fakeLedger) CreateBase(_ context.Context, request experiments.BaseRequest) (contracts.Experiment, error) {
	l.created = append(l.created, request)
	return contracts.Experiment{Id: 77}, nil
}

// fakeChooser answers with a fixed base setup and keeps the question.
type fakeChooser struct {
	reply   contracts.BaseSetup
	failure error
	asked   []contracts.BaseSetupRequest
}

func (c *fakeChooser) BaseSetup(_ context.Context, request contracts.BaseSetupRequest) (contracts.BaseSetup, error) {
	c.asked = append(c.asked, request)
	return c.reply, c.failure
}

var softerFront = contracts.BaseSetupChange{
	Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, FromValue: 3, ToValue: 2,
	Estimate: -0.0042, CiLow: -0.0068, CiHigh: -0.0016, NPasses: 84,
	EffectClass: contracts.BaseSetupChangeEffectClassInconclusive, ExperimentIds: []int{12},
}

func baseSetups(f *fixture, chooser *fakeChooser) (*BaseSetups, *fakeLedger) {
	ledger := &fakeLedger{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewBaseSetups(f.service, ledger, chooser, logger), ledger
}

func TestABaseSetupIsProposedFromTheCarsOwnMeasurements(t *testing.T) {
	f := newFixture(t, "base_setup_built")
	f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.0042)
	chooser := &fakeChooser{reply: contracts.BaseSetup{
		Channel: "corner_time_s", Changes: []contracts.BaseSetupChange{softerFront},
		LeftOut: []contracts.BaseSetupLeftOut{}, Measured: []contracts.BaseSetupMeasured{},
		Prediction: &contracts.BaseSetupPrediction{
			Channel: "corner_time_s", Estimate: -0.0042, CiLow: -0.0068, CiHigh: -0.0016, Level: 0.9,
		},
	}}
	builder, ledger := baseSetups(f, chooser)

	reply, err := builder.Build(t.Context(), f.sessionID, contracts.BaseSetupCreateDriverAi)

	if err != nil || reply.ExperimentId == nil || *reply.ExperimentId != 77 || reply.Error != nil {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	asked := chooser.asked[0]
	if len(asked.Effects) != 1 || asked.Effects[0].Parameter != "ARB_FRONT" || asked.Channel != "corner_time_s" ||
		asked.Level != 0.9 || len(asked.Guardrails) != 2 {
		t.Errorf("engine was asked %+v", asked)
	}
	paired := map[string]bool{}
	for _, item := range asked.Items {
		paired[item.Name] = item.Paired
	}
	if paired["ARB_FRONT"] || !paired["CAMBER_LF"] {
		t.Errorf("paired = %v: a camber per wheel is a pair, the front bar is not", paired)
	}
	created := ledger.created[0]
	if created.BaselineVersion != 41 || created.Changes["ARB_FRONT"] != -1 || len(created.Changes) != 1 ||
		created.Driver != contracts.ExperimentCreateDriverAi || !strings.HasPrefix(created.Statement, "Base setup: 1 measured change on ") ||
		!strings.Contains(created.Evidence[0], "experiments 12") {
		t.Errorf("proposed %+v", created)
	}
	want := contracts.PredictionSpec{Channel: "corner_time_s", Direction: contracts.PredictionSpecDirectionDecrease, Low: 0.0016, High: 0.0068}
	if created.Prediction != want {
		t.Errorf("prediction %+v, want %+v", created.Prediction, want)
	}
}

func TestWithoutAQualifyingChangeTheReplySaysWhatWasMeasured(t *testing.T) {
	tests := []struct {
		name     string
		measured []contracts.BaseSetupMeasured
		want     string
	}{
		{"nothing measured", []contracts.BaseSetupMeasured{}, "measured nothing yet"},
		{"measured, nothing faster", []contracts.BaseSetupMeasured{{
			Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: 1,
			EffectClass: contracts.BaseSetupMeasuredEffectClassInconclusive,
		}}, "ARB Front +1 inconclusive"},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t, "base_setup_none_"+string(rune('a'+i)))
			chooser := &fakeChooser{reply: contracts.BaseSetup{
				Channel: "corner_time_s", Changes: []contracts.BaseSetupChange{},
				LeftOut: []contracts.BaseSetupLeftOut{}, Measured: test.measured,
			}}
			builder, ledger := baseSetups(f, chooser)

			reply, err := builder.Build(t.Context(), f.sessionID, contracts.BaseSetupCreateDriverHuman)

			if err != nil || reply.ExperimentId != nil || reply.Error == nil || len(ledger.created) != 0 {
				t.Fatalf("reply %+v, err %v, created %d", reply, err, len(ledger.created))
			}
			if !strings.Contains(*reply.Error, test.want) || !strings.Contains(*reply.Error, "Train the AI on this car") {
				t.Errorf("error %q, want %q and the advice", *reply.Error, test.want)
			}
		})
	}
}

func TestABaseSetupForAnUnknownSessionOrWithoutTheEngineFails(t *testing.T) {
	f := newFixture(t, "base_setup_fails")
	builder, _ := baseSetups(f, &fakeChooser{failure: engine.ErrUnavailable})

	if _, err := builder.Build(t.Context(), 999, contracts.BaseSetupCreateDriverAi); !errors.Is(err, experiments.ErrNotFound) {
		t.Errorf("unknown session: err = %v, want ErrNotFound", err)
	}
	if _, err := builder.Build(t.Context(), f.sessionID, contracts.BaseSetupCreateDriverAi); !errors.Is(err, engine.ErrUnavailable) {
		t.Errorf("no engine: err = %v, want ErrUnavailable", err)
	}
}
