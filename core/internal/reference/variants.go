package reference

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/launcher"
)

// ApexToleranceM is how far apart, in metres along the lap, the same corner's apex may be on
// two track rows that are grouped as one track (docs/adr/0027-layout-variants.md). A test copy
// is a byte copy, so its corner map matches exactly; this only absorbs rounding.
const ApexToleranceM = 5.0

// TrackRow is one track row as variant grouping reads it.
type TrackRow struct {
	ID     int64
	Layout string
	Name   string
	// CornerMap is track.corner_map as stored: corners with a number and apex_m.
	CornerMap []byte
}

// VariantLayout returns the layout that would be the same physical track as layout on the
// same game track: its Paddock test copy (`<layout>_paddock`, tools/prepare_test_layout.py) or,
// for a test copy, its original. A single-layout track (layout "") has no test copy.
func VariantLayout(layout string) (string, bool) {
	if layout == "" || layout == launcher.TestCopySuffix {
		return "", false
	}
	if original, ok := strings.CutSuffix(layout, launcher.TestCopySuffix); ok {
		return original, true
	}
	return layout + launcher.TestCopySuffix, true
}

// Group returns the rows of a variant layout that are the same track as own, in the order
// given, and why the first row that is not was left out ("" when every row is grouped).
func Group(own TrackRow, variants []TrackRow) ([]TrackRow, string) {
	var grouped []TrackRow
	note := ""
	for _, variant := range variants {
		same, reason := SameCorners(own, variant)
		if same {
			grouped = append(grouped, variant)
		} else if note == "" {
			note = fmt.Sprintf("The AI's laps on %s are not compared with: %s.", variant.Name, reason)
		}
	}
	return grouped, note
}

// SameCorners says whether two track rows have the same corners: the same corner numbers and
// every apex within ApexToleranceM. When they differ, the reason names the first difference.
func SameCorners(own, other TrackRow) (bool, string) {
	mine, err := apexes(own.CornerMap)
	if err != nil {
		return false, fmt.Sprintf("this layout's corner map cannot be matched (%v)", err)
	}
	theirs, err := apexes(other.CornerMap)
	if err != nil {
		return false, fmt.Sprintf("its corner map cannot be matched (%v)", err)
	}
	if len(mine) == 0 {
		return false, "this layout's corner map has no corners"
	}
	if len(mine) != len(theirs) {
		return false, fmt.Sprintf("its corner map has %d corners and this layout's %d", len(theirs), len(mine))
	}
	return sameApexes(mine, theirs)
}

func sameApexes(mine, theirs map[int]float64) (bool, string) {
	numbers := make([]int, 0, len(mine))
	for number := range mine {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		apex, ok := theirs[number]
		if !ok {
			return false, fmt.Sprintf("its corner map has no corner %d", number)
		}
		if gap := math.Abs(apex - mine[number]); gap > ApexToleranceM {
			return false, fmt.Sprintf("corner %d's apex is %.1f m from this layout's, more than %.0f m",
				number, gap, ApexToleranceM)
		}
	}
	return true, ""
}

// apexes reads a stored corner map (track.corner_map). A corner without a number or an apex
// position is an error: such a map cannot be matched with another.
func apexes(cornerMap []byte) (map[int]float64, error) {
	var corners []struct {
		Number *int     `json:"number"`
		ApexM  *float64 `json:"apex_m"`
	}
	if err := json.Unmarshal(cornerMap, &corners); err != nil {
		return nil, fmt.Errorf("read corner map: %w", err)
	}
	found := make(map[int]float64, len(corners))
	for i, corner := range corners {
		if corner.Number == nil || corner.ApexM == nil {
			return nil, fmt.Errorf("corner map entry %d has no number or apex_m", i)
		}
		found[*corner.Number] = *corner.ApexM
	}
	return found, nil
}
