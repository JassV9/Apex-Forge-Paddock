package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// withBattery is the sample's first pass as a later pass (seq, start time) of a car that has
// a hybrid battery, or of one that has none when ers is nil.
func withBattery(t *testing.T, seq int, ers map[string]float64) string {
	t.Helper()
	var pass map[string]any
	if err := json.Unmarshal([]byte(samplePass(t)), &pass); err != nil {
		t.Fatal(err)
	}
	pass["seq"] = seq
	pass["t_start_ms"] = pass["t_start_ms"].(float64) + float64(seq)*1000
	if ers != nil {
		pass["ers"] = ers
	}
	line, err := json.Marshal(pass)
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}

func TestTheBatteryOfAPassIsStoredOnlyWhenTheCarHasOne(t *testing.T) {
	cases := []struct {
		name string
		seq  int
		ers  map[string]float64
		want [3]sql.NullFloat64
	}{
		{"hybrid", 11, map[string]float64{"charge_start": 0.82, "charge_end": 0.64, "deploy_mean": 0.37},
			[3]sql.NullFloat64{{Float64: 0.82, Valid: true}, {Float64: 0.64, Valid: true}, {Float64: 0.37, Valid: true}}},
		{"empty battery", 12, map[string]float64{"charge_start": 0, "charge_end": 0, "deploy_mean": 0},
			[3]sql.NullFloat64{{Valid: true}, {Valid: true}, {Valid: true}}},
		{"no battery", 13, nil, [3]sql.NullFloat64{}},
	}
	database := testdb.Open(t, "ingest_ers")
	spool := t.TempDir()
	lines := sampleLines(t)
	content := []string{string(lines[0]), string(lines[1]), string(lines[2])}
	for _, c := range cases {
		content = append(content, withBattery(t, c.seq, c.ers))
	}
	writeChunk(t, spool, "sample-session", "000001.jsonl", strings.Join(content, "\n")+"\n")

	scanInto(t, database, spool)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got [3]sql.NullFloat64
			err := database.QueryRowContext(context.Background(),
				`SELECT ers_charge_start, ers_charge_end, ers_deploy_mean FROM corner_pass WHERE seq = $1`,
				c.seq).Scan(&got[0], &got[1], &got[2])
			if err != nil {
				t.Fatalf("read pass %d: %v", c.seq, err)
			}
			if got != c.want {
				t.Errorf("battery = %+v, want %+v", got, c.want)
			}
		})
	}
}
