package training

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

func (f *fixture) start(target int) contracts.TrainingProgram {
	f.t.Helper()
	program, err := f.service.Start(context.Background(), f.session, contracts.TrainingCreate{Experiments: target})
	if err != nil {
		f.t.Fatalf("start training: %v", err)
	}
	return program
}

func (f *fixture) program(id int) contracts.TrainingProgram {
	f.t.Helper()
	program, err := f.service.Get(context.Background(), int64(id))
	if err != nil {
		f.t.Fatalf("read program: %v", err)
	}
	return program
}

func newest(program contracts.TrainingProgram) int64 {
	return int64(program.Steps[len(program.Steps)-1].ExperimentId)
}

func TestAProgramDrivesTheBestRankedChangeWithTheAI(t *testing.T) {
	f := newFixture(t, "training_first_step")
	f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1), proposal("ARB_REAR", 1)}

	program := f.start(2)

	if f.diagnose != 1 || program.State != contracts.TrainingProgramStateRunning || len(program.Steps) != 1 {
		t.Fatalf("diagnosed %d times, program %s with %d steps", f.diagnose, program.State, len(program.Steps))
	}
	request := f.ledger.created[0]
	if request.Parameter != "ARB_FRONT" || request.Driver != contracts.ExperimentCreateDriverAi ||
		!strings.HasPrefix(request.Statement, "Training: ") || *request.HypothesisKey != "entry_understeer.all" {
		t.Errorf("created %+v", request)
	}
	if len(request.Predictions) != 0 {
		t.Errorf("an interval that may be a loss is no prediction: %+v", request.Predictions)
	}
	want := []string{"create 1", "plan 1", "start 1"}
	if !slices.Equal(f.ledger.actions, want) {
		t.Errorf("actions %v, want %v", f.ledger.actions, want)
	}
}

func TestAPromisingInconclusiveIsExtendedOnceThenTheNextItemIsTested(t *testing.T) {
	f := newFixture(t, "training_extend")
	f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1), proposal("ARB_REAR", 1)}
	program := f.start(3)
	first := newest(program)
	ctx := context.Background()

	f.service.AdvanceAll(ctx)
	if last := f.ledger.actions[len(f.ledger.actions)-1]; last != "start 1" {
		t.Fatalf("an experiment still being driven is left alone, got %s", last)
	}
	f.ledger.drive(first)
	f.service.AdvanceAll(ctx)
	f.ledger.judge(first, experiments.Inconclusive, -0.0068, -0.0016)
	f.service.AdvanceAll(ctx) // extend
	f.ledger.drive(first)
	f.service.AdvanceAll(ctx) // evaluate again
	f.ledger.judge(first, experiments.Inconclusive, -0.006, 0.001)
	f.service.AdvanceAll(ctx) // revert: extended once already
	f.service.AdvanceAll(ctx) // next

	want := []string{"create 1", "plan 1", "start 1", "evaluate 1", "extend 1", "evaluate 1", "revert 1",
		"create 2", "plan 2", "start 2"}
	if !slices.Equal(f.ledger.actions, want) {
		t.Errorf("actions %v\nwant    %v", f.ledger.actions, want)
	}
	if len(f.ledger.reasons) != 1 || !strings.HasPrefix(f.ledger.reasons[0], "Training program 1 kept the baseline") {
		t.Errorf("the ledger must say the program decided, not the driver: %q", f.ledger.reasons)
	}
	if f.ledger.created[1].Parameter != "ARB_REAR" {
		t.Errorf("the second experiment tests %s, want the untested ARB_REAR", f.ledger.created[1].Parameter)
	}
}

func TestARejectedChangeIsRevertedAndTheProgramEndsAtItsTarget(t *testing.T) {
	f := newFixture(t, "training_target")
	f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1), proposal("ARB_REAR", 1)}
	program := f.start(1)
	first := newest(program)

	f.ledger.judge(first, experiments.Rejected, 0.01, 0.03)
	f.service.AdvanceAll(context.Background())
	f.service.AdvanceAll(context.Background())

	program = f.program(program.Id)
	if program.State != contracts.TrainingProgramStateDone || !strings.Contains(program.Message, "1 experiment") {
		t.Errorf("program %s: %q", program.State, program.Message)
	}
	if f.ledger.states[first].State != experiments.Reverted || len(f.ledger.created) != 1 {
		t.Errorf("first %s, %d created", f.ledger.states[first].State, len(f.ledger.created))
	}
}

func TestAProgramWaitsForTheDriver(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(f *fixture, first int64)
		want    string
	}{
		{"a trade-off is the driver's call", func(f *fixture, first int64) {
			f.ledger.judge(first, experiments.Tradeoff, -0.01, -0.002)
		}, "trade-off"},
		{"another experiment is on the car", func(f *fixture, first int64) {
			f.ledger.judge(first, experiments.Accepted, -0.01, -0.006)
			f.ledger.occupied = true
		}, "Waiting to start"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, "training_wait_"+string(rune('a'+i)))
			f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1), proposal("WING_1", 1)}
			program := f.start(3)
			tt.prepare(f, newest(program))

			f.service.AdvanceAll(context.Background())

			program = f.program(program.Id)
			if program.State != contracts.TrainingProgramStateWaiting || !strings.Contains(program.Message, tt.want) {
				t.Errorf("program %s: %q", program.State, program.Message)
			}
		})
	}
}

func TestAnOpenAIExperimentIsTakenOverAndAHumansIsLeftAlone(t *testing.T) {
	f := newFixture(t, "training_adopt")
	f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1), proposal("ARB_REAR", 1)}
	open := f.experimentRow(experiments.Inconclusive, "ai")
	f.ledger.put(open, experiments.Inconclusive, "ARB_FRONT")
	f.ledger.judge(open, experiments.Inconclusive, -0.0068, -0.0016)

	program := f.start(2)

	if len(program.Steps) != 1 || newest(program) != open || f.ledger.actions[0] != "extend 1" {
		t.Errorf("steps %+v, actions %v", program.Steps, f.ledger.actions)
	}

	h := newFixture(t, "training_human")
	h.experimentRow(experiments.Running, "human")
	_, err := h.service.Start(context.Background(), h.session, contracts.TrainingCreate{Experiments: 1})
	if !errors.Is(err, experiments.ErrConflict) || h.diagnose != 0 {
		t.Errorf("a human's experiment on the car: %v (diagnosed %d times)", err, h.diagnose)
	}
}

func TestOneProgramPerCarAndStop(t *testing.T) {
	f := newFixture(t, "training_stop")
	f.ranker.ranked = []contracts.Proposal{proposal("ARB_FRONT", -1)}
	program := f.start(2)

	_, err := f.service.Start(context.Background(), f.session, contracts.TrainingCreate{Experiments: 1})
	if !errors.Is(err, experiments.ErrConflict) {
		t.Errorf("a second program on the car: %v", err)
	}
	stopped, err := f.service.Stop(context.Background(), int64(program.Id))
	if err != nil || stopped.State != contracts.TrainingProgramStateStopped {
		t.Fatalf("stop: %v, %s", err, stopped.State)
	}
	if _, err := f.service.Stop(context.Background(), int64(program.Id)); !errors.Is(err, experiments.ErrConflict) {
		t.Errorf("stopping twice: %v", err)
	}
	if _, err := f.service.Stop(context.Background(), 999); !errors.Is(err, experiments.ErrNotFound) {
		t.Errorf("stopping no program: %v", err)
	}
	list, err := f.service.List(context.Background())
	if err != nil || len(list) != 1 || list[0].Steps[0].Parameter != "ARB_FRONT" {
		t.Errorf("list: %v %+v", err, list)
	}
}

func TestNothingLeftToTestEndsTheProgram(t *testing.T) {
	f := newFixture(t, "training_empty")
	program := f.start(3)

	program = f.program(program.Id)
	if program.State != contracts.TrainingProgramStateDone || !strings.Contains(program.Message, "nothing left") {
		t.Errorf("program %s: %q", program.State, program.Message)
	}
}

func TestPromisingMeansTheWholeIntervalIsBetter(t *testing.T) {
	tests := []struct {
		better        contracts.SuccessTestBetter
		ciLow, ciHigh float64
		cornerClass   contracts.ExperimentResultCornerClass
		wantPromising bool
	}{
		{contracts.SuccessTestBetterLower, -0.0068, -0.0016, contracts.ExperimentResultCornerClassAll, true},
		{contracts.SuccessTestBetterLower, -0.006, 0.001, contracts.ExperimentResultCornerClassAll, false},
		{contracts.SuccessTestBetterHigher, 0.001, 0.004, contracts.ExperimentResultCornerClassAll, true},
		{contracts.SuccessTestBetterLower, -0.0068, -0.0016, contracts.ExperimentResultCornerClassSlow, false},
	}
	for _, tt := range tests {
		exp := contracts.Experiment{SuccessTest: contracts.SuccessTest{Better: tt.better},
			Results: []contracts.ExperimentResult{{Role: contracts.ExperimentResultRolePrimary,
				CornerClass: tt.cornerClass, CiLow: tt.ciLow, CiHigh: tt.ciHigh}}}
		if got := promising(exp); got != tt.wantPromising {
			t.Errorf("%s [%v, %v] %s: promising %v", tt.better, tt.ciLow, tt.ciHigh, tt.cornerClass, got)
		}
	}
}
