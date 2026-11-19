package experiments

import (
	"errors"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestAManualEvaluateWaitsForEveryBlock(t *testing.T) {
	f := newFixture(t, "exp_undriven")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	for _, step := range []func() error{
		func() error { _, err := f.service.Plan(t.Context(), id); return err },
		func() error { _, err := f.service.Start(t.Context(), id); return err },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	f.drive(t, exp.Id)
	// The last block's last lap never happened.
	_, err := f.database.ExecContext(t.Context(), `DELETE FROM corner_pass WHERE block_id =
		(SELECT id FROM run_block WHERE experiment_id = $1 ORDER BY "order" DESC LIMIT 1) AND block_lap >= 2`, id)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.service.Evaluate(t.Context(), id)

	if !errors.Is(err, ErrConflict) || len(f.engine.evaluated) != 0 {
		t.Errorf("err = %v after %d engine calls, want ErrConflict and none", err, len(f.engine.evaluated))
	}
}

// spoil makes four push passes of a driven experiment unfit: one on the baseline's setup in a
// variant block, one from a session the AI drove, two off track.
func spoil(t *testing.T, f *fixture, id int64) {
	t.Helper()
	statements := []string{
		// corner 1 of block 2, lap 1: driven on the baseline's setup
		`UPDATE corner_pass p SET setup_version_id = e.baseline_version_id FROM run_block b, experiment e
		 WHERE p.block_id = b.id AND e.id = b.experiment_id AND b.experiment_id = $1 AND b."order" = 2
		   AND p.block_lap = 1 AND p.corner_id = (SELECT min(corner_id) FROM corner_pass)`,
		// corner 1 of block 3, lap 1: from a session the AI drove
		`WITH ai AS (INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
		            SELECT 's-ai', car_id, track_id, started_at, 'ai', 'test', '' FROM session LIMIT 1 RETURNING id)
		 UPDATE corner_pass p SET session_id = (SELECT id FROM ai) FROM run_block b
		 WHERE p.block_id = b.id AND b.experiment_id = $1 AND b."order" = 3 AND p.block_lap = 1
		   AND p.corner_id = (SELECT min(corner_id) FROM corner_pass)`,
		// two passes of block 4, lap 2: off track
		`UPDATE corner_pass p SET valid = false, invalid_reason = 'off track' FROM run_block b
		 WHERE p.block_id = b.id AND b.experiment_id = $1 AND b."order" = 4 AND p.block_lap = 2
		   AND p.corner_id IN (SELECT corner_id FROM corner_pass GROUP BY corner_id ORDER BY corner_id LIMIT 2)`,
	}
	for _, statement := range statements {
		if _, err := f.database.ExecContext(t.Context(), statement, id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPassesOnAnotherSetupOrFromAnotherDriverAreLeftOutAndOffTrackOnesCounted(t *testing.T) {
	f := newFixture(t, "exp_excluded")
	exp := f.propose(t, -1)
	id := int64(exp.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	f.drive(t, exp.Id)
	spoil(t, f, id)
	f.engine.decisions = append(f.engine.decisions, inconclusive)

	if _, err := f.service.Evaluate(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	sent := f.engine.evaluated[0]
	if sent.Excluded == nil || *sent.Excluded != (contracts.ExcludedPasses{SetupMismatch: 1, SourceMismatch: 1}) {
		t.Errorf("excluded %+v", sent.Excluded)
	}
	if len(sent.Passes) != 4*DefaultLapsPerBlock*7-4 {
		t.Errorf("%d passes compared, want all push passes but the 4 left out or off track", len(sent.Passes))
	}
	offtrack := map[int]int{}
	for _, block := range sent.Offtrack {
		offtrack[block.Block] = block.Offtrack
	}
	if offtrack[4] != 2 || offtrack[1] != 0 || len(sent.Offtrack) != 4 {
		t.Errorf("off track per block %+v", sent.Offtrack)
	}
	if sent.Look == nil || *sent.Look != 0 || sent.ExtensionsLeft == nil || *sent.ExtensionsLeft != MaxExtensions ||
		sent.Parameter == nil || *sent.Parameter != "ARB_FRONT" {
		t.Errorf("look %v, extensions left %v, parameter %v", sent.Look, sent.ExtensionsLeft, sent.Parameter)
	}
}

func TestResultsAreKeptForEveryVerdictWithTheEngineVersion(t *testing.T) {
	f := newFixture(t, "exp_append")
	first := f.run(t, -1, inconclusive)
	id := int64(first.Id)
	if _, err := f.service.Extend(t.Context(), id); err != nil {
		t.Fatal(err)
	}

	second := f.judge(t, first.Id, rejected)

	sets := f.count(t, `SELECT count(DISTINCT verdict_id) FROM result WHERE experiment_id = $1
	                    AND engine_version = 'test-engine'`, id)
	if sets != 2 || len(second.Results) != 1 || second.Results[0].NPasses != 6*DefaultLapsPerBlock*7 {
		t.Errorf("%d result sets kept, latest shows %+v", sets, second.Results)
	}
	v := second.Verdict
	if v.DecidedBy != contracts.VerdictSummaryDecidedByDriver && v.DecidedBy != contracts.VerdictSummaryDecidedByEngine {
		t.Fatalf("verdict %+v", v)
	}
	if v.FindingClass == nil || *v.FindingClass != contracts.VerdictSummaryFindingClassNeutral || *v.FindingDerived ||
		v.EngineVersion == nil || *v.EngineVersion != "test-engine" || len(v.Flags) != 1 || len(v.Confounders) != 1 {
		t.Errorf("finding of the latest engine verdict not read back: %+v", v)
	}
}

func TestOldVerdictsGetAFindingDerivedFromTheirDecision(t *testing.T) {
	f := newFixture(t, "exp_derived")
	exp := f.propose(t, -1)
	cases := []struct {
		decision, reason string
		want             contracts.VerdictSummaryFindingClass
	}{
		{"accepted", "Better on corner_time_s ...", contracts.VerdictSummaryFindingClassConfirmedImprovement},
		{"rejected", "Worse on corner_time_s ...", contracts.VerdictSummaryFindingClassConfirmedRegression},
		{"rejected", "No meaningful effect on corner_time_s ...", contracts.VerdictSummaryFindingClassNeutral},
		{"rejected", "No clear gain on corner_time_s ...", contracts.VerdictSummaryFindingClassInsufficientEvidence},
		{"inconclusive", "Unclear on corner_time_s ...", contracts.VerdictSummaryFindingClassInsufficientEvidence},
		{"inconclusive", "Not enough passes on both setups ...", contracts.VerdictSummaryFindingClassInvalid},
	}
	for _, c := range cases {
		_, err := f.database.ExecContext(t.Context(), `INSERT INTO verdict (experiment_id, decision, reason, decided_by)
		                                               VALUES ($1, $2, $3, 'engine')`, exp.Id, c.decision, c.reason)
		if err != nil {
			t.Fatal(err)
		}
		v, err := readVerdict(t.Context(), f.database, int64(exp.Id))
		if err != nil || v.FindingClass == nil || *v.FindingClass != c.want || !*v.FindingDerived {
			t.Errorf("%s %q: %v %+v, want %s derived", c.decision, c.reason, err, v, c.want)
		}
	}
}
