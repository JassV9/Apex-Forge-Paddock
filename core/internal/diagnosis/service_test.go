package diagnosis

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// The sample session: a Mazda MX-5 Cup with three passes and a recorded setup.
const sampleSpool = "../ingest/testdata/spool"

// What the features worker would have written onto a pass.
const withFeatures = `
UPDATE corner_pass
SET features = features || '{"metrics": {"front_slip_mid_deg": 7.5, "rear_slip_mid_deg": 3.0,
                                         "wheelspin": null}, "speed_class": "slow"}'::jsonb
WHERE session_id = $1`

type fakeEngine struct {
	failure  error
	requests []contracts.DiagnoseRequest
}

func answer() contracts.Diagnosis {
	return contracts.Diagnosis{
		Evidence: []contracts.Evidence{{
			Id: "front_slip_mid_deg.slow", Metric: "front_slip_mid_deg",
			CornerClass: contracts.EvidenceCornerClassSlow, Value: 7.5, Passes: 3,
		}},
		Hypotheses: []contracts.Hypothesis{{
			Key: "mid_understeer.slow", Statement: "Understeer mid-corner in slow corners.",
			Phase: contracts.HypothesisPhaseMid, CornerClass: contracts.HypothesisCornerClassSlow,
			Score: 2.5, Evidence: []string{"front_slip_mid_deg.slow"},
			Alternatives: []contracts.Alternative{}, Candidates: []contracts.Candidate{},
			NotTestable: []string{},
		}},
		Explanation: contracts.Explanation{
			Provider: contracts.ExplanationProviderNone, Summary: "Strongest reading: understeer.",
			Ranking: []contracts.RankedHypothesis{},
		},
	}
}

func (f *fakeEngine) Diagnose(_ context.Context, request contracts.DiagnoseRequest) (contracts.Diagnosis, error) {
	f.requests = append(f.requests, request)
	return answer(), f.failure
}

type fixture struct {
	database  *sql.DB
	engine    *fakeEngine
	service   *Service
	sessionID int64
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
	f := &fixture{database: database, engine: &fakeEngine{}}
	// The features worker is not running here; withFeatures stands in for its work.
	if _, err := database.ExecContext(t.Context(), `UPDATE jobs SET state = 'done'`); err != nil {
		t.Fatalf("finish the features jobs: %v", err)
	}
	if err := database.QueryRowContext(t.Context(), `SELECT id FROM session`).Scan(&f.sessionID); err != nil {
		t.Fatalf("find the sample session: %v", err)
	}
	setups := experiments.NewService(database, nil, experiments.Outbox{Dir: t.TempDir()}, logger)
	f.service = NewService(database, f.engine, setups, logger)
	return f
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.database.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestRunSendsThePassesAndTheSetupAndKeepsTheAnswer(t *testing.T) {
	f := newFixture(t, "diagnosis_run")
	f.exec(t, withFeatures, f.sessionID)

	got, err := f.service.Run(t.Context(), f.sessionID)

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(got, answer()) {
		t.Errorf("Run = %+v, want the engine's answer", got)
	}
	request := f.engine.requests[0]
	if len(request.Passes) != 3 {
		t.Fatalf("engine got %d passes, want 3", len(request.Passes))
	}
	pass := request.Passes[0]
	if pass.SpeedClass != "slow" || *pass.Metrics["front_slip_mid_deg"] != 7.5 || pass.Metrics["wheelspin"] != nil {
		t.Errorf("first pass = %+v", pass)
	}
	bar := itemNamed(t, request.Items, "ARB_FRONT")
	if bar.Role != "arb" || bar.Axle != "F" || bar.Value != 3 || bar.Max != 4 || bar.ShownScale == 0 {
		t.Errorf("ARB_FRONT = %+v", bar)
	}
	stored, err := f.service.Latest(t.Context(), f.sessionID)
	if err != nil || !reflect.DeepEqual(stored, answer()) {
		t.Errorf("Latest = %+v, %v; want what Run returned", stored, err)
	}
}

func itemNamed(t *testing.T, items []contracts.DiagnoseItem, name string) contracts.DiagnoseItem {
	t.Helper()
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("engine got no item %s among %d", name, len(items))
	return contracts.DiagnoseItem{}
}

func TestEveryDiagnosisIsKeptAndTheNewestIsTheOneShown(t *testing.T) {
	f := newFixture(t, "diagnosis_history")
	f.exec(t, withFeatures, f.sessionID)
	for range 2 {
		if _, err := f.service.Run(t.Context(), f.sessionID); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	f.exec(t, `UPDATE diagnosis SET body = jsonb_set(body, '{explanation,summary}', '"newest"')
	           WHERE id = (SELECT max(id) FROM diagnosis)`)

	latest, err := f.service.Latest(t.Context(), f.sessionID)

	if err != nil || latest.Explanation.Summary != "newest" {
		t.Errorf("Latest = %q, %v; want the newest row", latest.Explanation.Summary, err)
	}
	var kept int
	if err := f.database.QueryRowContext(t.Context(), `SELECT count(*) FROM diagnosis`).Scan(&kept); err != nil || kept != 2 {
		t.Errorf("diagnoses kept = %d, %v; want 2", kept, err)
	}
}

func TestOnlyPassesThatCanBeJudgedAreSent(t *testing.T) {
	f := newFixture(t, "diagnosis_filter")
	f.exec(t, withFeatures, f.sessionID)
	f.exec(t, `UPDATE corner_pass SET valid = FALSE WHERE seq = 1`)
	f.exec(t, `UPDATE corner_pass SET setup_version_id = NULL WHERE seq = 2`)

	if _, err := f.service.Run(t.Context(), f.sessionID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(f.engine.requests[0].Passes); got != 1 {
		t.Errorf("engine got %d passes, want only the valid one on the baseline setup", got)
	}
}

func TestASessionWithoutSetupDataIsDiagnosedWithoutItems(t *testing.T) {
	f := newFixture(t, "diagnosis_no_setup")
	f.exec(t, withFeatures, f.sessionID)
	f.exec(t, `UPDATE corner_pass SET setup_version_id = NULL`)

	if _, err := f.service.Run(t.Context(), f.sessionID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	request := f.engine.requests[0]
	if len(request.Passes) != 3 || request.Items == nil || len(request.Items) != 0 {
		t.Errorf("engine got %d passes and items %v; want 3 passes and an empty list", len(request.Passes), request.Items)
	}
}

func TestRunRefusesWhatItCannotDiagnose(t *testing.T) {
	f := newFixture(t, "diagnosis_refuse")

	_, noFeatures := f.service.Run(t.Context(), f.sessionID)
	f.exec(t, `UPDATE jobs SET state = 'queued'`)
	_, stillComputing := f.service.Run(t.Context(), f.sessionID)
	f.exec(t, `UPDATE jobs SET state = 'done'`)
	_, noSession := f.service.Run(t.Context(), f.sessionID+100)
	_, noDiagnosis := f.service.Latest(t.Context(), f.sessionID)
	f.exec(t, withFeatures, f.sessionID)
	f.engine.failure = engine.ErrUnavailable
	_, engineDown := f.service.Run(t.Context(), f.sessionID)

	cases := map[string]struct{ got, want error }{
		"features not computed yet": {noFeatures, experiments.ErrConflict},
		"features being computed":   {stillComputing, experiments.ErrConflict},
		"unknown session":           {noSession, experiments.ErrNotFound},
		"nothing stored yet":        {noDiagnosis, experiments.ErrNotFound},
		"engine down":               {engineDown, engine.ErrUnavailable},
	}
	for name, tc := range cases {
		if !errors.Is(tc.got, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, tc.got, tc.want)
		}
	}
	if _, err := f.service.Latest(t.Context(), f.sessionID); !errors.Is(err, experiments.ErrNotFound) {
		t.Errorf("a failed run must store nothing; Latest err = %v", err)
	}
}
