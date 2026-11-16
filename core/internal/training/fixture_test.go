package training

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

// fixture is a migrated schema with one car, track and AI session, and a fake ledger whose
// experiments are real rows (the program's steps reference them) with states held in memory.
type fixture struct {
	t        *testing.T
	db       *sql.DB
	session  int64
	car      int64
	track    int64
	setup    int64
	ledger   *fakeLedger
	ranker   *fakeRanker
	service  *Service
	diagnose int
}

func newFixture(t *testing.T, schema string) *fixture {
	t.Helper()
	f := &fixture{t: t, db: testdb.Open(t, schema)}
	f.exec(`INSERT INTO car (ac_id, name) VALUES ('ks_ferrari_488_gt3', 'Ferrari 488 GT3')`)
	f.car = f.id(`SELECT id FROM car`)
	f.exec(`INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version)
	        VALUES ('fn_cota', 'layout_f1_2026_paddock', 'COTA', 5500, '{}', 1)`)
	f.track = f.id(`SELECT id FROM track`)
	f.exec(`INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
	        VALUES ('s-ai', $1, $2, now(), 'ai', 'test', '')`, f.car, f.track)
	f.session = f.id(`SELECT id FROM session`)
	f.exec(`INSERT INTO setup_version (car_id, content_hash, "values", created_by) VALUES ($1, 'h', '{}', 'agent')`, f.car)
	f.setup = f.id(`SELECT id FROM setup_version`)
	f.ledger = &fakeLedger{f: f, states: map[int64]*contracts.Experiment{}}
	f.ranker = &fakeRanker{}
	f.service = NewService(f.db, f.ledger, f.ranker, diagnoserFunc(func() { f.diagnose++ }),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
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
	             guardrails, driver) VALUES ($1, 'ofat', $2, $3, '{}', '[]', $4) RETURNING id`,
		hypothesis, f.setup, string(state), driver)
}

type diagnoserFunc func()

func (d diagnoserFunc) Run(context.Context, int64) (contracts.Diagnosis, error) {
	d()
	return contracts.Diagnosis{}, nil
}

type fakeRanker struct{ ranked []contracts.Proposal }

func (r *fakeRanker) Proposals(context.Context, int64) (contracts.Proposals, error) {
	return contracts.Proposals{Ranked: r.ranked}, nil
}

func proposal(parameter string, clicks int) contracts.Proposal {
	return contracts.Proposal{Parameter: parameter, Clicks: clicks, Why: "a reason",
		Hypotheses: []string{"entry_understeer.all"}, GainLow: -0.02, GainHigh: 0.02}
}
