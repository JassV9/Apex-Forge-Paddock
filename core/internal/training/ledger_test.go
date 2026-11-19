package training

import (
	"context"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// fakeLedger plays the experiment service: it keeps each experiment's state in memory and
// records every action, so the tests can follow what a program asked for.
type fakeLedger struct {
	f        *fixture
	states   map[int64]*contracts.Experiment
	actions  []string
	occupied bool // Start answers with a conflict, as when another experiment is on the car
	created  []contracts.ExperimentCreate
	reasons  []string // what each revert wrote in the ledger
}

func (l *fakeLedger) record(action string, id int64) {
	l.actions = append(l.actions, fmt.Sprintf("%s %d", action, id))
}

func (l *fakeLedger) Get(_ context.Context, id int64) (contracts.Experiment, error) {
	exp, ok := l.states[id]
	if !ok {
		return contracts.Experiment{}, fmt.Errorf("experiment %d: %w", id, experiments.ErrNotFound)
	}
	return *exp, nil
}

// put registers an experiment row the fake knows about.
func (l *fakeLedger) put(id int64, state experiments.State, parameter string) *contracts.Experiment {
	exp := &contracts.Experiment{Id: int(id), State: state, Parameter: parameter, Clicks: -1,
		SuccessTest: contracts.SuccessTest{Better: contracts.SuccessTestBetterLower}}
	l.states[id] = exp
	return exp
}

func (l *fakeLedger) Create(_ context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	l.created = append(l.created, request)
	id := l.f.experimentRow(experiments.Proposed, string(request.Driver))
	exp := l.put(id, experiments.Proposed, request.Parameter)
	exp.Clicks = request.Clicks
	l.record("create", id)
	return *exp, nil
}

func (l *fakeLedger) move(action string, id int64, to experiments.State) (contracts.Experiment, error) {
	l.record(action, id)
	exp, ok := l.states[id]
	if !ok {
		return contracts.Experiment{}, experiments.ErrNotFound
	}
	exp.State = to
	return *exp, nil
}

func (l *fakeLedger) Plan(_ context.Context, id int64) (contracts.Experiment, error) {
	return l.move("plan", id, experiments.Planned)
}

func (l *fakeLedger) Start(_ context.Context, id int64) (contracts.Experiment, error) {
	if l.occupied {
		l.record("start refused", id)
		return contracts.Experiment{}, fmt.Errorf("experiment 99 is still on the car: %w", experiments.ErrConflict)
	}
	exp, err := l.move("start", id, experiments.Running)
	l.states[id].Blocks = []contracts.ExperimentBlock{{Order: 1, LapsTarget: 3}, {Order: 2, LapsTarget: 3}}
	return exp, err
}

func (l *fakeLedger) Evaluate(_ context.Context, id int64) (contracts.Experiment, error) {
	return l.move("evaluate", id, experiments.Evaluating)
}

func (l *fakeLedger) Extend(_ context.Context, id int64) (contracts.Experiment, error) {
	exp, err := l.move("extend", id, experiments.Running)
	l.states[id].Extensions++
	return exp, err
}

func (l *fakeLedger) RevertByTraining(_ context.Context, id int64, reason string) (contracts.Experiment, error) {
	l.reasons = append(l.reasons, reason)
	return l.move("revert", id, experiments.Reverted)
}

// drive marks every block of the experiment as driven.
func (l *fakeLedger) drive(id int64) {
	for i := range l.states[id].Blocks {
		l.states[id].Blocks[i].LapsDone = l.states[id].Blocks[i].LapsTarget
	}
}

// judge gives the experiment the engine's verdict and what it recommends ("" for nothing).
func (l *fakeLedger) judge(id int64, state experiments.State, next contracts.VerdictSummaryRecommendation) {
	exp := l.states[id]
	exp.State = state
	exp.Verdict = &contracts.VerdictSummary{Decision: contracts.VerdictSummaryDecision(state),
		DecidedBy: contracts.VerdictSummaryDecidedByEngine}
	if next != "" {
		exp.Verdict.Recommendation = &next
	}
}
