package sensitivity

import (
	"reflect"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

func hybridMenu() experiments.Menu {
	return experiments.Menu{
		Catalog: setups.Catalog{
			"WING_1": {Name: "WING_1", Label: "Rear Wing"},
			"WING_9": {Name: "WING_9", Label: "Rear Wing"},
		},
		Links: setups.Links{"WING_1": {"WING_1", "WING_9"}, "WING_9": {"WING_1", "WING_9"}},
	}
}

func TestALinkedGroupIsOneCandidate(t *testing.T) {
	diagnosis := contracts.Diagnosis{Hypotheses: []contracts.Hypothesis{
		{Key: "understeer", Candidates: []contracts.Candidate{
			{Parameter: "WING_1", Label: "Rear Wing", Clicks: -1, Why: "less rear wing"},
			{Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front bar"},
		}},
		{Key: "slow_rotation", Candidates: []contracts.Candidate{
			{Parameter: "WING_9", Label: "Rear Wing", Clicks: -1, Why: "less rear wing"},
		}},
	}}
	tests := []struct {
		name string
		menu experiments.Menu
		want []contracts.ProposeCandidate
	}{
		{name: "linked: one candidate under the leader", menu: hybridMenu(), want: []contracts.ProposeCandidate{
			{Parameter: "WING_1", Label: "Rear Wing (WING_1 + WING_9)", Clicks: -1, Why: "less rear wing",
				Hypotheses: []string{"understeer", "slow_rotation"}},
			{Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front bar",
				Hypotheses: []string{"understeer"}},
		}},
		{name: "not linked: as before", menu: experiments.Menu{}, want: []contracts.ProposeCandidate{
			{Parameter: "WING_1", Label: "Rear Wing", Clicks: -1, Why: "less rear wing", Hypotheses: []string{"understeer"}},
			{Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front bar",
				Hypotheses: []string{"understeer"}},
			{Parameter: "WING_9", Label: "Rear Wing", Clicks: -1, Why: "less rear wing", Hypotheses: []string{"slow_rotation"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := candidatesOf(diagnosis, tt.menu); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("candidates %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestAMeasurementOfAMemberIsTheGroups(t *testing.T) {
	cars := []car{{id: 1, links: hybridMenu().Links}, {id: 2}}
	measurements := map[int64][]contracts.Measurement{
		1: {{Parameter: "WING_9"}, {Parameter: "ARB_FRONT"}},
		2: {{Parameter: "WING_9"}},
	}
	fileUnderLeaders(cars, measurements)
	got := []string{measurements[1][0].Parameter, measurements[1][1].Parameter, measurements[2][0].Parameter}
	if want := []string{"WING_1", "ARB_FRONT", "WING_9"}; !reflect.DeepEqual(got, want) {
		t.Errorf("filed under %v, want %v", got, want)
	}
}
