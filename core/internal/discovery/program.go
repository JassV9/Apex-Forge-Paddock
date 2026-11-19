// Package discovery runs sensitivity discovery sweeps
// (docs/adr/0032-sensitivity-discovery-sweeps.md): AC's AI drives one setting of one car through
// a list of steps, each its own A-B-A-B experiment against the same baseline at a pinned AI hint,
// and when every step is measured the engine classifies how each observable responds. A sweep
// learns how the car responds; it never changes the setup. It only calls the experiment service's
// own actions, as a training program does, and its steps are experiments with purpose "sweep":
// the ledger puts the baseline back after each, whatever the verdict.
package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Ledger is the experiment actions a sweep uses. *experiments.Service implements it.
type Ledger interface {
	Get(ctx context.Context, id int64) (contracts.Experiment, error)
	CreateSweepStep(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error)
	Plan(ctx context.Context, id int64) (contracts.Experiment, error)
	Start(ctx context.Context, id int64) (contracts.Experiment, error)
	Evaluate(ctx context.Context, id int64) (contracts.Experiment, error)
	Extend(ctx context.Context, id int64) (contracts.Experiment, error)
	// RevertByTraining puts the baseline back with the sweep's reason; the ledger records that a
	// program decided, not the driver.
	RevertByTraining(ctx context.Context, id int64, reason string) (contracts.Experiment, error)
	SetupItems(ctx context.Context, sessionID int64) ([]contracts.SetupItem, error)
	BaselineVersion(ctx context.Context, sessionID int64) (int64, error)
	ResponseRequest(ctx context.Context, id int64) (contracts.ResponseRequest, error)
}

// Engine is the analysis a sweep asks for. *engine.Client implements it.
type Engine interface {
	Response(ctx context.Context, request contracts.ResponseRequest) (contracts.ResponseResponse, error)
	Curve(ctx context.Context, request contracts.DiscoveryCurveRequest) (contracts.DiscoveryCurve, error)
}

// Service starts, advances and stops sweeps.
type Service struct {
	database *sql.DB
	ledger   Ledger
	engine   Engine
	logger   *slog.Logger
}

// NewService wires sweeps to the database, the ledger and the engine.
func NewService(database *sql.DB, ledger Ledger, engine Engine, logger *slog.Logger) *Service {
	return &Service{database: database, ledger: ledger, engine: engine, logger: logger}
}

// draft is a sweep about to be stored.
type draft struct {
	sessionID, carID, trackID, baseline int64
	item                                contracts.SetupItem
	request                             contracts.SweepCreate
	planned                             []int
	skipped                             []string
}

// Start opens a sweep on the session's car and track and starts its first step. The car must be
// free (no experiment on it, no training program or sweep open), the hint pinned, and the
// baseline a setup the agent read back from the car.
func (s *Service) Start(ctx context.Context, sessionID int64, request contracts.SweepCreate) (contracts.Sweep, error) {
	if err := checkHint(request.AiHintShare); err != nil {
		return contracts.Sweep{}, err
	}
	d := draft{sessionID: sessionID, request: request}
	err := s.database.QueryRowContext(ctx, `SELECT car_id, track_id FROM session WHERE id = $1`, sessionID).
		Scan(&d.carID, &d.trackID)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Sweep{}, fmt.Errorf("session %d: %w", sessionID, experiments.ErrNotFound)
	}
	if err != nil {
		return contracts.Sweep{}, fmt.Errorf("read session %d: %w", sessionID, err)
	}
	if err := s.carIsFree(ctx, d.carID); err != nil {
		return contracts.Sweep{}, err
	}
	if d.item, err = s.item(ctx, sessionID, request.Parameter); err != nil {
		return contracts.Sweep{}, err
	}
	if d.baseline, err = s.verifiedBaseline(ctx, sessionID); err != nil {
		return contracts.Sweep{}, err
	}
	if d.planned, d.skipped = planSteps(d.item, request.Steps); len(d.planned) == 0 {
		return contracts.Sweep{}, fmt.Errorf("no step of %v is possible on %s (%v in %v..%v): %w",
			request.Steps, d.item.Label, d.item.Value, d.item.Min, d.item.Max, experiments.ErrInvalid)
	}
	id, err := s.insert(ctx, d)
	if err != nil {
		return contracts.Sweep{}, err
	}
	s.logger.InfoContext(ctx, "sweep started", "sweep_id", id, "session_id", sessionID,
		"parameter", d.item.Name, "steps", d.planned, "ai_hint_share", request.AiHintShare)
	if err := s.advance(ctx, id); err != nil {
		s.logger.ErrorContext(ctx, "sweep could not take its first step", "sweep_id", id, "error", err)
	}
	return s.Get(ctx, id)
}

// checkHint refuses a sweep without a pinned hint in range: steps driven on whatever hint the
// AI learned last cannot be compared (docs/adr/0028-run-context-and-ai-provenance.md).
func checkHint(share float64) error {
	if share < 0.5 || share > 1.5 {
		return fmt.Errorf("a sweep pins the AI's tyre hint: ai_hint_share %v is outside 0.5..1.5: %w",
			share, experiments.ErrInvalid)
	}
	return nil
}

// carIsFree refuses a sweep while anything else may put setups on the car: an open experiment
// (whoever drives it) or an open training program. Another sweep is refused by the unique index.
func (s *Service) carIsFree(ctx context.Context, carID int64) error {
	var experiment, program sql.NullInt64
	err := s.database.QueryRowContext(ctx, `
		SELECT (SELECT e.id FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
		        WHERE h.car_id = $1 AND e.state IN ('running', 'evaluating', 'tradeoff', 'inconclusive')
		        ORDER BY e.id DESC LIMIT 1),
		       (SELECT id FROM training_program WHERE car_id = $1 AND state IN ('running', 'waiting'))`,
		carID).Scan(&experiment, &program)
	if err != nil {
		return fmt.Errorf("look for work on car %d: %w", carID, err)
	}
	if experiment.Valid {
		return fmt.Errorf("experiment %d is on the car; a sweep starts from the baseline, finish it first: %w",
			experiment.Int64, experiments.ErrConflict)
	}
	if program.Valid {
		return fmt.Errorf("training program %d is open on this car; stop it first: %w",
			program.Int64, experiments.ErrConflict)
	}
	return nil
}

// item is the setting to sweep: a linked group is one setting, under its leader
// (docs/adr/0026-linked-setup-items.md), whichever member was asked for.
func (s *Service) item(ctx context.Context, sessionID int64, name string) (contracts.SetupItem, error) {
	menu, err := experiments.SessionMenu(ctx, s.database, sessionID)
	if err != nil {
		return contracts.SetupItem{}, err
	}
	items, err := s.ledger.SetupItems(ctx, sessionID)
	if err != nil {
		return contracts.SetupItem{}, err
	}
	leader := menu.Links.Leader(strings.TrimSpace(name))
	for _, item := range items {
		if item.Name == leader && item.Step > 0 {
			return item, nil
		}
	}
	return contracts.SetupItem{}, fmt.Errorf("%s is not a setup item an experiment may change on this car: %w",
		name, experiments.ErrInvalid)
}

// verifiedBaseline is the setup every step is measured against: the one a new experiment on the
// session starts from, and one the agent read back from the car (it carries the agent's
// fingerprint), so the baseline is known to exist on the car as recorded.
func (s *Service) verifiedBaseline(ctx context.Context, sessionID int64) (int64, error) {
	baseline, err := s.ledger.BaselineVersion(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	var readBack bool
	err = s.database.QueryRowContext(ctx,
		`SELECT agent_hash IS NOT NULL FROM setup_version WHERE id = $1`, baseline).Scan(&readBack)
	if err != nil {
		return 0, fmt.Errorf("read baseline setup %d: %w", baseline, err)
	}
	if !readBack {
		return 0, fmt.Errorf("the baseline, setup %d, was never read back from the car; drive a lap on it first: %w",
			baseline, experiments.ErrConflict)
	}
	return baseline, nil
}

func (s *Service) insert(ctx context.Context, d draft) (int64, error) {
	lists, err := json.Marshal([]any{d.request.Steps, d.planned, d.skipped})
	if err != nil {
		return 0, fmt.Errorf("encode the steps of a sweep: %w", err)
	}
	var id int64
	err = s.database.QueryRowContext(ctx, `
		INSERT INTO sweep (session_id, car_id, track_id, parameter, label, baseline_version_id,
		                   ai_hint_share, requested, planned, skipped)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb -> 0, $8::jsonb -> 1, $8::jsonb -> 2) RETURNING id`,
		d.sessionID, d.carID, d.trackID, d.item.Name, d.item.Label, d.baseline, d.request.AiHintShare,
		string(lists)).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "sweep_one_open_per_car") {
		return 0, fmt.Errorf("a sweep is already open on this car; stop it first: %w", experiments.ErrConflict)
	}
	if err != nil {
		return 0, fmt.Errorf("store sweep: %w", err)
	}
	return id, nil
}

// Stop ends a sweep. The experiment it started is left as it is: the driver can let it finish,
// judge it or abort it.
func (s *Service) Stop(ctx context.Context, id int64) (contracts.Sweep, error) {
	result, err := s.database.ExecContext(ctx,
		`UPDATE sweep SET state = 'stopped', message = 'Stopped by the driver.', updated_at = now()
		 WHERE id = $1 AND state IN ('running', 'waiting')`, id)
	if err != nil {
		return contracts.Sweep{}, fmt.Errorf("stop sweep %d: %w", id, err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		if _, err := s.Get(ctx, id); err != nil {
			return contracts.Sweep{}, err
		}
		return contracts.Sweep{}, fmt.Errorf("sweep %d has already ended: %w", id, experiments.ErrConflict)
	}
	s.logger.InfoContext(ctx, "sweep stopped", "sweep_id", id)
	return s.Get(ctx, id)
}
