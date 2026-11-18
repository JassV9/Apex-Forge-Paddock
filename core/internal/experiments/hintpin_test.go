package experiments

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func share(v float64) *float64 { return &v }

// The AI's tyre hint pinned at proposal is written into every revision of the run plan; an
// experiment without one leaves the learned share (docs/adr/0028-run-context-and-ai-provenance.md).
func TestAPinnedHintReachesTheRunPlan(t *testing.T) {
	cases := []struct {
		name  string
		share *float64
	}{
		{"pinned", share(0.93)},
		{"learned", nil},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "exp_hint_pin_"+string(rune('a'+i)))
			exp, err := f.service.Create(t.Context(), contracts.ExperimentCreate{
				SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: -1,
				Statement: "A softer front bar gives more front grip.",
				Driver:    contracts.ExperimentCreateDriverAi, AiHintShare: tc.share,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(RunPlanPath(f.outbox, "ks_mazda_mx5_cup"))
			if err != nil {
				t.Fatal(err)
			}
			var plan contracts.RunPlan
			if err := json.Unmarshal(content, &plan); err != nil {
				t.Fatal(err)
			}
			if (plan.AiHintShare == nil) != (tc.share == nil) ||
				(tc.share != nil && *plan.AiHintShare != *tc.share) {
				t.Errorf("run plan ai_hint_share = %v, want %v", plan.AiHintShare, tc.share)
			}
		})
	}
}

func TestAPinThatCannotBeDrivenIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		share  *float64
		driver contracts.ExperimentCreateDriver
		want   error
	}{
		{"none", nil, "", nil},
		{"for the AI", share(0.5), contracts.ExperimentCreateDriverAi, nil},
		{"for the human", share(0.93), contracts.ExperimentCreateDriverHuman, ErrInvalid},
		{"no driver means the human", share(0.93), "", ErrInvalid},
		{"below the range", share(0.49), contracts.ExperimentCreateDriverAi, ErrInvalid},
		{"above the range", share(1.51), contracts.ExperimentCreateDriverAi, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkHintPin(tc.share, tc.driver); !errors.Is(err, tc.want) {
				t.Errorf("checkHintPin = %v, want %v", err, tc.want)
			}
		})
	}
}
