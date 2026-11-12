package experiments

import (
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// DefaultTest is the success test every experiment is judged by.
func DefaultTest() contracts.SuccessTest {
	return defaultTest
}

// DefaultGuardrails are the channels every experiment must not make worse.
func DefaultGuardrails() []contracts.GuardrailSpec {
	return slices.Clone(defaultGuardrails)
}
