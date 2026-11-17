package experiments

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// linkARBFront gives the sample car a second "ARB Front" item that the car keeps equal to
// ARB_FRONT, as the Formula Hybrid 2026 keeps WING_9 equal to WING_1: every recorded setup has
// both at one value, and two more setups read back from the car move them together.
func (f *fixture) linkARBFront(t *testing.T) {
	t.Helper()
	statements := []string{
		`INSERT INTO setup_parameter (car_id, name, label, min, max, step)
		 SELECT car_id, 'ARB_FRONT_LINK', label, min, max, step FROM setup_parameter WHERE name = 'ARB_FRONT'`,
		`UPDATE setup_version SET "values" = "values" || jsonb_build_object('ARB_FRONT_LINK', "values"->'ARB_FRONT')`,
		`INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by)
		 SELECT car_id, 'linked-' || n, 'agent-' || n,
		        "values" || jsonb_build_object('ARB_FRONT', n, 'ARB_FRONT_LINK', n), 'agent'
		 FROM setup_version CROSS JOIN generate_series(1, 2) AS n WHERE agent_hash IS NOT NULL ORDER BY n LIMIT 2`,
	}
	for _, statement := range statements {
		if _, err := f.database.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func TestAChangeToALinkedItemMovesTheWholeGroup(t *testing.T) {
	f := newFixture(t, "experiments_linked")
	f.linkARBFront(t)
	exp, err := f.service.Create(t.Context(), contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT_LINK", Clicks: -1,
		Statement: "A softer front bar gives more front grip in slow corners.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if exp.Parameter != "ARB_FRONT" || exp.ParameterLabel != "ARB Front (ARB_FRONT + ARB_FRONT_LINK)" {
		t.Errorf("filed under %s %q, want the group once under its leader", exp.Parameter, exp.ParameterLabel)
	}
	moved := f.count(t, `
		SELECT count(*) FROM experiment_arm b
		JOIN setup_version bv ON bv.id = b.setup_version_id
		JOIN experiment_arm a ON a.experiment_id = b.experiment_id AND NOT a.is_baseline
		JOIN setup_version av ON av.id = a.setup_version_id
		CROSS JOIN LATERAL jsonb_object_keys(av."values") AS item
		WHERE b.experiment_id = $1 AND b.is_baseline
		  AND (av."values"->>item)::float8 = (bv."values"->>item)::float8 - 1`, exp.Id)
	if moved != 2 {
		t.Errorf("the variant moved %d items one click down, want both members", moved)
	}
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(f.outbox, "setups", "ks_mazda_mx5_cup", "*.ini"))
	for _, file := range files {
		content, err := os.ReadFile(file) //nolint:gosec // a file the service just wrote
		if err != nil || !strings.Contains(string(content), "[ARB_FRONT_LINK]") {
			t.Errorf("%s has no ARB_FRONT_LINK: %v", file, err)
		}
	}
}

func TestALinkedGroupIsOneSetupItem(t *testing.T) {
	f := newFixture(t, "experiments_linked_items")
	f.linkARBFront(t)
	items, err := f.service.SetupItems(t.Context(), int64(f.sessionID))
	if err != nil {
		t.Fatalf("SetupItems: %v", err)
	}
	var fronts []string
	for _, item := range items {
		if strings.HasPrefix(item.Name, "ARB_FRONT") {
			fronts = append(fronts, item.Name+" "+item.Label)
		}
	}
	if len(fronts) != 1 || fronts[0] != "ARB_FRONT ARB Front (ARB_FRONT + ARB_FRONT_LINK)" {
		t.Errorf("front bar items %v, want the group once", fronts)
	}
}

func TestItemsThatNeverMovedStayApart(t *testing.T) {
	f := newFixture(t, "experiments_unlinked")
	_, err := f.database.ExecContext(t.Context(),
		`INSERT INTO setup_parameter (car_id, name, label, min, max, step)
		 SELECT car_id, 'ARB_FRONT_LINK', label, min, max, step FROM setup_parameter WHERE name = 'ARB_FRONT'`)
	if err != nil {
		t.Fatal(err)
	}
	exp := f.propose(t, -1)
	if exp.ParameterLabel != "ARB Front" {
		t.Errorf("label %q, want the item's own", exp.ParameterLabel)
	}
}

func TestABaseSetupMovesAGroupOnce(t *testing.T) {
	links := setups.Links{"WING_1": {"WING_1", "WING_9"}, "WING_9": {"WING_1", "WING_9"}}
	tests := []struct {
		name    string
		changes map[string]int
		want    map[string]int
		wantErr bool
	}{
		{name: "one member", changes: map[string]int{"WING_9": -1, "ARB": 1},
			want: map[string]int{"WING_1": -1, "ARB": 1}},
		{name: "both members alike", changes: map[string]int{"WING_1": -1, "WING_9": -1},
			want: map[string]int{"WING_1": -1}},
		{name: "members asked to move differently", changes: map[string]int{"WING_1": -1, "WING_9": 1},
			wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oneChangePerSetting(links, tt.changes)
			if (err != nil) != tt.wantErr || (!tt.wantErr && !maps.Equal(got, tt.want)) {
				t.Errorf("settings %v, error %v; want %v", got, err, tt.want)
			}
		})
	}
}
