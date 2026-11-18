package reference_test

import (
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/reference"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// Two tracks, each an original layout and its Paddock test copy. At the Hungaroring the copy
// is a byte copy (same corner map); at COTA the copy's corner 2 is 50 m away, so it is not the
// same track. Sessions 1 and 3 are the driver on the originals, 2 and 4 the AI on the copies.
const variantsSQL = `
INSERT INTO car (ac_id, name) VALUES ('rss_formula_hybrid_2026', 'Formula Hybrid 2026');
INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version) VALUES
  ('fn_hungaroring', 'layout_f1_2026', 'Hungaroring F1 2026', 4342,
   '[{"number": 1, "name": "T1", "apex_m": 432.11}, {"number": 2, "name": "T2", "apex_m": 900}]', 1),
  ('fn_hungaroring', 'layout_f1_2026_paddock', 'Hungaroring F1 2026 (Paddock test)', 4342,
   '[{"number": 1, "name": "T1", "apex_m": 432.11}, {"number": 2, "name": "T2", "apex_m": 900}]', 1),
  ('fn_cota', 'layout_f1_2026', 'COTA F1 2026', 5428,
   '[{"number": 1, "name": "T1", "apex_m": 432.11}, {"number": 2, "name": "T2", "apex_m": 900}]', 1),
  ('fn_cota', 'layout_f1_2026_paddock', 'COTA F1 2026 (Paddock test)', 5428,
   '[{"number": 1, "name": "T1", "apex_m": 432.11}, {"number": 2, "name": "T2", "apex_m": 950}]', 1);
INSERT INTO corner (track_id, number, apex_s, radius_m)
SELECT t.id, n, n * 100, 50 FROM track t CROSS JOIN generate_series(1, 2) AS n;
INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri) VALUES
  ('me-hu', 1, 1, now(), 'human', '0.1.0', ''), ('ai-hu', 1, 2, now(), 'ai', '0.1.0', ''),
  ('me-cota', 1, 3, now(), 'human', '0.1.0', ''), ('ai-cota', 1, 4, now(), 'ai', '0.1.0', '');
INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by) VALUES
  (1, 'c1', 's1', '{"WING": 5}', 'agent');
INSERT INTO corner_pass (session_id, corner_id, seq, t_start, duration_s, valid, setup_hash, features)
SELECT s.id, c.id, seq, now() + seq * interval '1 second', CASE s.source WHEN 'ai' THEN 5.0 ELSE 5.2 END,
       true, 's1', '{"metrics": {}}'
FROM session s JOIN corner c ON c.track_id = s.track_id AND c.number = 1
CROSS JOIN generate_series(1, 3) AS seq;
`

func newVariantService(t *testing.T, schema string) *reference.Service {
	t.Helper()
	database := testdb.Open(t, schema)
	if _, err := database.ExecContext(t.Context(), variantsSQL); err != nil {
		t.Fatalf("load the fixture: %v", err)
	}
	return reference.NewService(database)
}

func TestSessionOnAnOriginalLayoutIsComparedWithTheAIOnItsTestCopy(t *testing.T) {
	service := newVariantService(t, "reference_variants")

	got, err := service.Session(t.Context(), 1, "")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	if got.Layout != "layout_f1_2026" || got.AiPasses != 3 || got.VariantNote != "" {
		t.Fatalf("layout %s, %d AI passes, note %q; want the copy's 3 passes", got.Layout, got.AiPasses, got.VariantNote)
	}
	if len(got.AiTracks) != 2 || got.AiTracks[0].Variant || got.AiTracks[0].AiPasses != 0 {
		t.Fatalf("AI tracks = %+v, want the own layout (no AI passes) first", got.AiTracks)
	}
	if copied := got.AiTracks[1]; !copied.Variant || copied.Name != "Hungaroring F1 2026 (Paddock test)" || copied.AiPasses != 3 {
		t.Errorf("variant = %+v", copied)
	}
	if len(got.Corners) != 1 || got.Corners[0].Match != "same" || *got.Corners[0].AiBestSessionId != 2 {
		t.Errorf("corners = %+v, want T1 against the AI's session 2 on the copy", got.Corners)
	}
}

func TestSessionIsNotComparedAcrossCornerMapsThatDiffer(t *testing.T) {
	service := newVariantService(t, "reference_variants_differ")

	got, err := service.Session(t.Context(), 3, "")
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	if got.AiPasses != 0 || len(got.AiTracks) != 1 || got.Corners[0].Match != "none" {
		t.Errorf("AI passes %d on %+v; the copy with another corner map must not count", got.AiPasses, got.AiTracks)
	}
	if !strings.Contains(got.VariantNote, "COTA F1 2026 (Paddock test) are not compared with: corner 2's apex is 50.0 m") {
		t.Errorf("note = %q", got.VariantNote)
	}
}

// The game's hotlap window asks GET /reference for its own layout. On an original layout it
// must get no AI times, even when the AI has driven the test copy (docs/adr/0027).
func TestForSetupNeverGroupsVariantsForTheGame(t *testing.T) {
	service := newVariantService(t, "reference_variants_game")
	tests := []struct {
		layout      string
		wantCorners int
	}{
		{layout: "layout_f1_2026", wantCorners: 0},
		{layout: "layout_f1_2026_paddock", wantCorners: 1},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			got, err := service.ForSetup(t.Context(), "rss_formula_hybrid_2026", "fn_hungaroring", tt.layout, "s1")
			if err != nil {
				t.Fatalf("ForSetup: %v", err)
			}
			if len(got.Corners) != tt.wantCorners {
				t.Errorf("%d AI corners on %s, want %d", len(got.Corners), tt.layout, tt.wantCorners)
			}
		})
	}
}
