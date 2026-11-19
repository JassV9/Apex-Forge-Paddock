package store_test

import (
	"context"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// Migration 00014 (docs/adr/0033-stratified-vehicle-sensitivity-profile.md) goes down and up
// again: the new tables and columns go, is_prior comes back, and up restores the strata.
func TestTheStratifiedSensitivityMigrationIsReversible(t *testing.T) {
	database := testdb.Open(t, "store_sensitivity")
	ctx := context.Background()
	added := []struct{ table, column string }{
		{"sensitivity_derivation", ""}, {"sensitivity_measurement", ""}, {"sensitivity_prior", ""},
		{"sensitivity_profile", ""}, {"sensitivity_effect", "track"}, {"sensitivity_effect", "hint_bucket"},
		{"sensitivity_effect", "knowledge_level"}, {"sensitivity_effect", "engine_versions"},
	}

	if err := store.MigrateDownTo(ctx, database, 13); err != nil {
		t.Fatalf("migrate down to 13: %v", err)
	}
	for _, o := range added {
		if present(t, database, o.table, o.column) {
			t.Errorf("%s %s still present after down", o.table, o.column)
		}
	}
	if !present(t, database, "sensitivity_effect", "is_prior") || !present(t, database, "result", "verdict_id") {
		t.Error("down did not restore is_prior, or went past 00012")
	}
	if _, err := store.MigrateUp(ctx, database); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, o := range added {
		if !present(t, database, o.table, o.column) {
			t.Errorf("%s %s missing after up", o.table, o.column)
		}
	}
	if present(t, database, "sensitivity_effect", "is_prior") || present(t, database, "sensitivity_prior", "effect_class") {
		t.Error("a prior column survived up: priors have their own table and no class")
	}
}
