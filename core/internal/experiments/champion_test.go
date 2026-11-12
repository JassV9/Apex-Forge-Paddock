package experiments

import (
	"errors"
	"fmt"
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
	if champion != 1 || original != 3 {
		t.Errorf("champion ARB_FRONT %v, original %v, want 1 (3 - 1 - 2 + 1) and 3", champion, original)
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
	if champion, _ := championValue(t, f); champion != 1 {
		t.Errorf("champion ARB_FRONT %v, want 1 kept", champion)
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
	if champion, _ := championValue(t, f); champion != 2 {
		t.Errorf("champion ARB_FRONT %v, want the second experiment's baseline 2", champion)
	}
	if since := f.count(t, `SELECT accepted_since_check FROM champion`); since != 1 {
		t.Errorf("accepted since the check = %d, want 1", since)
	}
}

func TestTheNextExperimentStartsFromTheChampionNotTheLastPass(t *testing.T) {
	tests := []struct {
		name     string
		decision contracts.EvaluateResponseDecision
		wantBase float64 // ARB_FRONT of the next experiment's baseline
	}{
		{"rejected: the baseline stays", rejected, 3},
		{"accepted: the variant is the new baseline", accepted, 2},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t, fmt.Sprintf("champion_baseline_%d", i))
			first := f.run(t, -1, test.decision)
			// The last laps of the experiment were driven on its variant.
			lastPassOnVariant(t, f, first)

			next := f.propose(t, 1)

			for _, arm := range next.Arms {
				if arm.IsBaseline && arm.Value != test.wantBase {
					t.Errorf("baseline ARB_FRONT %v, want %v", arm.Value, test.wantBase)
				}
			}
		})
	}
}

// lastPassOnVariant marks the session's newest pass as driven on the experiment's variant.
func lastPassOnVariant(t *testing.T, f *fixture, exp contracts.Experiment) {
	t.Helper()
	for _, arm := range exp.Arms {
		if arm.IsBaseline {
			continue
		}
		_, err := f.database.ExecContext(t.Context(),
			`UPDATE corner_pass SET setup_version_id = $1
			 WHERE id = (SELECT id FROM corner_pass WHERE session_id = $2 ORDER BY seq DESC LIMIT 1)`,
			arm.SetupVersionId, f.sessionID)
		if err != nil {
			t.Fatalf("mark the last pass: %v", err)
		}
	}
}

func TestAProposalTheLedgerMovedPastCannotStart(t *testing.T) {
	f := newFixture(t, "champion_stale")
	stale := f.propose(t, 1) // proposed against ARB_FRONT 3
	if _, err := f.service.Plan(t.Context(), int64(stale.Id)); err != nil {
		t.Fatal(err)
	}
	f.run(t, -1, accepted) // the champion is now ARB_FRONT 2

	_, err := f.service.Start(t.Context(), int64(stale.Id))

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("starting a stale proposal: err = %v, want ErrConflict", err)
	}
	if planned := f.count(t, `SELECT count(*) FROM experiment WHERE id = $1 AND state = 'planned'`, stale.Id); planned != 1 {
		t.Error("the refused start changed the experiment")
	}
}

func TestSetupItemsAreTheChangeableItemsOfTheNextBaseline(t *testing.T) {
	f := newFixture(t, "setup_items")
	f.run(t, -1, accepted) // the champion now has ARB_FRONT 2
	if _, err := f.database.ExecContext(t.Context(),
		`UPDATE setup_parameter SET excluded = true WHERE name = 'ARB_REAR'`); err != nil {
		t.Fatal(err)
	}

	items, err := f.service.SetupItems(t.Context(), int64(f.sessionID))

	if err != nil || len(items) == 0 {
		t.Fatalf("%d items, err %v", len(items), err)
	}
	if all := f.count(t, `SELECT count(*) FROM setup_parameter`); len(items) != all-1 {
		t.Errorf("%d items of %d: only the excluded item must be left out", len(items), all)
	}
	for i, item := range items {
		if item.Name == "ARB_REAR" {
			t.Error("an excluded item is offered for an experiment")
		}
		if item.Name == "ARB_FRONT" && (item.Value != 2 || item.Min != 0 || item.Max != 4 || item.Step != 1) {
			t.Errorf("ARB_FRONT = %+v, want the champion's value 2 in 0..4", item)
		}
		if i > 0 && items[i-1].Label > item.Label {
			t.Errorf("items are not sorted by label: %q before %q", items[i-1].Label, item.Label)
		}
	}
	if _, err := f.service.SetupItems(t.Context(), 999); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown session: err = %v, want ErrInvalid", err)
	}
}
