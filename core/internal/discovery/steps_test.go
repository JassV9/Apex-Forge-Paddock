package discovery

import (
	"slices"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestStepsAreClippedToTheItemsRange(t *testing.T) {
	rearARB := contracts.SetupItem{Name: "ARB_REAR", Min: 0, Max: 5, Step: 1, Value: 3}
	tests := []struct {
		name      string
		item      contracts.SetupItem
		requested []int
		planned   []int
		notes     []string // a fragment of each note, in order
	}{
		{"inside the range", rearARB, []int{-1, 1, 2}, []int{-1, 1, 2}, nil},
		{"past the maximum is clipped, a repeat skipped", rearARB, []int{-1, 1, 2, 3, 4},
			[]int{-1, 1, 2}, []string{"+3 would be 6, beyond the maximum 5: clipped to +2, already a step",
				"+4 would be 7, beyond the maximum 5: clipped to +2, already a step; skipped"}},
		{"past the minimum is clipped", rearARB, []int{-5}, []int{-3},
			[]string{"-5 would be -2, beyond the minimum 0: clipped to -3"}},
		{"at the limit nothing is left", contracts.SetupItem{Min: 0, Max: 5, Step: 1, Value: 5}, []int{1, -1},
			[]int{-1}, []string{"+1: the baseline (5) is at the maximum 5; skipped"}},
		{"0 is the baseline", rearARB, []int{0, 1}, []int{1}, []string{"0 clicks is the baseline"}},
		{"a repeat is driven once", rearARB, []int{1, 1}, []int{1}, []string{"+1 is already a step; skipped"}},
		{"a half-click grid", contracts.SetupItem{Min: 0, Max: 2, Step: 0.5, Value: 1}, []int{3, -1},
			[]int{2, -1}, []string{"+3 would be 2.5, beyond the maximum 2: clipped to +2"}},
		{"a value off the minimum's grid", contracts.SetupItem{Min: 0, Max: 2, Step: 0.5, Value: 1.25}, []int{2},
			[]int{1}, []string{"clipped to +1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planned, notes := planSteps(tt.item, tt.requested)
			if !slices.Equal(planned, tt.planned) {
				t.Errorf("planned %v, want %v", planned, tt.planned)
			}
			if len(notes) != len(tt.notes) {
				t.Fatalf("notes %q, want %d", notes, len(tt.notes))
			}
			for i, fragment := range tt.notes {
				if !strings.Contains(notes[i], fragment) {
					t.Errorf("note %q, want %q", notes[i], fragment)
				}
			}
		})
	}
}
