package store_test

import (
	"context"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// Migration 00013 (docs/adr/0034-vehicle-response-in-verdicts.md) goes down and up again, and
// current_result is the 00012 view again after down.
func TestTheResponseResultsMigrationIsReversible(t *testing.T) {
	database := testdb.Open(t, "store_response_results")
	ctx := context.Background()
	added := []struct{ table, column string }{
		{"result", "response_finding"}, {"result", "expected"}, {"result", "margin"},
		{"current_response", ""},
	}

	if err := store.MigrateDownTo(ctx, database, 12); err != nil {
		t.Fatalf("migrate down to 12: %v", err)
	}
	for _, o := range added {
		if present(t, database, o.table, o.column) {
			t.Errorf("%s %s still present after down", o.table, o.column)
		}
	}
	if !present(t, database, "current_result", "engine_version") || !present(t, database, "result", "verdict_id") {
		t.Error("down went past 00013, or current_result was not put back")
	}
	if _, err := store.MigrateUp(ctx, database); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, o := range added {
		if !present(t, database, o.table, o.column) {
			t.Errorf("%s %s missing after up", o.table, o.column)
		}
	}
	if present(t, database, "current_result", "response_finding") {
		t.Error("current_result shows the response columns: it should keep 00012's")
	}
}
