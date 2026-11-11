package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"go.opentelemetry.io/otel/metric"
)

// Config says where the spool is and where Parquet files go.
type Config struct {
	SpoolDir     string        // <spool>/<session-id>/<nnnnnn>.jsonl, written by the agent
	DataDir      string        // Parquet parts go to <data>/sessions/<session-id>/
	PollInterval time.Duration // how often the spool is scanned
	SettleTime   time.Duration // a file younger than this may still be being written
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Service tails the spool directory.
type Service struct {
	cfg     Config
	store   Store
	decoder *Decoder
	logger  *slog.Logger
	metrics *metrics
	now     func() time.Time
}

// New builds the service; it does not start scanning.
func New(cfg Config, store Store, logger *slog.Logger, meter metric.Meter) (*Service, error) {
	decoder, err := NewDecoder()
	if err != nil {
		return nil, err
	}
	instruments, err := newMetrics(meter)
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg: cfg, store: store, decoder: decoder, logger: logger,
		metrics: instruments, now: time.Now,
	}, nil
}

// Run scans the spool until ctx is cancelled. A failed scan is logged and retried.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := s.ScanOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("spool scan failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// ScanOnce ingests every spool file that is complete and not yet ingested, in file-name order
// within each session, and returns how many files it ingested.
func (s *Service) ScanOnce(ctx context.Context) (int, error) {
	sessions, err := os.ReadDir(s.cfg.SpoolDir)
	if os.IsNotExist(err) {
		return 0, nil // the agent has not written anything yet
	}
	if err != nil {
		return 0, fmt.Errorf("list spool directory: %w", err)
	}
	ingested := 0
	for _, entry := range sessions {
		if !entry.IsDir() || !sessionIDPattern.MatchString(entry.Name()) {
			continue
		}
		count, err := s.scanSession(ctx, entry.Name())
		ingested += count
		if err != nil {
			return ingested, fmt.Errorf("session %s: %w", entry.Name(), err)
		}
	}
	return ingested, nil
}

func (s *Service) scanSession(ctx context.Context, spoolID string) (int, error) {
	files, err := filepath.Glob(filepath.Join(s.cfg.SpoolDir, spoolID, "*.jsonl"))
	if err != nil {
		return 0, fmt.Errorf("list spool files: %w", err)
	}
	if len(files) == 0 {
		return 0, nil
	}
	done, err := s.store.DoneChunks(ctx, spoolID)
	if err != nil {
		return 0, fmt.Errorf("look up ingested files: %w", err)
	}
	sort.Strings(files)
	ingested := 0
	for _, path := range files {
		if done[filepath.Base(path)] {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return ingested, fmt.Errorf("stat spool file: %w", err)
		}
		if s.now().Sub(info.ModTime()) < s.cfg.SettleTime {
			break // keep file order: later files wait for this one
		}
		if err := s.ingestFile(ctx, spoolID, path, info.ModTime()); err != nil {
			return ingested, err
		}
		ingested++
	}
	return ingested, nil
}

func (s *Service) ingestFile(ctx context.Context, spoolID, path string, written time.Time) error {
	chunk, err := s.readChunk(spoolID, path)
	if err != nil {
		return err
	}
	part := filepath.Join(chunk.ParquetURI, parquetPartName(chunk.FileName))
	samples, err := writeParquet(part, chunk.Passes)
	if err != nil {
		return err
	}
	result, err := s.store.StoreChunk(ctx, chunk)
	if err != nil {
		return fmt.Errorf("store %s: %w", chunk.FileName, err)
	}
	s.metrics.record(ctx, chunk, result, s.now().Sub(written))
	s.logger.Info("spool file ingested", "session_id", spoolID, "file", chunk.FileName,
		"lines", chunk.Lines, "passes_inserted", result.Inserted,
		"duplicates", result.Duplicates, "samples", samples,
		"rejected", total(chunk.Rejects)+total(result.Rejects))
	return nil
}

func total(counts map[string]int) int {
	sum := 0
	for _, count := range counts {
		sum += count
	}
	return sum
}
