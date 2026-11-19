package experiments

import (
	"context"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The pre-registered looks (docs/adr/0029-block-level-inference-and-finding-classes.md): the
// first after FirstLookBlocks driven blocks, and one more after each extension of two blocks.
// Fewer blocks than the first look is look 0, which the engine never confirms a finding at.
const (
	FirstLookBlocks = 8
	blocksPerLook   = 2
	maxLook         = 1 + MaxExtensions
)

// AllDriven reports whether every block of the plan has its laps. A lap counts once the car has
// finished it (docs/adr/0025-finished-laps-and-lap-noise.md); a restore block has none to drive.
func AllDriven(blocks []contracts.ExperimentBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, block := range blocks {
		if block.LapsDone < block.LapsTarget {
			return false
		}
	}
	return true
}

// Look is the look the planned blocks make: 0 below FirstLookBlocks, then 1, 2, 3.
func Look(blocks []contracts.ExperimentBlock) int {
	planned := 0
	for _, block := range blocks {
		if block.LapsTarget > 0 {
			planned++
		}
	}
	if planned < FirstLookBlocks {
		return 0
	}
	return min(maxLook, 1+(planned-FirstLookBlocks)/blocksPerLook)
}

// absoluteInputs are the driver events measured in metres from the zone start (ADR 0031) that
// replace the pair measured from the pass's own slowest point, which jitters 20 to 46 m when the
// setup moves where the car is slowest (experiment 20: brake_dist_m +2.6 m while the braking
// point itself moved -0.2 m).
var absoluteInputs = map[string]string{
	"brake_dist_m":      "brake_point_m",
	"throttle_pickup_m": "throttle_pickup_point_m",
}

// driverInputs are the driver channels to test: the absolute events wherever every compared
// pass was measured with them, the apex-relative pair where features predate them, so that
// re-judging an older experiment never silently loses a driver check.
func driverInputs(passes []contracts.EvaluatePass) []string {
	out := make([]string, 0, len(driverChannels))
	for _, channel := range driverChannels {
		if absolute, known := absoluteInputs[channel]; known && allMeasure(passes, absolute) {
			channel = absolute
		}
		out = append(out, channel)
	}
	return out
}

// allMeasure reports whether there are passes and every one has the metric (null counts: it was
// computed and could not be measured on that pass).
func allMeasure(passes []contracts.EvaluatePass, metric string) bool {
	for _, pass := range passes {
		if _, ok := pass.Metrics[metric]; !ok {
			return false
		}
	}
	return len(passes) > 0
}

// evaluateRequest is what the engine judges: the compared passes, what core left out, the
// off-track count per block, the look the blocks make, and where the raw samples are for the
// car's response (docs/adr/0034-vehicle-response-in-verdicts.md), all in one request.
func evaluateRequest(ctx context.Context, q querier, exp record) (contracts.EvaluateRequest, error) {
	blocks, err := readBlocks(ctx, q, exp.id)
	if err != nil {
		return contracts.EvaluateRequest{}, fmt.Errorf("read blocks of experiment %d: %w", exp.id, err)
	}
	if !AllDriven(blocks) {
		return contracts.EvaluateRequest{}, fmt.Errorf("experiment %d: not every block is driven yet: %w", exp.id, ErrConflict)
	}
	compared, err := readComparison(ctx, q, exp.id)
	if err != nil {
		return contracts.EvaluateRequest{}, err
	}
	if compared.pending > 0 {
		return contracts.EvaluateRequest{}, fmt.Errorf(
			"experiment %d: the features of %d passes are not computed yet: %w", exp.id, compared.pending, ErrConflict)
	}
	predictions, err := readPredictions(ctx, q, exp.id)
	if err != nil {
		return contracts.EvaluateRequest{}, fmt.Errorf("read predictions of experiment %d: %w", exp.id, err)
	}
	look, left := Look(blocks), MaxExtensions-exp.extensions
	request := contracts.EvaluateRequest{
		SuccessTest: exp.test, Guardrails: exp.guardrails, Predictions: predictions,
		DriverChannels: driverInputs(compared.passes), Passes: compared.passes, Look: &look, ExtensionsLeft: &left,
		Excluded: &compared.excluded, Offtrack: compared.offtrack,
	}
	if exp.parameter != "" {
		request.Parameter = &exp.parameter
	}
	if request.Response, err = vehicleInput(ctx, q, exp.id, compared.vehicle); err != nil {
		return contracts.EvaluateRequest{}, err
	}
	return request, nil
}
