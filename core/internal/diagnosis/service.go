// Package diagnosis asks the engine what a session's passes say about the car, and keeps
// every answer. Core adds nothing to a diagnosis: the numbers, the hypotheses and the words
// are the engine's (docs/adr/0010-diagnosis-and-explain.md).
package diagnosis

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
)

// Engine writes diagnoses. *engine.Client implements it.
type Engine interface {
	Diagnose(ctx context.Context, request contracts.DiagnoseRequest) (contracts.Diagnosis, error)
}

// Setups knows the setup an experiment on a session would start from.
// *experiments.Service implements it.
type Setups interface {
	DiagnoseItems(ctx context.Context, sessionID int64) (int64, []contracts.DiagnoseItem, error)
}

// Service runs and reads diagnoses. Its errors are the ones the experiments use, so the API
// answers both the same way.
type Service struct {
	database *sql.DB
	engine   Engine
	setups   Setups
	logger   *slog.Logger
}

// NewService wires the service to its database, the engine and the setups.
func NewService(database *sql.DB, engine Engine, setups Setups, logger *slog.Logger) *Service {
	return &Service{database: database, engine: engine, setups: setups, logger: logger}
}

// Run diagnoses the session now and stores the answer. With setup data only the passes
// driven on the setup an experiment would start from count: a diagnosis that mixed a
// baseline with a rejected variant would describe neither car.
func (s *Service) Run(ctx context.Context, sessionID int64) (contracts.Diagnosis, error) {
	if err := s.mustBeReady(ctx, sessionID); err != nil {
		return contracts.Diagnosis{}, err
	}
	versionID, items, err := s.setups.DiagnoseItems(ctx, sessionID)
	if err != nil {
		return contracts.Diagnosis{}, err
	}
	passes, err := readPasses(ctx, s.database, sessionID, versionID)
	if err != nil {
		return contracts.Diagnosis{}, err
	}
	if len(passes) == 0 {
		return contracts.Diagnosis{}, fmt.Errorf(
			"session %d has no valid pass with computed features on its baseline setup yet: %w",
			sessionID, experiments.ErrConflict)
	}
	diagnosis, err := s.engine.Diagnose(ctx, contracts.DiagnoseRequest{Passes: passes, Items: items})
	if err != nil {
		return contracts.Diagnosis{}, fmt.Errorf("diagnose session %d: %w", sessionID, err)
	}
	if err := s.save(ctx, sessionID, diagnosis); err != nil {
		return contracts.Diagnosis{}, err
	}
	s.logger.InfoContext(ctx, "session diagnosed", "session_id", sessionID, "passes", len(passes),
		"hypotheses", len(diagnosis.Hypotheses), "provider", string(diagnosis.Explanation.Provider),
		"reply_rejected", diagnosis.Explanation.Rejected != "")
	return diagnosis, nil
}

// Latest returns the newest stored diagnosis of the session.
func (s *Service) Latest(ctx context.Context, sessionID int64) (contracts.Diagnosis, error) {
	var body []byte
	err := s.database.QueryRowContext(ctx,
		`SELECT body FROM diagnosis WHERE session_id = $1 ORDER BY id DESC LIMIT 1`, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Diagnosis{}, fmt.Errorf("session %d has no diagnosis: %w", sessionID, experiments.ErrNotFound)
	}
	if err != nil {
		return contracts.Diagnosis{}, fmt.Errorf("read diagnosis of session %d: %w", sessionID, err)
	}
	var diagnosis contracts.Diagnosis
	if err := json.Unmarshal(body, &diagnosis); err != nil {
		return contracts.Diagnosis{}, fmt.Errorf("decode diagnosis of session %d: %w", sessionID, err)
	}
	return diagnosis, nil
}

// A diagnosis of half a session would be stored as if it were the whole one, so the
// session must exist and have no pass whose features are still being computed.
const readySQL = `
SELECT EXISTS (SELECT 1 FROM session WHERE id = $1),
       EXISTS (SELECT 1 FROM jobs j JOIN session s ON s.spool_id = j.payload->>'spool_id'
               WHERE s.id = $1 AND j.kind = $2 AND j.state IN ('queued', 'running'))`

func (s *Service) mustBeReady(ctx context.Context, sessionID int64) error {
	var found, computing bool
	err := s.database.QueryRowContext(ctx, readySQL, sessionID, ingest.JobFeatures).Scan(&found, &computing)
	if err != nil {
		return fmt.Errorf("find session %d: %w", sessionID, err)
	}
	if !found {
		return fmt.Errorf("session %d: %w", sessionID, experiments.ErrNotFound)
	}
	if computing {
		return fmt.Errorf("the features of session %d are still being computed; ask again in a moment: %w",
			sessionID, experiments.ErrConflict)
	}
	return nil
}

func (s *Service) save(ctx context.Context, sessionID int64, diagnosis contracts.Diagnosis) error {
	body, err := json.Marshal(diagnosis)
	if err != nil {
		return fmt.Errorf("encode diagnosis of session %d: %w", sessionID, err)
	}
	_, err = s.database.ExecContext(ctx,
		`INSERT INTO diagnosis (session_id, body) VALUES ($1, $2)`, sessionID, string(body))
	if err != nil {
		return fmt.Errorf("save diagnosis of session %d: %w", sessionID, err)
	}
	return nil
}

// Out-laps (block lap 0) are left out, as they are when an experiment is judged. versionID 0
// means the session has no setup data: then every valid pass counts.
const passesSQL = `
SELECT c.number, p.features->>'speed_class', p.features->'metrics'
FROM corner_pass p JOIN corner c ON c.id = p.corner_id
WHERE p.session_id = $1 AND p.valid
  AND p.features->'metrics' IS NOT NULL
  AND p.features->>'speed_class' IN ('slow', 'medium', 'fast')
  AND COALESCE(p.block_lap, 1) > 0
  AND ($2 = 0 OR p.setup_version_id = $2)
ORDER BY p.seq`

func readPasses(ctx context.Context, database *sql.DB, sessionID, versionID int64) ([]contracts.DiagnosePass, error) {
	rows, err := database.QueryContext(ctx, passesSQL, sessionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("query passes of session %d: %w", sessionID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	passes := []contracts.DiagnosePass{}
	for rows.Next() {
		var pass contracts.DiagnosePass
		var metrics []byte
		if err := rows.Scan(&pass.Corner, &pass.SpeedClass, &metrics); err != nil {
			return nil, fmt.Errorf("scan pass of session %d: %w", sessionID, err)
		}
		if err := json.Unmarshal(metrics, &pass.Metrics); err != nil {
			return nil, fmt.Errorf("decode metrics of a pass of session %d: %w", sessionID, err)
		}
		passes = append(passes, pass)
	}
	return passes, rows.Err()
}
