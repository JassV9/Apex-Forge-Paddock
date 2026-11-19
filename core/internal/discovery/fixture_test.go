package discovery

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// fixture is a migrated schema with the FH26 at the Hungaroring, one AI session on a setup
// the agent read back, and a fake ledger and engine. The ledger's experiments are real rows
// (the steps reference them) with their states held in memory.
type fixture struct {
	t       *testing.T
	db      *sql.DB
	session int64
	car     int64
	track   int64
	setup   int64
	ledger  *fakeLedger
	engine  *fakeEngine
	service *Service
}

func newFixture(t *testing.T, schema string) *fixture {
	t.Helper()
	f := &fixture{t: t, db: testdb.Open(t, schema)}
	f.car = f.id(`INSERT INTO car (ac_id, name) VALUES ('rss_formula_hybrid_2026', 'Formula Hybrid 2026')
	              RETURNING id`)
	f.track = f.id(`INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version)
	                VALUES ('hungaroring', '', 'Hungaroring', 4381, '{}', 1) RETURNING id`)
	f.session = f.id(`INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
	                  VALUES ('s-ai', $1, $2, now(), 'ai', 'test', '') RETURNING id`, f.car, f.track)
	f.setup = f.id(`INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by)
	                VALUES ($1, 'h', 'a', '{"ARB_REAR": 3, "WING_1": 1, "WING_9": 1}', 'agent') RETURNING id`, f.car)
	f.exec(`INSERT INTO setup_parameter (car_id, name, label, min, max, step)
	        VALUES ($1, 'ARB_REAR', 'Rear ARB', 0, 5, 1), ($1, 'WING_1', 'Rear Wing', 0, 2, 1),
	               ($1, 'WING_9', 'Rear Wing', 0, 2, 1)`, f.car)
	f.ledger = &fakeLedger{f: f, states: map[int64]*contracts.Experiment{}, baseline: f.setup}
	f.ledger.items = []contracts.SetupItem{
		{Name: "ARB_REAR", Label: "Rear ARB", Min: 0, Max: 5, Step: 1, Value: 3},
		{Name: "WING_1", Label: "Rear Wing (WING_1 + WING_9)", Min: 0, Max: 2, Step: 1, Value: 1},
	}
	f.engine = &fakeEngine{}
	f.service = NewService(f.db, f.ledger, f.engine, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.ExecContext(context.Background(), query, args...); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
}

func (f *fixture) id(query string, args ...any) int64 {
	f.t.Helper()
	var id int64
	if err := f.db.QueryRowContext(context.Background(), query, args...).Scan(&id); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
	return id
}

// experimentRow stores an experiment row in the given state and driver and returns its id.
func (f *fixture) experimentRow(state experiments.State, driver string) int64 {
	f.t.Helper()
	hypothesis := f.id(`INSERT INTO hypothesis (car_id, track_id, statement, source)
	                    VALUES ($1, $2, 'test', 'rules') RETURNING id`, f.car, f.track)
	return f.id(`INSERT INTO experiment (hypothesis_id, kind, baseline_version_id, state, success_test,
	             guardrails, driver, purpose) VALUES ($1, 'ofat', $2, $3, '{}', '[]', $4, 'sweep') RETURNING id`,
		hypothesis, f.setup, string(state), driver)
}

// sweep starts a sweep of the item at a pinned hint of 0.93.
func (f *fixture) sweep(parameter string, steps ...int) contracts.Sweep {
	f.t.Helper()
	sweep, err := f.service.Start(context.Background(), f.session,
		contracts.SweepCreate{Parameter: parameter, Steps: steps, AiHintShare: 0.93})
	if err != nil {
		f.t.Fatalf("start sweep: %v", err)
	}
	return sweep
}

func (f *fixture) read(id int) contracts.Sweep {
	f.t.Helper()
	sweep, err := f.service.Get(context.Background(), int64(id))
	if err != nil {
		f.t.Fatalf("read sweep: %v", err)
	}
	return sweep
}

func newest(sweep contracts.Sweep) int64 {
	return int64(sweep.Steps[len(sweep.Steps)-1].ExperimentId)
}

// fakeEngine answers the response analysis with two cells, one the item's row expects, and
// the curve with one linear curve; it keeps what it was asked.
type fakeEngine struct {
	responses int
	curves    []contracts.DiscoveryCurveRequest
	failure   error
}

func (e *fakeEngine) Response(context.Context, contracts.ResponseRequest) (contracts.ResponseResponse, error) {
	e.responses++
	estimate, low, high, margin := 0.1, 0.05, 0.15, 0.05
	cell := contracts.ResponseEffect{Metric: "rear_slip_deg", Phase: "mid", Regime: "all", Estimate: &estimate,
		CiLow: &low, CiHigh: &high, Margin: &margin, Expected: contracts.ResponseResponseEffectsElemExpectedDirect,
		Kind: contracts.ResponseResponseEffectsElemKindDirect, Finding: contracts.ResponseResponseEffectsElemFindingIncrease}
	unexpected := cell
	unexpected.Metric, unexpected.Expected = "lat_g_mean", contracts.ResponseResponseEffectsElemExpectedNone
	return contracts.ResponseResponse{Effects: []contracts.ResponseEffect{cell, unexpected}}, nil
}

func (e *fakeEngine) Curve(_ context.Context, request contracts.DiscoveryCurveRequest) (contracts.DiscoveryCurve, error) {
	e.curves = append(e.curves, request)
	if e.failure != nil {
		return contracts.DiscoveryCurve{}, e.failure
	}
	return contracts.DiscoveryCurve{Parameter: request.Parameter, Level: 0.9, EngineVersion: "test",
		Curves: []contracts.CurveResult{{Metric: "rear_slip_deg", Phase: "mid", Regime: "all", Source: "response",
			Unit: "deg", Shape: contracts.CurveResultShapeLinear, Clicks: []int{-1, 1, 2}, Reason: "straight"}}}, nil
}
