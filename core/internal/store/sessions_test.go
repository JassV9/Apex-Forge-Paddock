package store_test

import (
	"context"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/store"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

const seedSQL = `
INSERT INTO car (ac_id, name) VALUES ('car_a', 'Car A');
INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version)
VALUES ('track_a', 'gp', 'Track A', 5000, '[]', 1);
INSERT INTO corner (track_id, number, apex_s, radius_m) VALUES (1, 1, 100, 50), (1, 2, 900, 80);
INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
VALUES ('older', 1, 1, '2026-10-26T15:10:03Z', 'demo', 'test', 'x'),
       ('newer', 1, 1, '2026-11-01T09:00:00Z', 'human', 'test', 'y');
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s)
VALUES (1, 1, 1, '2026-10-26T15:10:04Z', 9.7), (1, 1, 2, '2026-10-26T15:12:04Z', 9.6),
       (1, 2, 3, '2026-10-26T15:12:30Z', 5.1);`

func TestListSessionsNewestFirstWithCounts(t *testing.T) {
	database := testdb.Open(t, "store_sessions")
	reader := store.NewReader(database)

	empty, err := reader.ListSessions(context.Background())
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty database: %d sessions, err=%v", len(empty), err)
	}
	if _, err := database.ExecContext(context.Background(), seedSQL); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := reader.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	want := []contracts.SessionSummary{
		{
			Id: 2, SpoolId: "newer", CarName: "Car A", TrackName: "Track A", Layout: "gp",
			StartedAt: "2026-11-01T09:00:00Z", Source: contracts.SessionSummarySourceHuman,
		},
		{
			Id: 1, SpoolId: "older", CarName: "Car A", TrackName: "Track A", Layout: "gp",
			StartedAt: "2026-10-26T15:10:03Z", Source: contracts.SessionSummarySourceDemo,
			Passes: 3, Corners: 2,
		},
	}
	if len(got) != len(want) {
		t.Fatalf("%d sessions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("session %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
