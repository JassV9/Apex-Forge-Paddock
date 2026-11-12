package experiments

import "testing"

func TestOnlyTheDesignsTransitionsAreAllowed(t *testing.T) {
	all := []State{
		created, Proposed, Planned, Running, Evaluating, Accepted, Rejected, Tradeoff,
		Inconclusive, Reverted, Aborted, Discarded,
	}
	allowed := map[[2]State]bool{
		{created, Proposed}:        true,
		{Proposed, Planned}:        true,
		{Proposed, Discarded}:      true,
		{Planned, Running}:         true,
		{Planned, Discarded}:       true,
		{Running, Evaluating}:      true,
		{Running, Aborted}:         true,
		{Evaluating, Accepted}:     true,
		{Evaluating, Rejected}:     true,
		{Evaluating, Tradeoff}:     true,
		{Evaluating, Inconclusive}: true,
		{Inconclusive, Running}:    true,
		{Inconclusive, Reverted}:   true,
		{Tradeoff, Accepted}:       true,
		{Tradeoff, Reverted}:       true,
		{Rejected, Reverted}:       true,
		{Accepted, Reverted}:       true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := CanMove(from, to); got != allowed[[2]State{from, to}] {
				t.Errorf("CanMove(%q, %q) = %v", from, to, got)
			}
		}
	}
}

func TestFinishedExperimentsStayFinished(t *testing.T) {
	for _, state := range []State{Reverted, Aborted, Discarded} {
		if len(transitions[state]) != 0 {
			t.Errorf("%s can still move to %v", state, transitions[state])
		}
	}
}
