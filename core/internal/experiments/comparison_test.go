package experiments

import (
	"database/sql"
	"maps"
	"slices"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

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

// aiPass is a valid push-lap pass of the AI on its arm's setup.
func aiPass(block, session int, start float64) pushPass {
	return pushPass{corner: 1, block: block, session: session, lap: 1, baseline: block%2 == 1,
		valid: true, onArmSetup: true, source: "ai", driver: "ai", metrics: []byte(`{"corner_time_s": 5.1}`),
		start: start}
}

func TestPassesOnAnotherSetupOrDriverAreExcludedAndOffTrackOnesCounted(t *testing.T) {
	wrongSetup, human, offtrack, pending := aiPass(2, 1, 30), aiPass(2, 1, 40), aiPass(2, 1, 50), aiPass(1, 1, 60)
	wrongSetup.onArmSetup = false
	human.source = "human"
	offtrack.valid = false
	pending.metrics = nil
	rows := []pushPass{aiPass(1, 1, 10), aiPass(2, 1, 20), wrongSetup, human, offtrack, pending}

	got, err := compare(rows)

	if err != nil {
		t.Fatal(err)
	}
	if len(got.passes) != 2 || got.pending != 1 {
		t.Errorf("compared %d passes, %d pending; want 2 and 1", len(got.passes), got.pending)
	}
	if got.excluded != (contracts.ExcludedPasses{SetupMismatch: 1, SourceMismatch: 1}) {
		t.Errorf("excluded %+v", got.excluded)
	}
	want := []contracts.OfftrackBlock{
		{Block: 1, Arm: contracts.OfftrackBlockArmBaseline, Passes: 2, Offtrack: 0},
		{Block: 2, Arm: contracts.OfftrackBlockArmVariant, Passes: 2, Offtrack: 1},
	}
	if !slices.Equal(got.offtrack, want) {
		t.Errorf("off track %+v, want %+v", got.offtrack, want)
	}
}

func TestTimeRunsFromTheFirstPassAndEachLaterSessionHasATerm(t *testing.T) {
	known := func(value float64) sql.NullFloat64 { return sql.NullFloat64{Float64: value, Valid: true} }
	first, restarted, third := aiPass(1, 70, 1000), aiPass(2, 72, 1600), aiPass(3, 74, 2500)
	first.fuel, first.tyreCore, first.ersStart = known(31.5), known(88), known(0.72)
	cases := []struct {
		name string
		row  pushPass
		want contracts.EvaluatePassCovariates
	}{
		{"first session, all recorded", first, contracts.EvaluatePassCovariates{"time_since_start_s": 0,
			"fuel_l": 31.5, "tyre_core_c": 88, "ers_charge_start": 0.72, "session_2": 0, "session_3": 0}},
		{"after a restart", restarted, contracts.EvaluatePassCovariates{"time_since_start_s": 600,
			"session_2": 1, "session_3": 0}},
		{"third session", third, contracts.EvaluatePassCovariates{"time_since_start_s": 1500,
			"session_2": 0, "session_3": 1}},
	}
	got, err := compare([]pushPass{first, restarted, third})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		if !maps.Equal(got.passes[i].Covariates, c.want) {
			t.Errorf("%s: covariates %v, want %v", c.name, got.passes[i].Covariates, c.want)
		}
	}
}

func TestOneSessionHasNoSessionTerm(t *testing.T) {
	got, err := compare([]pushPass{aiPass(1, 70, 10), aiPass(2, 70, 20)})
	if err != nil {
		t.Fatal(err)
	}
	want := contracts.EvaluatePassCovariates{"time_since_start_s": 10}
	if !maps.Equal(got.passes[1].Covariates, want) {
		t.Errorf("covariates %v, want %v", got.passes[1].Covariates, want)
	}
}

func TestLooksFollowThePlannedBlocks(t *testing.T) {
	blocks := func(planned, restore int) []contracts.ExperimentBlock {
		out := make([]contracts.ExperimentBlock, 0, planned+restore)
		for range planned {
			out = append(out, contracts.ExperimentBlock{LapsTarget: 3, LapsDone: 3})
		}
		for range restore {
			out = append(out, contracts.ExperimentBlock{})
		}
		return out
	}
	cases := []struct{ planned, restore, want int }{
		{4, 0, 0}, {6, 1, 0}, {8, 0, 1}, {8, 1, 1}, {10, 0, 2}, {12, 1, 3},
	}
	for _, c := range cases {
		if got := Look(blocks(c.planned, c.restore)); got != c.want {
			t.Errorf("%d planned blocks (+%d restore): look %d, want %d", c.planned, c.restore, got, c.want)
		}
	}
	unfinished := blocks(8, 0)
	unfinished[7].LapsDone = 2
	if AllDriven(unfinished) || !AllDriven(blocks(8, 1)) || AllDriven(nil) {
		t.Error("every block of the plan must have its laps, a restore block has none to drive")
	}
}
