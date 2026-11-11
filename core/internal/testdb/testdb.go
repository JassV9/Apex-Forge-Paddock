// Package testdb gives integration tests a migrated PostgreSQL schema of their own.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
)

// EnvURL names the variable that points at a PostgreSQL the tests may use. Tests that need a
// database are skipped when it is unset; CI always sets it.
const EnvURL = "PADDOCK_TEST_DATABASE_URL"

// Open returns a database whose search_path is a fresh schema with every migration applied.
// Each test package passes its own schema name, so packages can run in parallel against one
// server. The schema is dropped when the test ends.
func Open(t *testing.T, schema string) *sql.DB {
	t.Helper()
	base := os.Getenv(EnvURL)
	if base == "" {
		t.Skipf("%s is not set", EnvURL)
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, base)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	recreate := fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE; CREATE SCHEMA %q`, schema, schema)
	if _, err := admin.ExecContext(ctx, recreate); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	database, err := store.Open(ctx, withSearchPath(t, base, schema))
	if err != nil {
		t.Fatalf("open schema %s: %v", schema, err)
	}
	if _, err := store.MigrateUp(ctx, database); err != nil {
		t.Fatalf("migrate schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		_, _ = admin.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
		_ = admin.Close()
	})
	return database
}

func withSearchPath(t *testing.T, base, schema string) string {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvURL, err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
