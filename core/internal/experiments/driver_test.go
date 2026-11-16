package experiments

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The driver chosen at proposal is stored, returned, and written into the run plan, so the
// agent knows whether to hand the car to AC's AI (docs/adr/0014-ghost-driver.md).
func TestTheDriverOfAnExperimentReachesTheRunPlan(t *testing.T) {
	cases := []struct {
		name    string
		request contracts.ExperimentCreateDriver
		want    string
	}{
		{"unset means the human", "", "human"},
		{"the human", contracts.ExperimentCreateDriverHuman, "human"},
		{"the AI", contracts.ExperimentCreateDriverAi, "ai"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "exp_driver_"+string(rune('a'+i)))
			exp, err := f.service.Create(t.Context(), contracts.ExperimentCreate{
				SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: -1,
				Statement: "A softer front bar gives more front grip.", Driver: tc.request,
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if string(exp.Driver) != tc.want {
				t.Errorf("experiment driver = %q, want %q", exp.Driver, tc.want)
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
			if string(plan.Driver) != tc.want {
				t.Errorf("run plan driver = %q, want %q", plan.Driver, tc.want)
			}
		})
	}
}
