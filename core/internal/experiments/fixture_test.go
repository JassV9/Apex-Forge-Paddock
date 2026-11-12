package experiments

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// The sample session: a Mazda MX-5 Cup on a 7-corner track, ARB_FRONT at 3 of 0..4.
const sampleSpool = "../ingest/testdata/spool"

// fakeEngine plans four blocks and hands out the queued decisions, one per Evaluate.
type fakeEngine struct {
	failure   error
	decisions []contracts.EvaluateResponseDecision
	plans     []contracts.PlanRequest
	evaluated []contracts.EvaluateRequest
}

func (f *fakeEngine) Plan(_ context.Context, request contracts.PlanRequest) (contracts.PlanResponse, error) {
	f.plans = append(f.plans, request)
	if f.failure != nil {
		return contracts.PlanResponse{}, f.failure
	}
	reply := contracts.PlanResponse{
		Sigma: 0.028, SigmaSource: contracts.PlanResponseSigmaSourceAssumed, Corners: request.Corners,
		PassesPerArm: 2 * request.LapsPerBlock, MdeDetect: 0.02, MdeDecide: 0.03,
	}
	arms := []contracts.PlanResponseBlocksElemArm{
		contracts.PlanResponseBlocksElemArmBaseline, contracts.PlanResponseBlocksElemArmVariant,
		contracts.PlanResponseBlocksElemArmBaseline, contracts.PlanResponseBlocksElemArmVariant,
	}
	for i, arm := range arms {
		reply.Blocks = append(reply.Blocks, contracts.PlanBlock{Order: i + 1, Arm: arm, LapsTarget: request.LapsPerBlock})
	}
	return reply, nil
}

func (f *fakeEngine) Evaluate(_ context.Context, request contracts.EvaluateRequest) (contracts.EvaluateResponse, error) {
	f.evaluated = append(f.evaluated, request)
	if f.failure != nil {
		return contracts.EvaluateResponse{}, f.failure
	}
	decision := f.decisions[0]
	f.decisions = f.decisions[1:]
	return contracts.EvaluateResponse{
		Decision: decision, Reason: "The engine says " + string(decision) + ".",
		Results: []contracts.ChannelResult{{
			Channel: request.SuccessTest.Channel, Role: contracts.ChannelResultRolePrimary,
			CornerClass: contracts.ChannelResultCornerClassAll, Estimate: -0.03, CiLow: -0.04,
			CiHigh: -0.02, NPasses: len(request.Passes),
			EffectClass: contracts.ChannelResultEffectClassNegative,
		}},
		Guardrails:     []contracts.GuardrailResult{},
		PredictionHits: 1, PredictionTotal: 1, PassesUsed: len(request.Passes),
	}, nil
}

type fixture struct {
	database  *sql.DB
	service   *Service
	engine    *fakeEngine
	outbox    string
	sessionID int
}

// newFixture ingests the sample session into a fresh schema and wires a service to it.
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
	f := &fixture{database: database, engine: &fakeEngine{}, outbox: t.TempDir()}
	f.sessionID = f.count(t, `SELECT id FROM session`)
	f.service = NewService(database, f.engine, Outbox{Dir: f.outbox}, logger)
	return f
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.database.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// propose creates an experiment that moves the front anti-roll bar.
func (f *fixture) propose(t *testing.T, clicks int) contracts.Experiment {
	t.Helper()
	exp, err := f.service.Create(t.Context(), contracts.ExperimentCreate{
		SessionId: f.sessionID, Parameter: "ARB_FRONT", Clicks: clicks,
		Statement: "A softer front bar gives more front grip in slow corners.",
		Predictions: []contracts.PredictionSpec{{
			Channel: "corner_time_s", Direction: contracts.PredictionSpecDirectionDecrease, Low: 0.01, High: 0.05,
		}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return exp
}

// Every block that has laps to drive and no passes yet gets an out-lap (lap 0) and its push
// laps in every corner, with features as the engine would have computed them.
const driveSQL = `
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s, features, block_id,
                         block_lap, fuel_l, tyre_core_c)
SELECT $1, c.id, $2 * 100000 + b."order" * 1000 + lap * 10 + c.number,
       now() + make_interval(secs => $2 * 100000 + b."order" * 1000 + lap * 10 + c.number),
       5.0, '{"metrics": {"corner_time_s": 5.0}, "speed_class": "slow"}'::jsonb,
       b.id, lap, 40 - lap, ARRAY[80, 81, 82, 83]::float8[]
FROM run_block b
CROSS JOIN generate_series(0, b.laps_target) AS lap
CROSS JOIN corner c
WHERE b.experiment_id = $2 AND b.laps_target > 0
  AND c.track_id = (SELECT track_id FROM session WHERE id = $1)
  AND NOT EXISTS (SELECT 1 FROM corner_pass p WHERE p.block_id = b.id)`

func (f *fixture) drive(t *testing.T, id int) {
	t.Helper()
	if _, err := f.database.ExecContext(t.Context(), driveSQL, f.sessionID, id); err != nil {
		t.Fatalf("drive experiment %d: %v", id, err)
	}
}

// run takes an experiment from proposed to the engine's verdict.
func (f *fixture) run(t *testing.T, clicks int, decision contracts.EvaluateResponseDecision) contracts.Experiment {
	t.Helper()
	exp := f.propose(t, clicks)
	id := int64(exp.Id)
	if _, err := f.service.Plan(t.Context(), id); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := f.service.Start(t.Context(), id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return f.judge(t, exp.Id, decision)
}

// judge drives the open blocks and has the engine answer with the decision.
func (f *fixture) judge(t *testing.T, id int, decision contracts.EvaluateResponseDecision) contracts.Experiment {
	t.Helper()
	f.drive(t, id)
	f.engine.decisions = append(f.engine.decisions, decision)
	exp, err := f.service.Evaluate(t.Context(), int64(id))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return exp
}

// path is the experiment's states in the order it passed through them.
func path(exp contracts.Experiment) []string {
	states := make([]string, len(exp.Events))
	for i, event := range exp.Events {
		states[i] = event.To
	}
	return states
}

func baselineLabel(t *testing.T, exp contracts.Experiment) string {
	t.Helper()
	for _, arm := range exp.Arms {
		if arm.IsBaseline {
			return arm.Label
		}
	}
	t.Fatal("experiment has no baseline arm")
	return ""
}
