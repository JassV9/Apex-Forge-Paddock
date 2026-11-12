package experiments

import (
	"errors"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// championValue is ARB_FRONT on the champion setup; original is the same on the setup the
// champion is re-checked against.
func championValue(t *testing.T, f *fixture) (champion, original float64) {
	t.Helper()
	err := f.database.QueryRowContext(t.Context(),
		`SELECT (now."values"->>'ARB_FRONT')::float8, (first."values"->>'ARB_FRONT')::float8
		 FROM champion c
		 JOIN setup_version now ON now.id = c.setup_version_id
		 JOIN setup_version first ON first.id = c.original_version_id`).Scan(&champion, &original)
	if err != nil {
		t.Fatalf("read champion: %v", err)
	}
	return champion, original
}

// threeAccepted accepts three experiments and returns the re-check that follows.
func threeAccepted(t *testing.T, f *fixture) contracts.Experiment {
	t.Helper()
	for _, clicks := range []int{-1, -2, 1} {
		if exp := f.run(t, clicks, accepted); exp.State != Accepted {
			t.Fatalf("experiment with %d clicks: state %s", clicks, exp.State)
		}
		wantOpen := 0
		if clicks == 1 {
			wantOpen = 1
		}
		if open := f.count(t, `SELECT count(*) FROM experiment WHERE kind = 'recheck'`); open != wantOpen {
			t.Fatalf("after %d clicks: %d re-checks, want %d", clicks, open, wantOpen)
		}
	}
	ledger, err := f.service.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ledger[0]
}

func TestThreeAcceptedExperimentsProposeAReCheckAgainstTheOriginal(t *testing.T) {
	f := newFixture(t, "champion_recheck")

	recheck := threeAccepted(t, f)

	if recheck.Kind != contracts.ExperimentKindRecheck || recheck.State != Proposed || recheck.Parameter != "" {
		t.Fatalf("newest experiment = %s, %s, parameter %q", recheck.Kind, recheck.State, recheck.Parameter)
	}
	champion, original := championValue(t, f)
	if champion != 4 || original != 3 {
		t.Errorf("champion ARB_FRONT %v, original %v, want 4 and 3", champion, original)
	}
	versions := map[bool]int{}
	for _, arm := range recheck.Arms {
		versions[arm.IsBaseline] = arm.SetupVersionId
	}
	againstOriginal := f.count(t, `SELECT count(*) FROM champion
		WHERE original_version_id = $1 AND setup_version_id = $2`, versions[true], versions[false])
	if againstOriginal != 1 {
		t.Errorf("re-check arms %v are not the original and the champion", versions)
	}
}

func TestAcceptedReCheckConfirmsTheChampion(t *testing.T) {
	f := newFixture(t, "champion_confirmed")
	recheck := threeAccepted(t, f)
	id := int64(recheck.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	confirmed := f.judge(t, recheck.Id, accepted)

	if confirmed.State != Accepted {
		t.Fatalf("state %s", confirmed.State)
	}
	if champion, _ := championValue(t, f); champion != 4 {
		t.Errorf("champion ARB_FRONT %v, want 4 kept", champion)
	}
	if since := f.count(t, `SELECT accepted_since_check FROM champion`); since != 0 {
		t.Errorf("accepted since the check = %d, want 0", since)
	}
	if rechecks := f.count(t, `SELECT count(*) FROM experiment WHERE kind = 'recheck'`); rechecks != 1 {
		t.Errorf("%d re-checks: confirming the champion must not propose another", rechecks)
	}
}

func TestFailedReCheckReturnsTheChampionToTheOriginal(t *testing.T) {
	f := newFixture(t, "champion_drifted")
	recheck := threeAccepted(t, f)
	id := int64(recheck.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	failed := f.judge(t, recheck.Id, rejected)

	if failed.State != Reverted {
		t.Fatalf("state %s", failed.State)
	}
	if champion, original := championValue(t, f); champion != original {
		t.Errorf("champion ARB_FRONT %v, want the original %v", champion, original)
	}
	plan := readPlan(t, f)
	if last := plan.Blocks[len(plan.Blocks)-1]; last.Values["ARB_FRONT"] != 3 || last.LapsTarget != 0 {
		t.Errorf("last block %+v: want the original setup back on the car", last)
	}
}

func TestOnlyTheCurrentChampionCanBeRolledBack(t *testing.T) {
	f := newFixture(t, "champion_rollback")
	first := f.run(t, -1, accepted)
	second := f.run(t, -2, accepted)

	_, err := f.service.Revert(t.Context(), int64(first.Id))
	if !errors.Is(err, ErrConflict) {
		t.Errorf("rolling back an older experiment: err = %v, want ErrConflict", err)
	}
	if state := f.count(t, `SELECT count(*) FROM experiment WHERE id = $1 AND state = 'accepted'`, first.Id); state != 1 {
		t.Error("the refused rollback changed the experiment")
	}

	rolledBack, err := f.service.Revert(t.Context(), int64(second.Id))

	if err != nil || rolledBack.State != Reverted {
		t.Fatalf("state %s, err %v", rolledBack.State, err)
	}
	if champion, _ := championValue(t, f); champion != 3 {
		t.Errorf("champion ARB_FRONT %v, want the second experiment's baseline 3", champion)
	}
	if since := f.count(t, `SELECT accepted_since_check FROM champion`); since != 1 {
		t.Errorf("accepted since the check = %d, want 1", since)
	}
}
