package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/testdb"
)

// sessionLine is the sample session's first line with another id, driver and layout, the way
// the agent starts a spool session (source "human" or "ai", the layout the game reports).
func sessionLine(t *testing.T, id, source, layout string) string {
	t.Helper()
	first, err := os.ReadFile(filepath.Join(sampleSpool, "sample-session", "000001.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line, _, _ := strings.Cut(string(first), "\n")
	line = strings.Replace(line, `"session_id":"sample-session"`, `"session_id":"`+id+`"`, 1)
	line = strings.Replace(line, `"source":"human"`, `"source":"`+source+`"`, 1)
	return strings.Replace(line, `"layout":""`, `"layout":"`+layout+`"`, 1)
}

// The driver is compared with the AI by track row (docs/adr/0019): a person's session and the
// AI's on the same layout must share one, and a session on the original layout must not.
func TestADriversSessionSharesTheAIsTrackOnlyOnTheSameLayout(t *testing.T) {
	database := testdb.Open(t, "ingest_layouts")
	spool := t.TempDir()
	sessions := []struct{ id, source, layout string }{
		{"ai-copy", "ai", "layout_f1_paddock"},
		{"human-copy", "human", "layout_f1_paddock"},
		{"human-original", "human", "layout_f1"},
	}
	for _, s := range sessions {
		writeChunk(t, spool, s.id, "000001.jsonl", sessionLine(t, s.id, s.source, s.layout)+"\n")
	}

	scanInto(t, database, spool)

	track := func(spoolID string) int {
		return count(t, database, `SELECT track_id FROM session WHERE spool_id = '`+spoolID+`'`)
	}
	if track("ai-copy") != track("human-copy") {
		t.Errorf("the AI's session is on track %d and the driver's on %d, on the same layout",
			track("ai-copy"), track("human-copy"))
	}
	if track("human-original") == track("human-copy") {
		t.Errorf("a session on the original layout shares track %d with the test copy", track("human-copy"))
	}
	if got := count(t, database, `SELECT count(*) FROM track`); got != 2 {
		t.Errorf("%d track rows, want one per layout (2)", got)
	}
}
