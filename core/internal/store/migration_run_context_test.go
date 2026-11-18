package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// present reports whether a table, or one of its columns, is in the test's schema.
func present(t *testing.T, database *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = $1 AND ($2 = '' OR column_name = $2)`,
		table, column).Scan(&n)
	if err != nil {
		t.Fatalf("look up %s.%s: %v", table, column, err)
	}
	return n > 0
}

// Migration 00011 (docs/adr/0028-run-context-and-ai-provenance.md) goes down and up again.
func TestTheRunContextMigrationIsReversible(t *testing.T) {
	database := testdb.Open(t, "store_run_context")
	ctx := context.Background()
	objects := []struct{ table, column string }{
		{"run_context", ""}, {"experiment", "ai_hint_share"}, {"corner_pass", "ai_hint"},
		{"corner_pass", "clock"}, {"corner_pass", "abs_share"},
	}

	if err := store.MigrateDownTo(ctx, database, 10); err != nil {
		t.Fatalf("migrate down to 10: %v", err)
	}
	for _, o := range objects {
		if present(t, database, o.table, o.column) {
			t.Errorf("%s %s still present after down", o.table, o.column)
		}
	}
	if !present(t, database, "corner_pass", "ers_charge_start") {
		t.Error("down went past 00011")
	}
	if _, err := store.MigrateUp(ctx, database); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, o := range objects {
		if !present(t, database, o.table, o.column) {
			t.Errorf("%s %s missing after up", o.table, o.column)
		}
	}
}
