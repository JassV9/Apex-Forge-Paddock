package experiments

import (
	"errors"
	"os"
	"testing"
)

// moveToAnotherCar makes the experiment's hypothesis about a second car, as if it had been
// proposed from a session of that car.
func (f *fixture) moveToAnotherCar(t *testing.T, id int) {
	t.Helper()
	_, err := f.database.ExecContext(t.Context(),
		`WITH other AS (INSERT INTO car (ac_id, name) VALUES ('ks_ferrari_488_gt3', 'Ferrari 488 GT3')
		                ON CONFLICT (ac_id) DO UPDATE SET name = EXCLUDED.name RETURNING id)
		 UPDATE hypothesis SET car_id = (SELECT id FROM other)
		 WHERE id = (SELECT hypothesis_id FROM experiment WHERE id = $1)`, id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExperimentsOnDifferentCarsCanBothBeOnTheirCars(t *testing.T) {
	f := newFixture(t, "exp_per_car")
	first, second, third := f.propose(t, -1), f.propose(t, 1), f.propose(t, -1)
	f.moveToAnotherCar(t, second.Id)
	for _, exp := range []int{first.Id, second.Id, third.Id} {
		if _, err := f.service.Plan(t.Context(), int64(exp)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.Start(t.Context(), int64(first.Id)); err != nil {
		t.Fatal(err)
	}

	if started, err := f.service.Start(t.Context(), int64(second.Id)); err != nil || started.State != Running {
		t.Errorf("the other car's experiment: state %s, err %v", started.State, err)
	}
	if _, err := f.service.Start(t.Context(), int64(third.Id)); !errors.Is(err, ErrConflict) {
		t.Errorf("a second experiment on the same car: err = %v, want ErrConflict", err)
	}
	for _, car := range []string{"ks_mazda_mx5_cup", "ks_ferrari_488_gt3"} {
		if _, err := os.Stat(RunPlanPath(f.outbox, car)); err != nil {
			t.Errorf("run plan of %s: %v", car, err)
		}
	}
}

func TestRunningExperimentsArePublishedAgainAtStartup(t *testing.T) {
	f := newFixture(t, "exp_republish")
	exp := f.propose(t, -1)
	if _, err := f.service.Plan(t.Context(), int64(exp.Id)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), int64(exp.Id)); err != nil {
		t.Fatal(err)
	}
	path := RunPlanPath(f.outbox, "ks_mazda_mx5_cup")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := f.service.PublishRunning(t.Context()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("run plan not written again: %v", err)
	}
}
