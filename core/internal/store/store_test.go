package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

// Integration tests need a real PostgreSQL. They run when PADDOCK_TEST_DATABASE_URL points at
// an empty database (CI provides one) and are skipped otherwise.
const envTestDatabaseURL = "PADDOCK_TEST_DATABASE_URL"

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv(envTestDatabaseURL)
	if url == "" {
		t.Skipf("%s is not set", envTestDatabaseURL)
	}
	database, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return database
}

func tableExists(t *testing.T, database *sql.DB, name string) bool {
	t.Helper()
	var exists bool
	err := database.QueryRowContext(context.Background(),
		"SELECT to_regclass('public.' || $1) IS NOT NULL", name).Scan(&exists)
	if err != nil {
		t.Fatalf("look up table %s: %v", name, err)
	}
	return exists
}

func TestMigrationsAreReversible(t *testing.T) {
	database := openTestDatabase(t)
	ctx := context.Background()

	applied, err := MigrateUp(ctx, database)
	if err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if applied == 0 {
		t.Fatal("no migration ran: the test database must start empty")
	}
	if !tableExists(t, database, "jobs") {
		t.Error("jobs table missing after migrate up")
	}

	again, err := MigrateUp(ctx, database)
	if err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	if again != 0 {
		t.Errorf("second migrate up ran %d migrations, want 0 (idempotent)", again)
	}

	if err := MigrateDownTo(ctx, database, 0); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if tableExists(t, database, "jobs") {
		t.Error("jobs table still present after migrate down")
	}
}

func TestOpenRejectsUnreachableDatabase(t *testing.T) {
	// Port 1 is never a PostgreSQL server; no network access is needed for this to fail fast.
	_, err := Open(context.Background(), "postgres://paddock@127.0.0.1:1/paddock?connect_timeout=1")
	if err == nil {
		t.Fatal("Open succeeded against an unreachable database")
	}
}
