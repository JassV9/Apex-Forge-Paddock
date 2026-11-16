package reference

import (
	"math"
	"slices"
)

// robustScale turns a median absolute deviation into a standard deviation for normal data.
const robustScale = 1.4826

// median of a non-empty list.
func median(values []float64) float64 {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// robustSpread is 1.4826 times the median absolute deviation: the scatter of the typical
// passes, which one lap where the AI went off does not inflate.
func robustSpread(values []float64) float64 {
	centre := median(values)
	deviations := make([]float64, len(values))
	for i, value := range values {
		deviations[i] = math.Abs(value - centre)
	}
	return robustScale * median(deviations)
}

// timesOf lists the zone times of the passes.
func timesOf(passes []Pass) []float64 {
	times := make([]float64, len(passes))
	for i, pass := range passes {
		times[i] = pass.DurationS
	}
	return times
}

// fastest is the pass with the shortest zone time; the earlier one on a tie.
func fastest(passes []Pass) Pass {
	best := passes[0]
	for _, pass := range passes[1:] {
		if pass.DurationS < best.DurationS {
			best = pass
		}
	}
	return best
}
