package discovery

import (
	"context"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// fakeLedger plays the experiment service: it keeps each experiment's state in memory and
// records every action, so the tests can follow what a sweep asked for.
type fakeLedger struct {
	f        *fixture
	states   map[int64]*contracts.Experiment
	actions  []string
	occupied bool // Start answers with a conflict, as when another experiment is on the car
	created  []contracts.ExperimentCreate
	reasons  []string // what each revert wrote in the ledger
	items    []contracts.SetupItem
	baseline int64
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

func (l *fakeLedger) CreateSweepStep(_ context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	if request.Clicks == 0 {
		return contracts.Experiment{}, fmt.Errorf("a change of 0 clicks: %w", experiments.ErrInvalid)
	}
	l.created = append(l.created, request)
	id := l.f.experimentRow(experiments.Proposed, string(request.Driver))
	l.states[id] = &contracts.Experiment{Id: int(id), State: experiments.Proposed, Parameter: request.Parameter,
		Clicks: request.Clicks, SuccessTest: contracts.SuccessTest{Level: 0.9}}
	l.record("create", id)
	return *l.states[id], nil
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

func (l *fakeLedger) SetupItems(context.Context, int64) ([]contracts.SetupItem, error) {
	return l.items, nil
}

func (l *fakeLedger) BaselineVersion(context.Context, int64) (int64, error) {
	return l.baseline, nil
}

func (l *fakeLedger) ResponseRequest(_ context.Context, id int64) (contracts.ResponseRequest, error) {
	l.record("response", id)
	return contracts.ResponseRequest{}, nil
}

// drive marks every block of the experiment as driven.
func (l *fakeLedger) drive(id int64) {
	for i := range l.states[id].Blocks {
		l.states[id].Blocks[i].LapsDone = l.states[id].Blocks[i].LapsTarget
	}
}

// judge leaves the experiment inconclusive with the engine's finding and recommendation ("" for
// none) and a time effect at the look of 8 blocks: a sweep step's verdict whenever the ledger
// keeps the step open.
func (l *fakeLedger) judge(id int64, finding contracts.VerdictSummaryFindingClass,
	next contracts.VerdictSummaryRecommendation) {
	exp := l.states[id]
	exp.State = experiments.Inconclusive
	look, margin := 1, 0.005
	exp.Verdict = &contracts.VerdictSummary{Decision: contracts.VerdictSummaryDecisionInconclusive,
		DecidedBy: contracts.VerdictSummaryDecidedByEngine, Reason: "The engine says so.", FindingClass: &finding,
		Look: &look, Margin: &margin}
	if next != "" {
		exp.Verdict.Recommendation = &next
	}
	exp.Results = []contracts.ExperimentResult{{Channel: "corner_time_s", Role: contracts.ExperimentResultRolePrimary,
		CornerClass: contracts.ExperimentResultCornerClassAll, Estimate: 0.001 * float64(exp.Clicks),
		CiLow: -0.004, CiHigh: 0.006}}
}
