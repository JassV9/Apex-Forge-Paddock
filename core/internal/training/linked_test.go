package training

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

func TestPickTreatsALinkedGroupAsOneSetting(t *testing.T) {
	links := setups.Links{"WING_1": {"WING_1", "WING_9"}, "WING_9": {"WING_1", "WING_9"}}
	steps := func(parameters ...string) []contracts.TrainingStep {
		answer := []contracts.TrainingStep{}
		for _, parameter := range parameters {
			answer = append(answer, contracts.TrainingStep{Parameter: parameter})
		}
		return answer
	}
	tests := []struct {
		name   string
		ranked []contracts.Proposal
		steps  []contracts.TrainingStep
		links  setups.Links
		want   string
	}{
		{name: "a lone member is proposed as the group's leader",
			ranked: []contracts.Proposal{proposal("WING_9", -1)}, links: links, want: "WING_1"},
		{name: "testing one member tested the group",
			ranked: []contracts.Proposal{proposal("WING_1", -1), proposal("WING_0", 1)},
			steps:  steps("WING_9"), links: links, want: "WING_0"},
		{name: "a tested group is never proposed again",
			ranked: []contracts.Proposal{proposal("WING_9", 1)}, steps: steps("WING_1"), links: links},
		{name: "without links every item is its own",
			ranked: []contracts.Proposal{proposal("WING_1", -1), proposal("WING_9", -1)},
			steps:  steps("WING_9"), want: "WING_1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := pick(tt.ranked, tt.steps, tt.links)
			if found != (tt.want != "") || got.Parameter != tt.want {
				t.Errorf("picked %q (found %v), want %q", got.Parameter, found, tt.want)
			}
		})
	}
}

func TestTrainingNeverAsksForALoneMember(t *testing.T) {
	f := newFixture(t, "training_linked")
	f.exec(`INSERT INTO setup_parameter (car_id, name, label, min, max, step)
	        VALUES ($1, 'WING_1', 'Rear Wing', 0, 2, 1), ($1, 'WING_9', 'Rear Wing', 0, 2, 1)`, f.car)
	f.exec(`INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by)
	        SELECT $1, 'w' || n, 'a' || n, jsonb_build_object('WING_1', n % 2, 'WING_9', n % 2), 'agent'
	        FROM generate_series(1, 3) AS n`, f.car)
	f.ranker.ranked = []contracts.Proposal{proposal("WING_9", -1)}

	f.start(2)

	if len(f.ledger.created) != 1 || f.ledger.created[0].Parameter != "WING_1" {
		t.Errorf("created %+v, want the rear wing group under WING_1", f.ledger.created)
	}
}
