// Package experiments owns the life of an experiment: one setup change, planned with a success
// test fixed before driving, driven as A-B-A-B blocks against the re-driven baseline, judged
// by the engine, and kept in the ledger whatever the outcome.
//
// The state machine is the one in docs/design.md section F. Ties go to the baseline.
package experiments

import (
	"errors"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// State is where an experiment is in its life.
type State = contracts.ExperimentState

// The states, in the order an experiment can pass through them.
const (
	Proposed     = contracts.ExperimentStateProposed
	Planned      = contracts.ExperimentStatePlanned
	Running      = contracts.ExperimentStateRunning
	Evaluating   = contracts.ExperimentStateEvaluating
	Accepted     = contracts.ExperimentStateAccepted
	Rejected     = contracts.ExperimentStateRejected
	Tradeoff     = contracts.ExperimentStateTradeoff
	Inconclusive = contracts.ExperimentStateInconclusive
	Reverted     = contracts.ExperimentStateReverted
	Aborted      = contracts.ExperimentStateAborted
	Discarded    = contracts.ExperimentStateDiscarded
)

// created is the state before an experiment exists; only the first event starts from it.
const created State = ""

// MaxExtensions is how often an inconclusive experiment may be driven further.
const MaxExtensions = 2

// transitions lists every allowed move. Anything not listed is refused.
var transitions = map[State][]State{
	created:      {Proposed},
	Proposed:     {Planned, Discarded},
	Planned:      {Running, Discarded},
	Running:      {Evaluating, Aborted},
	Evaluating:   {Accepted, Rejected, Tradeoff, Inconclusive},
	Inconclusive: {Running, Reverted},  // extend (at most MaxExtensions), or keep the baseline
	Tradeoff:     {Accepted, Reverted}, // the driver decides
	Rejected:     {Reverted},           // the baseline goes back on the car
	Accepted:     {Reverted},           // rollback of the current champion
}

// CanMove reports whether an experiment may go from one state to another.
func CanMove(from, to State) bool {
	return slices.Contains(transitions[from], to)
}

// occupying are the states in which an experiment has its setups on the car: only one
// experiment at a time may be in one of them.
var occupying = []State{Running, Evaluating, Tradeoff, Inconclusive}

// Errors the service returns. Use errors.Is; the wrapped message says what happened.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid experiment")
	ErrConflict = errors.New("not possible in the current state")
)
