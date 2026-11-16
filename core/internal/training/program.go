// Package training runs AI training programs (docs/adr/0020-ai-training-program.md): AC's AI
// drives a car's top-ranked experiments one after another, each is judged when its laps are
// in, and the next is taken from the ranking that the verdict changed. It only calls the
// experiment service's own actions, so every rule of the ledger (one experiment on a car at a
// time, the champion, the success test fixed before driving) holds for it as for a human.
package training

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Ledger is the experiment actions a program uses. *experiments.Service implements it.
type Ledger interface {
	Get(ctx context.Context, id int64) (contracts.Experiment, error)
	Create(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error)
	Plan(ctx context.Context, id int64) (contracts.Experiment, error)
	Start(ctx context.Context, id int64) (contracts.Experiment, error)
	Evaluate(ctx context.Context, id int64) (contracts.Experiment, error)
	Extend(ctx context.Context, id int64) (contracts.Experiment, error)
	// RevertByTraining puts the baseline back with the program's reason in the ledger, never the
	// driver's: the ledger says who decided.
	RevertByTraining(ctx context.Context, id int64, reason string) (contracts.Experiment, error)
}

// Ranker ranks the experiments a session's diagnosis names. *sensitivity.Service implements it.
type Ranker interface {
	Proposals(ctx context.Context, sessionID int64) (contracts.Proposals, error)
}

// Diagnoser diagnoses a session. *diagnosis.Service implements it.
type Diagnoser interface {
	Run(ctx context.Context, sessionID int64) (contracts.Diagnosis, error)
}

// Service starts, advances and stops training programs.
type Service struct {
	database  *sql.DB
	ledger    Ledger
	ranker    Ranker
	diagnoser Diagnoser
	logger    *slog.Logger
}

// NewService wires programs to the database and the services they drive.
func NewService(database *sql.DB, ledger Ledger, ranker Ranker, diagnoser Diagnoser, logger *slog.Logger) *Service {
	return &Service{database: database, ledger: ledger, ranker: ranker, diagnoser: diagnoser, logger: logger}
}

// The open experiment on a car, if any: the one whose setups are on it.
const occupyingSQL = `
SELECT e.id, e.driver FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
WHERE h.car_id = $1 AND e.state IN ('running', 'evaluating', 'tradeoff', 'inconclusive')
ORDER BY e.id DESC LIMIT 1`

// Start opens a program on the session's car and track and starts its first experiment. The
// session is diagnosed afresh: the program ranks from what its passes say now. An open AI
// experiment on the car is taken over as the first step; a human's is left alone.
func (s *Service) Start(ctx context.Context, sessionID int64, request contracts.TrainingCreate) (contracts.TrainingProgram, error) {
	var carID, trackID int64
	err := s.database.QueryRowContext(ctx, `SELECT car_id, track_id FROM session WHERE id = $1`, sessionID).
		Scan(&carID, &trackID)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.TrainingProgram{}, fmt.Errorf("session %d: %w", sessionID, experiments.ErrNotFound)
	}
	if err != nil {
		return contracts.TrainingProgram{}, fmt.Errorf("read session %d: %w", sessionID, err)
	}
	adopt, err := s.adoptable(ctx, carID)
	if err != nil {
		return contracts.TrainingProgram{}, err
	}
	if _, err := s.diagnoser.Run(ctx, sessionID); err != nil {
		return contracts.TrainingProgram{}, fmt.Errorf("diagnose session %d for training: %w", sessionID, err)
	}
	id, err := s.insert(ctx, sessionID, carID, trackID, request.Experiments, adopt)
	if err != nil {
		return contracts.TrainingProgram{}, err
	}
	s.logger.InfoContext(ctx, "training started", "program_id", id, "session_id", sessionID,
		"target", request.Experiments, "adopted_experiment_id", adopt)
	if err := s.advance(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "training could not take its first step", "program_id", id, "error", err)
	}
	return s.Get(ctx, id)
}

// adoptable returns the open AI experiment on the car (0 if none) and refuses when a human's
// experiment is on it.
func (s *Service) adoptable(ctx context.Context, carID int64) (int64, error) {
	var id int64
	var driver string
	err := s.database.QueryRowContext(ctx, occupyingSQL, carID).Scan(&id, &driver)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find the experiment on car %d: %w", carID, err)
	}
	if driver != string(contracts.ExperimentDriverAi) {
		return 0, fmt.Errorf("experiment %d is on the car with you driving; finish or abort it first: %w",
			id, experiments.ErrConflict)
	}
	return id, nil
}

func (s *Service) insert(ctx context.Context, sessionID, carID, trackID int64, target int, adopt int64) (int64, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit
	var id int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO training_program (session_id, car_id, track_id, target) VALUES ($1, $2, $3, $4) RETURNING id`,
		sessionID, carID, trackID, target).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "training_program_one_open_per_car") {
		return 0, fmt.Errorf("a training program is already open on this car; stop it first: %w", experiments.ErrConflict)
	}
	if err != nil {
		return 0, fmt.Errorf("store training program: %w", err)
	}
	if adopt != 0 {
		if err := addStep(ctx, tx, id, adopt); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

func addStep(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, programID, experimentID int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO training_step (program_id, experiment_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		programID, experimentID)
	if err != nil {
		return fmt.Errorf("add experiment %d to program %d: %w", experimentID, programID, err)
	}
	return nil
}

// Stop ends a program. The experiment it started is left as it is: the driver can let it
// finish, judge it or abort it.
func (s *Service) Stop(ctx context.Context, id int64) (contracts.TrainingProgram, error) {
	result, err := s.database.ExecContext(ctx,
		`UPDATE training_program SET state = 'stopped', message = 'Stopped by the driver.', updated_at = now()
		 WHERE id = $1 AND state IN ('running', 'waiting')`, id)
	if err != nil {
		return contracts.TrainingProgram{}, fmt.Errorf("stop program %d: %w", id, err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return contracts.TrainingProgram{}, err
		}
		return contracts.TrainingProgram{}, fmt.Errorf("program %d has already ended: %w", id, experiments.ErrConflict)
	}
	s.logger.InfoContext(ctx, "training stopped", "program_id", id)
	return s.Get(ctx, id)
}
