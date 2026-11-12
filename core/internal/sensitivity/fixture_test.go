package sensitivity

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"math"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// The sample session: a Mazda MX-5 Cup on a 7-corner track, with a recorded setup.
const sampleSpool = "../ingest/testdata/spool"

const fakeModel = "fake-1"

// fakeEngine echoes: every measurement comes back as a measured effect, every transfer as a
// prior at half its size. The pooling and the shrinkage themselves are the engine's tests.
type fakeEngine struct {
	failure  error
	asked    []contracts.SensitivityRequest
	proposed []contracts.ProposeRequest
}

func (f *fakeEngine) Sensitivity(_ context.Context, request contracts.SensitivityRequest) (contracts.SensitivityResponse, error) {
	f.asked = append(f.asked, request)
	reply := contracts.SensitivityResponse{ModelVersion: fakeModel, Effects: []contracts.SensitivityEffect{}}
	for _, m := range request.Measurements {
		reply.Effects = append(reply.Effects, contracts.SensitivityEffect{
			Parameter: m.Parameter, Clicks: m.Clicks, Channel: m.Channel,
			CornerClass: contracts.SensitivityEffectCornerClass(m.CornerClass),
			Estimate:    m.Estimate, CiLow: m.CiLow, CiHigh: m.CiHigh, NPasses: m.NPasses,
			EffectClass:   contracts.SensitivityEffectEffectClassNegative,
			ExperimentIds: []int{m.ExperimentId},
		})
	}
	for _, tr := range request.Transfers {
		reply.Effects = append(reply.Effects, contracts.SensitivityEffect{
			Parameter: tr.Parameter, Clicks: tr.Clicks, Channel: tr.Channel,
			CornerClass: contracts.SensitivityEffectCornerClass(tr.CornerClass),
			Estimate:    tr.Estimate / 2, CiLow: tr.CiLow, CiHigh: -tr.CiLow,
			EffectClass: contracts.SensitivityEffectEffectClassInconclusive,
			IsPrior:     true, PriorSource: tr.SourceCar, Note: "a prior", ExperimentIds: []int{},
		})
	}
	return reply, f.failure
}

func (f *fakeEngine) Propose(_ context.Context, request contracts.ProposeRequest) (contracts.Proposals, error) {
	f.proposed = append(f.proposed, request)
	return contracts.Proposals{
		Stage:   contracts.ProposalsStageColdStart,
		Ranked:  []contracts.Proposal{{Parameter: "ARB_FRONT", Clicks: -1, LapsNeeded: 16}},
		Skipped: []contracts.SkippedCandidate{},
	}, f.failure
}

type fakeDiagnoses struct {
	failure error
}

// Two hypotheses name the same softer front bar; the second also names a rear change.
func (f fakeDiagnoses) Latest(context.Context, int64) (contracts.Diagnosis, error) {
	softer := contracts.Candidate{Parameter: "ARB_FRONT", Label: "ARB Front", Clicks: -1, Why: "softer front"}
	stiffer := contracts.Candidate{Parameter: "ARB_REAR", Label: "ARB Rear", Clicks: 1, Why: "stiffer rear"}
	return contracts.Diagnosis{Hypotheses: []contracts.Hypothesis{
		{Key: "mid_understeer.slow", Candidates: []contracts.Candidate{softer}},
		{Key: "entry_understeer.slow", Candidates: []contracts.Candidate{softer, stiffer}},
	}}, f.failure
}

type fixture struct {
	database  *sql.DB
	engine    *fakeEngine
	diagnoses *fakeDiagnoses
	service   *Service
	sessionID int64
	carID     int64
}

func newFixture(t *testing.T, schema string) *fixture {
	t.Helper()
	database := testdb.Open(t, schema)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := ingest.Config{SpoolDir: sampleSpool, DataDir: t.TempDir(), PollInterval: time.Second}
	ingester, err := ingest.New(cfg, ingest.NewPostgresStore(database), logger, noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("ingest.New: %v", err)
	}
	if _, err := ingester.ScanOnce(t.Context()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
	f := &fixture{database: database, engine: &fakeEngine{}, diagnoses: &fakeDiagnoses{}}
	f.scan(t, `SELECT id, car_id FROM session`, nil, &f.sessionID, &f.carID)
	f.service = NewService(database, f.engine, f.diagnoses, logger)
	return f
}

func (f *fixture) scan(t *testing.T, query string, args []any, into ...any) {
	t.Helper()
	if err := f.database.QueryRowContext(t.Context(), query, args...).Scan(into...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.database.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// entry is one experiment in the ledger, written directly: how experiments get there is the
// experiments package's business.
type entry struct {
	car       int64
	parameter string
	clicks    int
	state     string
	kind      string
	source    string
	level     float64
}

const entrySQL = `
WITH h AS (
    INSERT INTO hypothesis (car_id, track_id, statement, source)
    SELECT $1::bigint, (SELECT track_id FROM session LIMIT 1), 'made up', $2::text
    RETURNING id)
INSERT INTO experiment (hypothesis_id, kind, baseline_version_id, parameter_id, clicks, state,
                        success_test, guardrails)
SELECT h.id, $3::text, (SELECT id FROM setup_version LIMIT 1),
       (SELECT id FROM setup_parameter WHERE car_id = $1::bigint AND name = $4::text),
       $5::int, $6::text, jsonb_build_object('channel', 'corner_time_s', 'level', $7::float8), '[]'
FROM h RETURNING id`

// add writes an experiment; unset fields are a finished one-factor experiment by a human on
// the sample car, judged at the default level.
func (f *fixture) add(t *testing.T, e entry) int {
	t.Helper()
	defaults := entry{car: f.carID, state: "accepted", kind: "ofat", source: "human", level: experiments.DefaultTest().Level}
	if e.car == 0 {
		e.car = defaults.car
	}
	if e.state == "" {
		e.state = defaults.state
	}
	if e.kind == "" {
		e.kind = defaults.kind
	}
	if e.source == "" {
		e.source = defaults.source
	}
	if e.level == 0 {
		e.level = defaults.level
	}
	var id int
	f.scan(t, entrySQL, []any{e.car, e.source, e.kind, e.parameter, e.clicks, e.state, e.level}, &id)
	return id
}

// result gives the experiment a measured effect on corner time over all corners.
func (f *fixture) result(t *testing.T, id int, estimate float64) {
	t.Helper()
	low, high := math.Round((estimate-0.01)*1000)/1000, math.Round((estimate+0.01)*1000)/1000
	f.exec(t, `INSERT INTO result (experiment_id, channel, role, estimate, ci_low, ci_high, n_passes, effect_class)
	           VALUES ($1, 'corner_time_s', 'primary', $2, $3, $4, 84, 'negative')`, id, estimate, low, high)
}

func (f *fixture) verdict(t *testing.T, id int, decidedBy string, hits, total int) {
	t.Helper()
	decision := map[string]string{"engine": "accepted", "driver": "reverted"}[decidedBy]
	f.exec(t, `INSERT INTO verdict (experiment_id, decision, reason, prediction_hits, prediction_total, decided_by)
	           VALUES ($1, $2, $3, $4, $5, $6)`, id, decision, fmt.Sprintf("by the %s", decidedBy), hits, total, decidedBy)
}

// finished is an accepted experiment with its result and the engine's verdict.
func (f *fixture) finished(t *testing.T, e entry, estimate float64) int {
	t.Helper()
	id := f.add(t, e)
	f.result(t, id, estimate)
	f.verdict(t, id, "engine", 1, 1)
	return id
}
