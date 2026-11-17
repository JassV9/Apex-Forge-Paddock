package experiments

import "testing"

func TestLapsOfARestartedBlockAreToldApartBySession(t *testing.T) {
	// Block 4 of experiment 6: lap 1 was cut short by a game exit in session 72 and driven
	// again in session 74. They are two laps, not one cluster of 32 passes.
	steps := []struct {
		block, session, lap int
		want                int
	}{
		{1, 70, 1, 1001},
		{1, 70, 1, 1001}, // another pass of the same lap
		{1, 70, 2, 1002},
		{4, 72, 1, 4001},
		{4, 72, 1, 4001},
		{4, 74, 1, 4002},
		{4, 74, 2, 4003},
		{4, 74, 1, 4002}, // late arrival of an earlier lap keeps its code
	}
	laps := newLapCodes()
	for _, s := range steps {
		if got := laps.of(s.block, s.session, s.lap); got != s.want {
			t.Errorf("block %d session %d lap %d: code %d, want %d", s.block, s.session, s.lap, got, s.want)
		}
	}
}
