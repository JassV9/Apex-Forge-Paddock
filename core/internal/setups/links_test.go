package setups

import (
	"errors"
	"reflect"
	"testing"
)

// The Formula Hybrid 2026's two "Rear Wing" items, two dampers with one label that the driver
// never moved, and an item with a label of its own.
func hybrid() Catalog {
	return Catalog{
		"WING_1":       {Name: "WING_1", Label: "Rear Wing", Min: 0, Max: 2, Step: 1},
		"WING_9":       {Name: "WING_9", Label: "Rear Wing", Min: 0, Max: 2, Step: 1},
		"WING_0":       {Name: "WING_0", Label: "Front Wing", Min: 0, Max: 2, Step: 1},
		"DAMP_BUMP_LF": {Name: "DAMP_BUMP_LF", Label: "Bump", Min: 0, Max: 20, Step: 1},
		"DAMP_BUMP_RF": {Name: "DAMP_BUMP_RF", Label: "Bump", Min: 0, Max: 20, Step: 1},
		"ERS_MODE":     {Name: "ERS_MODE", Label: "Rear Wing", Min: 0, Max: 2, Step: 1, Excluded: true},
	}
}

// recorded is setups read back from the car: the wings move together, the front wing with
// them, the dampers never.
func recorded(wings ...float64) []Values {
	versions := make([]Values, len(wings))
	for i, wing := range wings {
		versions[i] = Values{
			"WING_1": wing, "WING_9": wing, "WING_0": wing, "ERS_MODE": wing,
			"DAMP_BUMP_LF": 7, "DAMP_BUMP_RF": 7,
		}
	}
	return versions
}

func TestDetectLinks(t *testing.T) {
	rearWing := []string{"WING_1", "WING_9"}
	split := recorded(1, 0, 1)
	split[1]["WING_9"] = 1
	missing := recorded(1, 0, 1)
	delete(missing[2], "WING_9")
	otherRange := hybrid()
	otherRange["WING_9"] = Parameter{Name: "WING_9", Label: "Rear Wing", Min: 0, Max: 3, Step: 1}
	otherLabel := hybrid()
	otherLabel["WING_9"] = Parameter{Name: "WING_9", Label: "Rear Flap", Min: 0, Max: 2, Step: 1}
	tests := []struct {
		name     string
		catalog  Catalog
		recorded []Values
		declared [][]string
		want     Links
	}{
		{name: "equal and moved together: linked", catalog: hybrid(), recorded: recorded(1, 0, 1, 1, 0),
			want: Links{"WING_1": rearWing, "WING_9": rearWing}},
		{name: "never changed: not linked", catalog: hybrid(), recorded: recorded(1, 1, 1), want: Links{}},
		{name: "too few versions: not linked", catalog: hybrid(), recorded: recorded(1, 0), want: Links{}},
		{name: "different in one version: not linked", catalog: hybrid(), recorded: split, want: Links{}},
		{name: "missing from one version: not linked", catalog: hybrid(), recorded: missing, want: Links{}},
		{name: "different labels: not linked", catalog: otherLabel, recorded: recorded(1, 0, 1), want: Links{}},
		{name: "different ranges: not linked", catalog: otherRange, recorded: recorded(1, 0, 1), want: Links{}},
		{name: "declared: linked without recording", catalog: hybrid(), declared: [][]string{rearWing},
			want: Links{"WING_1": rearWing, "WING_9": rearWing}},
		{name: "declared item not on the car: ignored", catalog: hybrid(),
			declared: [][]string{{"WING_1", "WING_42"}}, want: Links{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLinks(tt.catalog, tt.recorded, tt.declared)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("links = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinksNameAGroupOnce(t *testing.T) {
	links := DetectLinks(hybrid(), nil, DeclaredLinks["rss_formula_hybrid_2026"])
	tests := []struct {
		item, leader, label string
	}{
		{item: "WING_1", leader: "WING_1", label: "Rear Wing (WING_1 + WING_9)"},
		{item: "WING_9", leader: "WING_1", label: "Rear Wing (WING_1 + WING_9)"},
		{item: "WING_0", leader: "WING_0", label: "Front Wing"},
	}
	for _, tt := range tests {
		if got := links.Leader(tt.item); got != tt.leader {
			t.Errorf("Leader(%s) = %s, want %s", tt.item, got, tt.leader)
		}
		if got := links.Label(hybrid(), tt.item); got != tt.label {
			t.Errorf("Label(%s) = %q, want %q", tt.item, got, tt.label)
		}
	}
}

func TestStepMovesTheWholeGroup(t *testing.T) {
	links := Links{"WING_1": {"WING_1", "WING_9"}, "WING_9": {"WING_1", "WING_9"}}
	base := Values{"WING_1": 1, "WING_9": 1, "WING_0": 1}
	tests := []struct {
		name    string
		item    string
		clicks  int
		want    []Change
		wantErr error
	}{
		{name: "first member", item: "WING_1", clicks: -1,
			want: []Change{{Name: "WING_1", From: 1, To: 0}, {Name: "WING_9", From: 1, To: 0}}},
		{name: "second member", item: "WING_9", clicks: 1,
			want: []Change{{Name: "WING_1", From: 1, To: 2}, {Name: "WING_9", From: 1, To: 2}}},
		{name: "an item that is not linked moves alone", item: "WING_0", clicks: 1,
			want: []Change{{Name: "WING_0", From: 1, To: 2}}},
		{name: "past the range of the group", item: "WING_9", clicks: 2, wantErr: ErrOutOfRange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variant, err := Step(hybrid(), links, base, tt.item, tt.clicks)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got := Diff(base, variant); tt.wantErr == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("changes = %+v, want %+v", got, tt.want)
			}
		})
	}
}
