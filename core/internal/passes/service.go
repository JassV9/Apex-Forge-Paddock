// Package passes is the read path for what a session recorded: its stored corner passes with
// the zone times per corner, and the raw samples of one pass from the session's Parquet
// files. It never writes: ingest owns the rows and the files.
package passes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// ErrNotFound means the session or the pass does not exist, or the pass has no samples.
var ErrNotFound = errors.New("not found")

// Limits of the max_points a caller may ask for (docs/api/openapi.yaml).
const (
	MinPoints     = 50
	MaxPoints     = 5000
	DefaultPoints = 1000
)

// Service answers the passes routes from PostgreSQL and the Parquet files under dataDir.
type Service struct {
	database *sql.DB
	dataDir  string
}

// NewService reads passes from database and samples from <dataDir>/sessions/<spool id>/,
// where ingest writes them.
func NewService(database *sql.DB, dataDir string) *Service {
	return &Service{database: database, dataDir: dataDir}
}

// Passes returns every pass of the session and the zone times of each corner.
func (s *Service) Passes(ctx context.Context, sessionID int64) (contracts.SessionPasses, error) {
	found, err := s.session(ctx, sessionID)
	if err != nil {
		return contracts.SessionPasses{}, err
	}
	records, err := s.records(ctx, sessionID)
	if err != nil {
		return contracts.SessionPasses{}, err
	}
	names, err := s.cornerNames(ctx, sessionID)
	if err != nil {
		return contracts.SessionPasses{}, err
	}
	return contracts.SessionPasses{
		SessionId:    int(sessionID),
		TrackLengthM: found.trackLengthM,
		Corners:      Summarize(records, names),
		Passes:       records,
	}, nil
}

// Trace returns the samples of pass seq, thinned to at most maxPoints per column.
func (s *Service) Trace(ctx context.Context, sessionID int64, seq, maxPoints int) (contracts.PassTrace, error) {
	found, err := s.session(ctx, sessionID)
	if err != nil {
		return contracts.PassTrace{}, err
	}
	corner, err := s.passCorner(ctx, sessionID, seq)
	if err != nil {
		return contracts.PassTrace{}, err
	}
	dir, ok := s.sessionDir(found.spoolID)
	if !ok {
		return contracts.PassTrace{}, fmt.Errorf("session %d has no data directory: %w", sessionID, ErrNotFound)
	}
	rows, err := readPass(dir, seq)
	if err != nil {
		return contracts.PassTrace{}, fmt.Errorf("session %d pass %d: %w", sessionID, seq, err)
	}
	if len(rows) == 0 {
		return contracts.PassTrace{}, fmt.Errorf("session %d pass %d has no stored samples: %w", sessionID, seq, ErrNotFound)
	}
	trace := buildTrace(rows, maxPoints)
	trace.SessionId, trace.Seq, trace.Corner = int(sessionID), seq, corner
	return trace, nil
}

// sessionDir is where ingest writes a session's Parquet parts (ingest.Config.DataDir). A spool
// id that is not one plain directory name has none, so nothing outside it is ever read.
func (s *Service) sessionDir(spoolID string) (string, bool) {
	if spoolID == "." || spoolID == ".." || filepath.Base(spoolID) != spoolID {
		return "", false
	}
	return filepath.Join(s.dataDir, "sessions", spoolID), true
}
