package ingest

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"go.opentelemetry.io/otel/metric/noop"
)

// ConvertSession writes the Parquet parts of one spool session directory into outDir, exactly
// as ingest would, without a database. It returns the number of parts written. It is used to
// build test fixtures for the engine (cmd/spool2parquet).
func ConvertSession(sessionDir, outDir string, logger *slog.Logger) (int, error) {
	service, err := New(Config{}, nil, logger, noop.NewMeterProvider().Meter("convert"))
	if err != nil {
		return 0, err
	}
	files, err := filepath.Glob(filepath.Join(sessionDir, "*.jsonl"))
	if err != nil {
		return 0, fmt.Errorf("list spool files: %w", err)
	}
	sort.Strings(files)
	parts := 0
	for _, path := range files {
		chunk, err := service.readChunk(filepath.Base(sessionDir), path)
		if err != nil {
			return parts, err
		}
		if total(chunk.Rejects) > 0 {
			return parts, fmt.Errorf("%s: %d lines rejected", chunk.FileName, total(chunk.Rejects))
		}
		rows, err := writeParquet(filepath.Join(outDir, parquetPartName(chunk.FileName)), chunk.Passes)
		if err != nil {
			return parts, err
		}
		if rows > 0 {
			parts++
		}
	}
	return parts, nil
}
