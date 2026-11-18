package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// The sample's session line as a session that names its run context
// (docs/adr/0028-run-context-and-ai-provenance.md).
const sessionContext = `{"cars_count":1,"csp_version":"0.3.0-preview581","csp_build":3400,
"ai":{"tyre_hint_base":1.1,"learned_share":0.885,"pinned_share":0.93,"applied_hint":1.023,
"extra_grip":1,"brake_hint":0.97,"level":1,"aggression":0.5}}`

// edited is a sample line decoded, changed and encoded again.
func edited(t *testing.T, line []byte, change func(map[string]any)) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(line, &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func decoded(t *testing.T, text string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// aPinnedExperiment stores an AI experiment pinned at 0.93 with one block and returns its id.
func aPinnedExperiment(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	var id int64
	err := database.QueryRowContext(context.Background(), `
WITH s AS (SELECT car_id, track_id FROM session WHERE spool_id = 'sample-session'),
     h AS (INSERT INTO hypothesis (car_id, track_id, statement, source)
           SELECT car_id, track_id, 'test', 'rules' FROM s RETURNING id),
     v AS (SELECT id FROM setup_version ORDER BY id LIMIT 1),
     e AS (INSERT INTO experiment (hypothesis_id, kind, baseline_version_id, state, success_test,
                                   guardrails, driver, ai_hint_share)
           SELECT h.id, 'ofat', v.id, 'running', '{}', '[]', 'ai', 0.93 FROM h, v RETURNING id),
     a AS (INSERT INTO experiment_arm (experiment_id, setup_version_id, label_code, is_baseline)
           SELECT e.id, v.id, 'Setup K', TRUE FROM e, v RETURNING id, experiment_id)
INSERT INTO run_block (experiment_id, arm_id, "order", laps_target)
SELECT experiment_id, id, 1, 3 FROM a RETURNING experiment_id`).Scan(&id)
	if err != nil {
		t.Fatalf("store experiment: %v", err)
	}
	return id
}

// passWith is the sample's first pass as pass seq, in block 1 of the experiment when block, with
// the given context (none when empty).
func passWith(t *testing.T, seq int, experiment int64, block bool, passContext string) string {
	return edited(t, sampleLines(t)[3], func(pass map[string]any) {
		pass["seq"] = seq
		pass["t_start_ms"] = pass["t_start_ms"].(float64) + float64(seq)*1000
		if block {
			pass["block"] = map[string]any{"experiment_id": experiment, "order": 1, "lap": 1}
		}
		if passContext != "" {
			pass["context"] = decoded(t, passContext)
		}
	})
}

func TestTheRunContextOfASessionAndItsBlockIsStored(t *testing.T) {
	database := testdb.Open(t, "ingest_context")
	spool := t.TempDir()
	lines := sampleLines(t)
	session := edited(t, lines[0], func(s map[string]any) { s["context"] = decoded(t, sessionContext) })
	writeChunk(t, spool, "sample-session", "000001.jsonl",
		strings.Join([]string{session, string(lines[1]), string(lines[2])}, "\n")+"\n")
	scanInto(t, database, spool)
	experiment := aPinnedExperiment(t, database)

	ai := `{"clock":"physics","ai_hint":%v,"damage_max":1.5,"resets":2,"abs_share":0.25,"tc_share":0,"drs_share":0}`
	passes := []string{
		passWith(t, 21, experiment, true, fmt.Sprintf(ai, 1.023)),
		passWith(t, 22, experiment, true, fmt.Sprintf(ai, 0.98)),
		passWith(t, 23, experiment, true, ""),
		passWith(t, 24, experiment, false, `{"clock":"app","damage_max":0,"resets":0,"abs_share":0,"tc_share":0.5,"drs_share":1}`),
	}
	writeChunk(t, spool, "sample-session", "000002.jsonl", strings.Join(passes, "\n")+"\n")
	scanInto(t, database, spool)
	// a file read again stores nothing twice
	if _, err := database.ExecContext(context.Background(), `DELETE FROM spool_chunk`); err != nil {
		t.Fatal(err)
	}
	scanInto(t, database, spool)

	checkPassContexts(t, database)
	checkRunContexts(t, database, experiment)
}

func checkPassContexts(t *testing.T, database *sql.DB) {
	t.Helper()
	cases := []struct {
		seq  int
		want string
	}{
		{21, "1.023 1.5 2 0.25 0 0 physics"},
		{22, "0.98 1.5 2 0.25 0 0 physics"},
		{23, "<nil> <nil> <nil> <nil> <nil> <nil> <nil>"}, // a line without a context
		{24, "<nil> 0 0 0 0.5 1 app"},                     // a human's pass: no hint
	}
	for _, c := range cases {
		var got [7]sql.NullString
		err := database.QueryRowContext(context.Background(),
			`SELECT ai_hint::text, damage_max::text, reset_counter::text, abs_share::text,
			        tc_share::text, drs_share::text, clock FROM corner_pass WHERE seq = $1`, c.seq).
			Scan(&got[0], &got[1], &got[2], &got[3], &got[4], &got[5], &got[6])
		if err != nil {
			t.Fatalf("read pass %d: %v", c.seq, err)
		}
		if text := nullsText(got[:]); text != c.want {
			t.Errorf("pass %d context = %s, want %s", c.seq, text, c.want)
		}
	}
}

func nullsText(values []sql.NullString) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = "<nil>"
		if value.Valid {
			parts[i] = value.String
		}
	}
	return strings.Join(parts, " ")
}

func checkRunContexts(t *testing.T, database *sql.DB, experiment int64) {
	t.Helper()
	cases := []struct {
		name, query string
		want        string
	}{
		{"the session line", `SELECT concat_ws(' ', csp_version, csp_build, cars_count, ai_tyre_hint_base,
		    ai_learned_share, ai_pinned_share, ai_applied_hint, ai_extra_grip, coalesce(ai_aero_hint::text, '-'),
		    ai_brake_hint, ai_level, ai_aggression) FROM run_context WHERE block_id IS NULL`,
			"0.3.0-preview581 3400 1 1.1 0.885 0.93 1.023 1 - 0.97 1 0.5"},
		{"the block: pinned, the hints driven, the passes counted once", `SELECT concat_ws(' ',
		    ai_pinned_share, ai_hint_min, ai_hint_max, passes) FROM run_context c
		    JOIN run_block b ON b.id = c.block_id WHERE b.experiment_id = ` + strconv.FormatInt(experiment, 10),
			"0.93 0.98 1.023 3"},
		{"one row each", `SELECT count(*)::text FROM run_context`, "2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got string
			if err := database.QueryRowContext(context.Background(), c.query).Scan(&got); err != nil {
				t.Fatalf("query: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestASessionLineWithoutAContextStoresNone(t *testing.T) {
	database := testdb.Open(t, "ingest_context_none")

	scanInto(t, database, sampleSpool)

	if got := count(t, database, `SELECT count(*) FROM run_context`); got != 0 {
		t.Errorf("run_context has %d rows for a session recorded before the agent sent one", got)
	}
}
