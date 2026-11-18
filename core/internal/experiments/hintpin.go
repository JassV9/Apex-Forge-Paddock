package experiments

import (
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The range of a pinned share of the car's AI tyre hint, as contracts/experiment_create says.
const (
	minHintShare = 0.5
	maxHintShare = 1.5
)

// checkHintPin refuses a pinned AI tyre hint that is out of range or on an experiment the AI
// does not drive (docs/adr/0028-run-context-and-ai-provenance.md).
func checkHintPin(share *float64, driver contracts.ExperimentCreateDriver) error {
	if share == nil {
		return nil
	}
	if *share < minHintShare || *share > maxHintShare {
		return fmt.Errorf("ai_hint_share %v is outside %v..%v: %w", *share, minHintShare, maxHintShare, ErrInvalid)
	}
	if driver != contracts.ExperimentCreateDriverAi {
		return fmt.Errorf("ai_hint_share pins the AI's tyre hint, but the AI does not drive this experiment: %w", ErrInvalid)
	}
	return nil
}
