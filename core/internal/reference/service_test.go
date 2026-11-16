package reference_test

import (
	"errors"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/reference"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// One car on two layouts. Session 1 is the driver on setup s1 (and two laps on s2); session 2
// the AI on s1 and on s2; session 3 the AI on the other layout, which must never count.
const fixtureSQL = `
INSERT INTO car (ac_id, name) VALUES ('ks_ferrari_488_gt3', 'Ferrari 488 GT3');
INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version) VALUES
  ('fn_cota', 'gp', 'COTA', 5500, '[{"number": 1, "name": "T1"}, {"number": 2, "name": "T2"}]', 1),
  ('fn_cota', 'short', 'COTA short', 3000, '[{"number": 1, "name": "T1"}]', 1);
INSERT INTO corner (track_id, number, apex_s, radius_m) VALUES (1, 1, 100, 50), (1, 2, 900, 80), (2, 1, 100, 50);
INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri) VALUES
  ('me', 1, 1, now(), 'human', '0.1.0', ''),
  ('me-ai', 1, 1, now(), 'ai', '0.1.0', ''),
  ('short-ai', 1, 2, now(), 'ai', '0.1.0', '');
INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by) VALUES
  (1, 'c1', 's1', '{"WING": 5, "ARB_FRONT": 3}', 'agent'),
  (1, 'c2', 's2', '{"WING": 5, "ARB_FRONT": 4}', 'agent');
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s, valid, setup_hash, features)
SELECT session_id, corner_id, seq, now() + seq * interval '1 second', duration_s, valid, setup_hash,
       jsonb_build_object('metrics', jsonb_build_object('brake_dist_m', brake, 'brake_peak', NULL))
FROM (VALUES
  (1, 1, 1, 5.30, true, 's1', 120), (1, 1, 2, 5.20, true, 's1', 120), (1, 1, 3, 5.10, true, 's1', 120),
  (1, 1, 4, 4.00, false, 's1', 120), (1, 2, 5, 3.00, true, 's1', 90), (1, 1, 6, 5.25, true, 's2', 120),
  (2, 1, 1, 5.00, true, 's1', 100), (2, 1, 2, 5.01, true, 's1', 100), (2, 1, 3, 4.99, true, 's1', 100),
  (2, 1, 4, 3.00, false, 's1', 100), (2, 2, 5, 3.00, true, 's2', 90), (2, 2, 6, 3.00, true, 's2', 90),
  (2, 2, 7, 3.00, true, 's2', 90),
  (3, 3, 1, 1.00, true, 's1', 100), (3, 3, 2, 1.00, true, 's1', 100), (3, 3, 3, 1.00, true, 's1', 100)
) AS p(session_id, corner_id, seq, duration_s, valid, setup_hash, brake);
`

func newService(t *testing.T, schema string) *reference.Service {
	t.Helper()
	database := testdb.Open(t, schema)
	if _, err := database.ExecContext(t.Context(), fixtureSQL); err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	return reference.NewService(database)
}

func TestSessionComparesTheLatestSetupWithTheAI(t *testing.T) {
	service := newService(t, "reference_session")

	got, err := service.Session(t.Context(), 1, "")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	if got.SetupHash == nil || *got.SetupHash != "s2" || got.Car != "ks_ferrari_488_gt3" || got.Layout != "gp" {
		t.Fatalf("compared setup %v on %s %s, want the latest pass's s2", got.SetupHash, got.Car, got.Layout)
	}
	if len(got.Setups) != 2 || got.Setups[0].SetupHash != "s1" || got.Setups[0].Passes != 4 {
		t.Errorf("setups = %+v, want s1 with 4 valid passes first", got.Setups)
	}
	if got.AiPasses != 6 {
		t.Errorf("AI passes = %d, want the 6 valid ones on this layout", got.AiPasses)
	}
	if len(got.Corners) != 1 || got.Corners[0].Match != "nearest" || *got.Corners[0].AiSetupHash != "s1" {
		t.Errorf("corners on s2 = %+v, want T1 against the AI on the nearest setup s1", got.Corners)
	}
}

func TestSessionOnAnAskedSetup(t *testing.T) {
	service := newService(t, "reference_asked")

	got, err := service.Session(t.Context(), 1, "s1")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	if len(got.Corners) != 2 {
		t.Fatalf("%d corners, want T1 and T2", len(got.Corners))
	}
	first, second := got.Corners[0], got.Corners[1]
	if first.Match != "same" || first.Class != "driver" || first.HumanN != 3 || first.AiN != 3 || *first.AiBestSessionId != 2 {
		t.Errorf("T1 = %+v", first)
	}
	if len(first.Differences) != 1 || first.Differences[0].Metric != "brake_dist_m" {
		t.Errorf("T1 differences = %+v; a null metric must be left out", first.Differences)
	}
	if second.Match != "nearest" || second.Class != "car" || *second.SetupChanges != 1 || second.Name != "T2" {
		t.Errorf("T2 = %+v, want the AI on s2 one item away", second)
	}
}

func TestSessionRefusals(t *testing.T) {
	service := newService(t, "reference_refusals")
	tests := []struct {
		name    string
		session int64
		setup   string
	}{
		{name: "no such session", session: 99},
		{name: "a setup the session never drove", session: 1, setup: "s9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Session(t.Context(), tt.session, tt.setup); !errors.Is(err, reference.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestForSetupListsTheAIOnExactlyThatSetup(t *testing.T) {
	service := newService(t, "reference_for_setup")
	tests := []struct {
		name, car, track, layout, setup string
		wantCorners                     []int
	}{
		{name: "the AI's corners on s1", car: "ks_ferrari_488_gt3", track: "fn_cota", layout: "gp", setup: "s1", wantCorners: []int{1}},
		{name: "on s2", car: "ks_ferrari_488_gt3", track: "fn_cota", layout: "gp", setup: "s2", wantCorners: []int{2}},
		{name: "a setup the AI never drove", car: "ks_ferrari_488_gt3", track: "fn_cota", layout: "gp", setup: "s9"},
		{name: "an unknown car", car: "nope", track: "fn_cota", layout: "gp", setup: "s1"},
		{name: "an unknown layout", car: "ks_ferrari_488_gt3", track: "fn_cota", layout: "", setup: "s1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.ForSetup(t.Context(), tt.car, tt.track, tt.layout, tt.setup)
			if err != nil {
				t.Fatalf("ForSetup: %v", err)
			}
			var numbers []int
			for _, corner := range got.Corners {
				numbers = append(numbers, corner.Corner)
			}
			if len(numbers) != len(tt.wantCorners) || (len(numbers) == 1 && numbers[0] != tt.wantCorners[0]) {
				t.Errorf("corners = %v, want %v", numbers, tt.wantCorners)
			}
			if got.Corners == nil || got.SetupHash != tt.setup {
				t.Errorf("reply = %+v; corners must be [] when empty", got)
			}
		})
	}
}
