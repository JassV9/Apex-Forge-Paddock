package ingest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSpool = "testdata/spool"

// sampleLines returns every line the real agent wrote in testdata (see
// agent/tests/export_spool_sample.py): one session line, then three passes.
func sampleLines(t *testing.T) [][]byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(sampleSpool, "sample-session", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no sample spool files (err=%v)", err)
	}
	var lines [][]byte
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // fixed test data
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for line := range bytes.SplitSeq(bytes.TrimSpace(raw), []byte("\n")) {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDecodeAcceptsWhatTheAgentWrites(t *testing.T) {
	decoder, err := NewDecoder()
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	lines := sampleLines(t)
	if len(lines) != 4 {
		t.Fatalf("sample has %d lines, want 4", len(lines))
	}

	session, err := decoder.Decode(lines[0])
	if err != nil {
		t.Fatalf("session line rejected: %v", err)
	}
	if session.Session == nil || len(session.Session.Track.Corners) != 7 {
		t.Fatalf("session line decoded as %+v", session)
	}
	for index, raw := range lines[1:] {
		line, err := decoder.Decode(raw)
		if err != nil {
			t.Fatalf("pass line %d rejected: %v", index+1, err)
		}
		pass := line.Pass
		if pass == nil || pass.Seq != index+1 || pass.Conditions == nil || pass.Samples == nil {
			t.Fatalf("pass line %d decoded as %+v", index+1, pass)
		}
		if got := len(pass.Samples.Columns.SaFl); got != pass.Samples.N {
			t.Errorf("pass %d: %d slip samples, n is %d", pass.Seq, got, pass.Samples.N)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	decoder, err := NewDecoder()
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	pass := string(sampleLines(t)[1])
	tests := []struct {
		name       string
		line       string
		wantReason string
	}{
		{name: "not json", line: `{"kind":"pass"`, wantReason: ReasonInvalidJSON},
		{name: "unknown kind", line: `{"kind":"lap"}`, wantReason: ReasonUnknownKind},
		{name: "no kind", line: `{}`, wantReason: ReasonUnknownKind},
		{name: "missing fields", line: `{"kind":"pass","v":1}`, wantReason: ReasonSchema},
		{
			name:       "sequence below one",
			line:       strings.Replace(pass, `"seq":1,`, `"seq":0,`, 1),
			wantReason: ReasonSchema,
		},
		{
			name:       "field outside the contract",
			line:       strings.Replace(pass, `"seq":1,`, `"seq":1,"verdict":"better",`, 1),
			wantReason: ReasonSchema,
		},
		{
			name:       "newer format version",
			line:       strings.Replace(pass, `"v":1,`, `"v":2,`, 1),
			wantReason: ReasonSchema,
		},
		{
			name:       "column length differs from n",
			line:       shortenColumn(t, pass, "rh_r"),
			wantReason: ReasonColumnLength,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decoder.Decode([]byte(tt.line))
			var rejected *RejectError
			if !errors.As(err, &rejected) {
				t.Fatalf("error = %v, want a *RejectError", err)
			}
			if rejected.Reason != tt.wantReason {
				t.Errorf("reason = %q (%s), want %q", rejected.Reason, rejected.Detail, tt.wantReason)
			}
		})
	}
}

// shortenColumn drops the last value of one column, leaving valid JSON.
func shortenColumn(t *testing.T, pass, column string) string {
	t.Helper()
	start := strings.Index(pass, `"`+column+`":[`)
	end := start + strings.Index(pass[start:], "]")
	lastComma := strings.LastIndex(pass[start:end], ",")
	if start < 0 || lastComma < 0 {
		t.Fatalf("column %s not found in the sample pass", column)
	}
	return pass[:start+lastComma] + pass[end:]
}
