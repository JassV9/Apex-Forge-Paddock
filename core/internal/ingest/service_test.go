package ingest

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const demoSpool = "../../../datasets/demo/spool"

// fakeStore keeps chunks in memory and remembers which files it has seen.
type fakeStore struct {
	chunks []Chunk
}

func (f *fakeStore) DoneChunks(_ context.Context, spoolID string) (map[string]bool, error) {
	done := map[string]bool{}
	for _, chunk := range f.chunks {
		if chunk.SpoolID == spoolID {
			done[chunk.FileName] = true
		}
	}
	return done, nil
}

func (f *fakeStore) StoreChunk(_ context.Context, chunk Chunk) (ChunkResult, error) {
	f.chunks = append(f.chunks, chunk)
	return ChunkResult{Inserted: len(chunk.Passes), Rejects: map[string]int{}}, nil
}

func (f *fakeStore) passesByCorner() map[int]int {
	counts := map[int]int{}
	for _, chunk := range f.chunks {
		for _, pass := range chunk.Passes {
			counts[pass.Corner]++
		}
	}
	return counts
}

type harness struct {
	service *Service
	store   *fakeStore
	reader  *sdkmetric.ManualReader
	dataDir string
}

func newHarness(t *testing.T, spoolDir string) *harness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")
	store := &fakeStore{}
	cfg := Config{SpoolDir: spoolDir, DataDir: t.TempDir(), PollInterval: time.Second}
	service, err := New(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil)), meter)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{service: service, store: store, reader: reader, dataDir: cfg.DataDir}
}

// counter sums one counter's data points that carry the given attribute value.
func (h *harness) counter(t *testing.T, name, key, value string) int64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var sum int64
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			data, ok := metric.Data.(metricdata.Sum[int64])
			if metric.Name != name || !ok {
				continue
			}
			for _, point := range data.DataPoints {
				if got, _ := point.Attributes.Value(attribute.Key(key)); got.AsString() == value {
					sum += point.Value
				}
			}
		}
	}
	return sum
}

func TestScanIngestsWhatTheAgentWroteAndWritesParquet(t *testing.T) {
	h := newHarness(t, sampleSpool)

	ingested, err := h.service.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}

	if ingested != 4 || len(h.store.chunks) != 4 {
		t.Fatalf("ingested %d files into %d chunks, want 4", ingested, len(h.store.chunks))
	}
	if h.store.chunks[0].Header == nil {
		t.Error("first chunk carries no session line")
	}
	wantSamples := 0
	for _, chunk := range h.store.chunks {
		for _, pass := range chunk.Passes {
			wantSamples += pass.Samples.N
		}
	}
	parts, _ := filepath.Glob(filepath.Join(h.dataDir, "sessions", "sample-session", "*.parquet"))
	gotSamples := 0
	for _, part := range parts {
		rows, err := parquet.ReadFile[sampleRow](part)
		if err != nil {
			t.Fatalf("read %s: %v", part, err)
		}
		gotSamples += len(rows)
	}
	if len(parts) != 3 || gotSamples != wantSamples || wantSamples == 0 {
		t.Errorf("%d parquet parts with %d rows, want 3 parts with %d rows", len(parts), gotSamples, wantSamples)
	}
	if got := h.counter(t, "paddock.ingest.lines", "outcome", "ingested"); got != 4 {
		t.Errorf("ingested lines metric = %d, want 4", got)
	}
}

func TestScanSkipsFilesAlreadyIngested(t *testing.T) {
	h := newHarness(t, sampleSpool)
	if _, err := h.service.ScanOnce(context.Background()); err != nil {
		t.Fatalf("first scan: %v", err)
	}

	again, err := h.service.ScanOnce(context.Background())

	if err != nil || again != 0 {
		t.Errorf("second scan ingested %d files (err=%v), want 0", again, err)
	}
}

func TestDemoDatasetReplaysToTheSamePassesAsV0(t *testing.T) {
	h := newHarness(t, demoSpool)

	if _, err := h.service.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}

	// Apex Forge v0 logged 136 passes in that session: 8 through each of COTA's 17 corners.
	counts := h.store.passesByCorner()
	if len(counts) != 17 {
		t.Fatalf("%d corners, want 17", len(counts))
	}
	for corner := 1; corner <= 17; corner++ {
		if counts[corner] != 8 {
			t.Errorf("corner %d has %d passes, want 8", corner, counts[corner])
		}
	}
	if parts, _ := filepath.Glob(filepath.Join(h.dataDir, "sessions", "*", "*.parquet")); len(parts) != 0 {
		t.Errorf("parquet written for data without samples: %v", parts)
	}
}

func TestScanCountsRejectedLinesByReason(t *testing.T) {
	spool := t.TempDir()
	lines := sampleLines(t)
	content := string(lines[0]) + "\n" + `{"kind":"pass","v":1}` + "\nnot json\n" +
		`{"kind":"pass","v":1,"session_id":"other","seq":1,"corner":1,"t_start_ms":1,"duration_s":1}` + "\n"
	writeChunk(t, spool, "sample-session", "000001.jsonl", content)
	h := newHarness(t, spool)

	if _, err := h.service.ScanOnce(context.Background()); err != nil {
		t.Fatalf("ScanOnce: %v", err)
	}

	chunk := h.store.chunks[0]
	if chunk.Lines != 4 || chunk.Header == nil || len(chunk.Passes) != 0 {
		t.Fatalf("chunk = %d lines, header %v, %d passes", chunk.Lines, chunk.Header != nil, len(chunk.Passes))
	}
	for reason, want := range map[string]int64{
		ReasonSchema: 1, ReasonInvalidJSON: 1, ReasonSessionMismatch: 1,
	} {
		if got := h.counter(t, "paddock.ingest.rejects", "reason", reason); got != want {
			t.Errorf("rejects{reason=%s} = %d, want %d", reason, got, want)
		}
	}
}

func TestScanWaitsForFilesThatMayStillBeWritten(t *testing.T) {
	spool := t.TempDir()
	writeChunk(t, spool, "sample-session", "000001.jsonl", string(sampleLines(t)[0])+"\n")
	h := newHarness(t, spool)
	h.service.cfg.SettleTime = time.Minute

	early, err := h.service.ScanOnce(context.Background())
	if err != nil || early != 0 {
		t.Fatalf("fresh file: ingested %d (err=%v), want 0", early, err)
	}
	h.service.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	later, err := h.service.ScanOnce(context.Background())

	if err != nil || later != 1 {
		t.Errorf("settled file: ingested %d (err=%v), want 1", later, err)
	}
}

func TestScanIgnoresAMissingSpoolAndStrayEntries(t *testing.T) {
	spool := t.TempDir()
	writeChunk(t, spool, "not a session id!", "000001.jsonl", "{}\n")
	if err := os.WriteFile(filepath.Join(spool, "notes.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{spool, filepath.Join(spool, "does-not-exist")} {
		h := newHarness(t, dir)
		if ingested, err := h.service.ScanOnce(context.Background()); err != nil || ingested != 0 {
			t.Errorf("%s: ingested %d (err=%v), want 0 and no error", dir, ingested, err)
		}
	}
}

func writeChunk(t *testing.T, spool, session, name, content string) {
	t.Helper()
	dir := filepath.Join(spool, session)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConvertSessionWritesTheSamePartsWithoutADatabase(t *testing.T) {
	out := t.TempDir()

	parts, err := ConvertSession(filepath.Join(sampleSpool, "sample-session"), out,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err != nil || parts != 3 {
		t.Fatalf("ConvertSession = %d parts, err=%v; want 3", parts, err)
	}
	rows, err := parquet.ReadFile[sampleRow](filepath.Join(out, "000002.parquet"))
	if err != nil || len(rows) == 0 || rows[0].Seq != 1 {
		t.Errorf("first part: %d rows, err=%v", len(rows), err)
	}
}
