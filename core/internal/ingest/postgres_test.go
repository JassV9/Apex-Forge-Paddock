package ingest

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

func scanInto(t *testing.T, database *sql.DB, spoolDir string) {
	t.Helper()
	cfg := Config{SpoolDir: spoolDir, DataDir: t.TempDir(), PollInterval: time.Second}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service, err := New(cfg, NewPostgresStore(database), logger, noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := service.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}
}

func count(t *testing.T, database *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := database.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestDemoDatasetIsStoredOnceHoweverOftenItIsReplayed(t *testing.T) {
	database := testdb.Open(t, "ingest_demo")

	scanInto(t, database, demoSpool)
	// Forget that the file was ingested: the next scan replays every line.
	if _, err := database.ExecContext(context.Background(), `DELETE FROM spool_chunk`); err != nil {
		t.Fatal(err)
	}
	scanInto(t, database, demoSpool)

	for query, want := range map[string]int{
		`SELECT count(*) FROM corner_pass`:                           136,
		`SELECT count(DISTINCT corner_id) FROM corner_pass`:          17,
		`SELECT count(*) FROM corner`:                                17,
		`SELECT count(*) FROM session WHERE source = 'demo'`:         1,
		`SELECT count(*) FROM car`:                                   1,
		`SELECT count(*) FROM track`:                                 1,
		`SELECT count(*) FROM spool_chunk`:                           1,
		`SELECT count(*) FROM corner_pass WHERE fuel_l IS NULL`:      136, // v0 recorded no conditions
		`SELECT count(*) FROM corner_pass WHERE features ? 'legacy'`: 136,
	} {
		if got := count(t, database, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	perCorner := count(t, database,
		`SELECT count(*) FROM (SELECT corner_id FROM corner_pass GROUP BY corner_id HAVING count(*) = 8) c`)
	if perCorner != 17 {
		t.Errorf("%d corners have exactly 8 passes, want 17", perCorner)
	}
}

func TestAgentPassesAreStoredWithTheirConditions(t *testing.T) {
	database := testdb.Open(t, "ingest_sample")

	scanInto(t, database, sampleSpool)

	if got := count(t, database, `SELECT count(*) FROM corner_pass`); got != 3 {
		t.Fatalf("%d passes stored, want 3", got)
	}
	var fuel, coreFrontLeft, grip float64
	var samples int
	var setupHash string
	err := database.QueryRowContext(context.Background(),
		`SELECT fuel_l, tyre_core_c[1], grip, n_samples, setup_hash FROM corner_pass WHERE seq = 1`).
		Scan(&fuel, &coreFrontLeft, &grip, &samples, &setupHash)
	if err != nil {
		t.Fatalf("read pass 1: %v", err)
	}
	if fuel != 42.5 || coreFrontLeft != 90 || grip != 0.97 || samples < 100 || setupHash == "" {
		t.Errorf("pass 1 = fuel %v, core %v, grip %v, %d samples, setup %q",
			fuel, coreFrontLeft, grip, samples, setupHash)
	}
}

func TestPassesWithoutTheirSessionAreRejectedNotLost(t *testing.T) {
	database := testdb.Open(t, "ingest_orphan")
	spool := t.TempDir()
	writeChunk(t, spool, "sample-session", "000002.jsonl", string(sampleLines(t)[1])+"\n")

	scanInto(t, database, spool)

	if got := count(t, database, `SELECT count(*) FROM corner_pass`); got != 0 {
		t.Errorf("%d passes stored without a session, want 0", got)
	}
	if got := count(t, database, `SELECT rejected FROM spool_chunk WHERE file_name = '000002.jsonl'`); got != 1 {
		t.Errorf("chunk recorded %d rejected lines, want 1", got)
	}
}
