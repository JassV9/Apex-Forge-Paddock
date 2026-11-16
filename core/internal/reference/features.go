package reference

import (
	"math"
	"sort"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// technique is a metric of the engine's features (engine/paddock_engine/features) that says
// how a corner was driven, with the phase it belongs to and the smallest difference worth
// naming. The steps are a judgment, kept here so they can be read and changed in one place.
type technique struct {
	metric string
	phase  contracts.FeatureDifferencePhase
	step   float64
}

var techniques = []technique{
	{metric: "brake_dist_m", phase: contracts.FeatureDifferencePhaseBrake, step: 5},
	{metric: "brake_peak", phase: contracts.FeatureDifferencePhaseBrake, step: 0.05},
	{metric: "v_entry_kmh", phase: contracts.FeatureDifferencePhaseEntry, step: 2},
	{metric: "trail_share", phase: contracts.FeatureDifferencePhaseEntry, step: 0.1},
	{metric: "coast_s", phase: contracts.FeatureDifferencePhaseMid, step: 0.1},
	{metric: "v_min_kmh", phase: contracts.FeatureDifferencePhaseMid, step: 2},
	{metric: "steer_corrections_per_s", phase: contracts.FeatureDifferencePhaseMid, step: 0.5},
	{metric: "throttle_pickup_m", phase: contracts.FeatureDifferencePhaseExit, step: 5},
	{metric: "throttle_to_full_s", phase: contracts.FeatureDifferencePhaseExit, step: 0.2},
}

// maxDifferences is how many differing metrics a corner names.
const maxDifferences = 3

// metricValues lists the value of one metric over the passes that have it.
func metricValues(passes []Pass, metric string) []float64 {
	var values []float64
	for _, pass := range passes {
		if value, ok := pass.Metrics[metric]; ok {
			values = append(values, value)
		}
	}
	return values
}

// differences names the technique metrics whose median differs between the driver and the
// AI by at least the metric's step, largest (in steps) first.
func differences(human, ai []Pass) []contracts.FeatureDifference {
	found := []contracts.FeatureDifference{}
	for _, t := range techniques {
		mine, theirs := metricValues(human, t.metric), metricValues(ai, t.metric)
		if len(mine) == 0 || len(theirs) == 0 {
			continue
		}
		humanMedian, aiMedian := median(mine), median(theirs)
		if math.Abs(humanMedian-aiMedian) < t.step {
			continue
		}
		found = append(found, contracts.FeatureDifference{
			Metric: t.metric, Phase: t.phase, HumanMedian: humanMedian, AiMedian: aiMedian,
			Difference: humanMedian - aiMedian, Step: t.step, HumanN: len(mine), AiN: len(theirs),
		})
	}
	sort.SliceStable(found, func(i, j int) bool {
		return math.Abs(found[i].Difference)/found[i].Step > math.Abs(found[j].Difference)/found[j].Step
	})
	if len(found) > maxDifferences {
		found = found[:maxDifferences]
	}
	return found
}
