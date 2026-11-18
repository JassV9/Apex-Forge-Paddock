package reference_test

import (
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/reference"
)

func TestVariantLayoutPairsALayoutWithItsTestCopy(t *testing.T) {
	tests := []struct {
		layout, want string
		ok           bool
	}{
		{layout: "layout_f1_2026", want: "layout_f1_2026_paddock", ok: true},
		{layout: "layout_f1_2026_paddock", want: "layout_f1_2026", ok: true},
		{layout: "", ok: false},         // a single-layout track has no test copy
		{layout: "_paddock", ok: false}, // not a copy of anything
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			got, ok := reference.VariantLayout(tt.layout)
			if got != tt.want || ok != tt.ok {
				t.Errorf("VariantLayout(%q) = %q, %v; want %q, %v", tt.layout, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func row(name, cornerMap string) reference.TrackRow {
	return reference.TrackRow{Name: name, CornerMap: []byte(cornerMap)}
}

const hungaroring = `[{"number": 1, "name": "T1", "apex_m": 432.11}, {"number": 2, "apex_m": 900}]`

func TestSameCornersGroupsOnlyMatchingCornerMaps(t *testing.T) {
	tests := []struct {
		name, other, wantReason string
		own                     string
		want                    bool
	}{
		{name: "a byte copy", own: hungaroring, other: hungaroring, want: true},
		{name: "an apex within the tolerance", own: hungaroring, want: true,
			other: `[{"number": 1, "apex_m": 436}, {"number": 2, "apex_m": 896}]`},
		{name: "an apex further away", own: hungaroring, wantReason: "corner 2's apex is 50.0 m",
			other: `[{"number": 1, "apex_m": 432.11}, {"number": 2, "apex_m": 950}]`},
		{name: "another corner count", own: hungaroring, wantReason: "has 1 corners and this layout's 2",
			other: `[{"number": 1, "apex_m": 432.11}]`},
		{name: "other corner numbers", own: hungaroring, wantReason: "has no corner 2",
			other: `[{"number": 1, "apex_m": 432.11}, {"number": 3, "apex_m": 900}]`},
		{name: "a corner without a position", own: hungaroring, wantReason: "cannot be matched",
			other: `[{"number": 1}, {"number": 2, "apex_m": 900}]`},
		{name: "no corners at all", own: `[]`, other: `[]`, wantReason: "has no corners"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := reference.SameCorners(row("own", tt.own), row("copy", tt.other))
			if got != tt.want || !strings.Contains(reason, tt.wantReason) {
				t.Errorf("SameCorners = %v, %q; want %v, %q", got, reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestGroupKeepsMatchingRowsAndSaysWhyNotTheOthers(t *testing.T) {
	own := row("Hungaroring F1 2026", hungaroring)
	copied := row("Hungaroring F1 2026 (Paddock test)", hungaroring)
	moved := row("Hungaroring F1 2026 (Paddock test) v2", `[{"number": 1, "apex_m": 432.11}]`)

	grouped, note := reference.Group(own, []reference.TrackRow{moved, copied})

	if len(grouped) != 1 || grouped[0].Name != copied.Name {
		t.Errorf("grouped = %+v, want only the matching copy", grouped)
	}
	if !strings.HasPrefix(note, "The AI's laps on Hungaroring F1 2026 (Paddock test) v2 are not compared with") {
		t.Errorf("note = %q", note)
	}
	if _, note := reference.Group(own, []reference.TrackRow{copied}); note != "" {
		t.Errorf("note = %q, want none when every row is grouped", note)
	}
}
