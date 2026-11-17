package experiments

import (
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// tagSQL records one push or out-lap of a block in the first `corners` corners of the track,
// `ago` seconds before now. Each call gets its own seconds so the passes never collide.
const tagSQL = `
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s, features, block_id,
                         block_lap, lap_in_session)
SELECT $1, c.id, $6 * 100 + c.number, now() - make_interval(secs => $5 - c.number),
       2.0, '{"metrics": {"corner_time_s": 5.0}}'::jsonb, b.id, $3, $6
FROM run_block b CROSS JOIN corner c
WHERE b.experiment_id = $2 AND b."order" = $7 AND c.number <= $4
  AND c.track_id = (SELECT track_id FROM session WHERE id = $1)`

type lapRun struct {
	t       *testing.T
	f       *fixture
	id      int
	session int
	gameLap int
}

// lap drives one lap of a block: block order, block lap, corners reached, seconds ago.
func (r *lapRun) lap(order, blockLap, corners, ago int) {
	r.t.Helper()
	r.gameLap++
	_, err := r.f.database.ExecContext(r.t.Context(), tagSQL, r.session, r.id, blockLap, corners, ago,
		r.gameLap, order)
	if err != nil {
		r.t.Fatalf("tag block %d lap %d: %v", order, blockLap, err)
	}
}

func (r *lapRun) lapsDone() []int {
	r.t.Helper()
	exp, err := r.f.service.Get(r.t.Context(), int64(r.id))
	if err != nil {
		r.t.Fatal(err)
	}
	done := make([]int, len(exp.Blocks))
	for i, block := range exp.Blocks {
		done[i] = block.LapsDone
	}
	return done
}

func startedRun(t *testing.T, schema string) *lapRun {
	t.Helper()
	f := newFixture(t, schema)
	exp := f.propose(t, -1)
	for _, step := range []func(contracts.Experiment) error{
		func(e contracts.Experiment) error { _, err := f.service.Plan(t.Context(), int64(e.Id)); return err },
		func(e contracts.Experiment) error { _, err := f.service.Start(t.Context(), int64(e.Id)); return err },
	} {
		if err := step(exp); err != nil {
			t.Fatal(err)
		}
	}
	return &lapRun{t: t, f: f, id: exp.Id, session: f.sessionID}
}

// driveUpToLastLap drives blocks 1 to 3 and block 4 up to its last lap, which reaches
// `corners` corners of the track's 7.
func (r *lapRun) driveUpToLastLap(corners, ago int) {
	for order := 1; order <= 4; order++ {
		for lap := 0; lap <= DefaultLapsPerBlock; lap++ {
			reached := 7
			if order == 4 && lap == DefaultLapsPerBlock {
				reached = corners
			}
			r.lap(order, lap, reached, ago)
			ago -= 10
		}
	}
}

func TestTheLastLapCountsOnlyWhenItIsComplete(t *testing.T) {
	run := startedRun(t, "laps_last")
	run.driveUpToLastLap(2, 600)
	if got := run.lapsDone(); got[3] != 2 || got[0] != 3 || got[2] != 3 {
		t.Fatalf("two corners into the last lap: laps done %v, want block 4 at 2", got)
	}
	run.gameLap--
	run.lap(4, DefaultLapsPerBlock, 7, 100)
	if got := run.lapsDone(); got[3] != 3 {
		t.Errorf("last lap complete: laps done %v, want block 4 at 3", got)
	}
}

func TestALastLapThatNeverCompletesCountsOnceNothingArrives(t *testing.T) {
	run := startedRun(t, "laps_quiet")
	run.driveUpToLastLap(4, int(quietAfter.Seconds())+600)
	if got := run.lapsDone(); got[3] != 3 {
		t.Errorf("stuck %v ago four corners into the last lap: laps done %v, want block 4 at 3", quietAfter, got)
	}
}

func TestARestartedSessionCountsTheLapDrivenAgain(t *testing.T) {
	run := startedRun(t, "laps_restart")
	run.driveUpToLastLap(7, 900)
	// Delete block 4's laps 1 to 3 and drive them as experiment 6 did: lap 1 cut short by a game
	// exit, then the new session's out-lap and laps 1 to 3, the last one under way.
	_, err := run.f.database.ExecContext(t.Context(),
		`DELETE FROM corner_pass p USING run_block b WHERE b.id = p.block_id AND b.experiment_id = $1
		 AND b."order" = 4 AND p.block_lap >= 1`, run.id)
	if err != nil {
		t.Fatal(err)
	}
	run.lap(4, 1, 5, 500)
	_, err = run.f.database.ExecContext(t.Context(),
		`INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
		 SELECT 'restart', car_id, track_id, now(), 'ai', 'test', '' FROM session WHERE id = $1`, run.session)
	if err != nil {
		t.Fatal(err)
	}
	run.session = run.f.count(t, `SELECT id FROM session WHERE spool_id = 'restart'`)
	run.gameLap = 0
	steps := []struct{ lap, corners, want int }{{0, 7, 0}, {1, 7, 1}, {2, 7, 2}, {3, 3, 2}, {3, 7, 3}}
	for i, step := range steps {
		if step.lap == 3 && step.corners == 7 {
			run.gameLap--
		}
		run.lap(4, step.lap, step.corners, 400-60*i)
		if got := run.lapsDone(); got[3] != step.want {
			t.Errorf("after lap %d (%d corners) in the new session: block 4 at %d, want %d", step.lap, step.corners, got[3], step.want)
		}
	}
}
