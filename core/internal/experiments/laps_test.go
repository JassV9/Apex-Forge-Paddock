package experiments

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestAPushLapCountsOnceTheCarHasFinishedIt(t *testing.T) {
	const corners = 14 // the Hungaroring's map
	full := func(lap int) lapAttempt { return lapAttempt{lap: lap, corners: corners} }
	cases := []struct {
		name     string
		attempts []lapAttempt
		movedOn  bool
		quiet    bool
		want     int
	}{
		{"nothing driven, or only the out-lap", nil, false, false, 0},
		{"three complete laps", []lapAttempt{full(1), full(2), full(3)}, false, false, 3},
		{"last lap four corners in (experiments 7 to 10)",
			[]lapAttempt{full(1), full(2), {3, 4}}, false, false, 2},
		{"first push lap under way", []lapAttempt{{1, 6}}, false, false, 0},
		{"a lap below the highest is finished even with a pass missing",
			[]lapAttempt{full(1), {2, 13}, {3, 2}}, false, false, 2},
		{"the next block started: the agent crossed the line",
			[]lapAttempt{full(1), full(2), {3, 13}}, true, false, 3},
		{"stuck or game closed: nothing arrived for a while",
			[]lapAttempt{full(1), full(2), {3, 4}}, false, true, 3},
		{"car stuck, the AI retries the lap and it is still under way",
			[]lapAttempt{full(1), full(2), {3, 5}, {3, 3}}, false, false, 2},
		{"car stuck, the retry finished the lap",
			[]lapAttempt{full(1), full(2), {3, 5}, full(3)}, false, false, 3},
		{"restarted session (experiment 6, block 4): lap 1 cut short, then driven again",
			[]lapAttempt{{1, 12}, full(1), full(2), {3, 6}}, false, false, 2},
		{"restarted session, only the out-lap of the new session so far",
			[]lapAttempt{{1, 13}}, false, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := finishedLaps(c.attempts, corners, c.movedOn, c.quiet); got != c.want {
				t.Errorf("finishedLaps = %d, want %d", got, c.want)
			}
		})
	}
}

func TestALaterBlockWithPassesFinishesTheOneBefore(t *testing.T) {
	blocks := []contracts.ExperimentBlock{
		{Order: 1, LapsTarget: 3, Passes: 40}, {Order: 2, LapsTarget: 3, Passes: 3}, {Order: 3, LapsTarget: 3},
	}
	evidence := lapEvidence{trackCorners: 14, attempts: map[int][]lapAttempt{
		1: {{1, 14}, {2, 14}, {3, 12}}, // a pass of lap 3 was dropped, but block 2 started
		2: {{1, 3}},
	}}
	countLaps(blocks, evidence)
	if blocks[0].LapsDone != 3 || blocks[1].LapsDone != 0 || blocks[2].LapsDone != 0 {
		t.Errorf("laps done = %d, %d, %d; want 3, 0, 0", blocks[0].LapsDone, blocks[1].LapsDone, blocks[2].LapsDone)
	}
}
