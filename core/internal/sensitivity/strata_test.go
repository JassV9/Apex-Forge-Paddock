package sensitivity

import (
	"database/sql"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

func TestATestCopyIsTheSameTrackAsItsLayout(t *testing.T) {
	tests := []struct {
		acID, layout, name, wantKey, wantName string
	}{
		{"fn_hungaroring", "layout_f1_2026_paddock", "Hungaroring F1 2026 (Paddock test)",
			"fn_hungaroring/layout_f1_2026", "Hungaroring F1 2026"},
		{"fn_hungaroring", "layout_f1_2026", "Hungaroring F1 2026", "fn_hungaroring/layout_f1_2026", "Hungaroring F1 2026"},
		{"ks_zandvoort", "", "Zandvoort", "ks_zandvoort/", "Zandvoort"},
		{"odd", "_paddock", "Odd", "odd/_paddock", "Odd"},
	}
	for _, tt := range tests {
		key, name := trackKey(tt.acID, tt.layout, tt.name)
		if key != tt.wantKey || name != tt.wantName {
			t.Errorf("trackKey(%q, %q) = %q, %q", tt.acID, tt.layout, key, name)
		}
	}
}

func TestTheHintBucketComesFromThePinElseTheRunContext(t *testing.T) {
	some := func(v float64) sql.NullFloat64 { return sql.NullFloat64{Float64: v, Valid: true} }
	none := sql.NullFloat64{}
	tests := []struct {
		name, driver      string
		pinned, low, high sql.NullFloat64
		want              string
	}{
		{"a person drives", "human", some(0.93), none, none, "none"},
		{"pinned", "ai", some(0.9409), some(0.80), some(0.99), "0.94"},
		{"learned, one share", "ai", none, some(0.8848), some(0.8852), "0.89"},
		{"learned, several shares", "ai", none, some(0.885), some(0.94), "mixed 0.89-0.94"},
		{"before run context", "ai", none, none, none, "unknown"},
	}
	for _, tt := range tests {
		if got := hintBucket(tt.driver, tt.pinned, tt.low, tt.high); got != tt.want {
			t.Errorf("%s: hintBucket = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOnlyTheSessionsTrackAndDriverAreKept(t *testing.T) {
	effects := []contracts.SensitivityEffect{
		{Parameter: "A", Track: "t/1", Driver: contracts.SensitivityEffectDriverAi},
		{Parameter: "B", Track: "t/1", Driver: contracts.SensitivityEffectDriverHuman},
		{Parameter: "C", Track: "all", Driver: contracts.SensitivityEffectDriverAi},
		{Parameter: "D", Track: "t/2", Driver: contracts.SensitivityEffectDriverAi},
	}

	names := func(rows []contracts.SensitivityEffect) string {
		out := ""
		for _, row := range rows {
			out += row.Parameter
		}
		return out
	}

	if got := names(onTrack(effects, "t/1", "ai")); got != "A" {
		t.Errorf("ai on t/1 = %s", got)
	}
	if got := names(onTrack(effects, "t/1", "")); got != "AB" {
		t.Errorf("anyone on t/1 = %s", got)
	}
}

func TestAnExperimentsStratumIsReadFromTheLedger(t *testing.T) {
	f := newFixture(t, "sensitivity_stratum")
	id := f.finished(t, entry{parameter: "ARB_FRONT", clicks: -1}, -0.03)
	f.exec(t, `UPDATE experiment SET driver = 'ai', ai_hint_share = 0.93 WHERE id = $1`, id)

	judged, err := readJudged(t.Context(), f.database, 0.9)

	if err != nil || len(judged) != 1 {
		t.Fatalf("readJudged = %+v, %v", judged, err)
	}
	s := judged[0].stratum
	if s.driver != "ai" || s.hint != "0.93" || s.clickStep == nil || *s.clickStep <= 0 || s.track == "" {
		t.Errorf("stratum = %+v", s)
	}
}
