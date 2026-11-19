package discovery

import (
	"fmt"
	"math"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// gridTolerance absorbs floating-point error when counting clicks to a limit.
const gridTolerance = 1e-6

// planSteps turns the steps asked for into the steps a sweep can drive, in the order asked:
// each is clipped to the item's real range, and a step that clipping makes 0 (the baseline is at
// that limit) or a repeat of an earlier one is skipped. Every change is said in words, so the
// driver sees why +3 became +2 or disappeared.
func planSteps(item contracts.SetupItem, requested []int) (planned []int, notes []string) {
	planned, notes = []int{}, []string{}
	for _, asked := range requested {
		clicks, note := clip(item, asked)
		switch {
		case clicks == 0:
			notes = append(notes, note)
		case slices.Contains(planned, clicks) && note != "":
			notes = append(notes, fmt.Sprintf("%s, already a step; skipped", note))
		case slices.Contains(planned, clicks):
			notes = append(notes, fmt.Sprintf("%+d is already a step; skipped", asked))
		default:
			planned = append(planned, clicks)
			if note != "" {
				notes = append(notes, note)
			}
		}
	}
	return planned, notes
}

// clip returns the step the item's range allows for the clicks asked, 0 when none, and what
// clipping did in words ("" when nothing).
func clip(item contracts.SetupItem, asked int) (int, string) {
	if asked == 0 {
		return 0, "0 clicks is the baseline itself; skipped"
	}
	target := item.Value + float64(asked)*item.Step
	limit, side := item.Max, "maximum"
	allowed := int(math.Floor((item.Max-item.Value)/item.Step + gridTolerance))
	if asked < 0 {
		limit, side = item.Min, "minimum"
		allowed = int(math.Ceil((item.Min-item.Value)/item.Step - gridTolerance))
	}
	if (asked > 0 && asked <= allowed) || (asked < 0 && asked >= allowed) {
		return asked, ""
	}
	if allowed == 0 || (allowed > 0) != (asked > 0) {
		return 0, fmt.Sprintf("%+d: the baseline (%v) is at the %s %v; skipped", asked, item.Value, side, limit)
	}
	return allowed, fmt.Sprintf("%+d would be %v, beyond the %s %v: clipped to %+d", asked, target, side, limit, allowed)
}
