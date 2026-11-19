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

// evaluateRequest is what the engine judges: the compared passes, what core left out, the
// off-track count per block, and the look the blocks make.
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
		DriverChannels: driverChannels, Passes: compared.passes, Look: &look, ExtensionsLeft: &left,
		Excluded: &compared.excluded, Offtrack: compared.offtrack,
	}
	if exp.parameter != "" {
		request.Parameter = &exp.parameter
	}
	return request, nil
}
