package experiments

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// DefaultLapsPerBlock gives 6 passes per setup in every corner over an A-B-A-B plan: the
// plan the statistics were validated on (docs/adr/0008-effect-model.md).
const DefaultLapsPerBlock = 3

// Engine plans and judges experiments. *engine.Client implements it.
type Engine interface {
	Plan(ctx context.Context, request contracts.PlanRequest) (contracts.PlanResponse, error)
	Evaluate(ctx context.Context, request contracts.EvaluateRequest) (contracts.EvaluateResponse, error)
}

// Service is every action on experiments. Each action is one transaction on the locked
// experiment; the engine is asked before the transaction starts, never inside it.
type Service struct {
	database     *sql.DB
	engine       Engine
	outbox       Outbox
	logger       *slog.Logger
	lapsPerBlock int
}

// NewService wires the service to its database, the engine and the outbox.
func NewService(database *sql.DB, engine Engine, outbox Outbox, logger *slog.Logger) *Service {
	return &Service{
		database: database, engine: engine, outbox: outbox, logger: logger,
		lapsPerBlock: DefaultLapsPerBlock,
	}
}

// Get returns one experiment with its plan, results, verdict and history.
func (s *Service) Get(ctx context.Context, id int64) (contracts.Experiment, error) {
	return readExperiment(ctx, s.database, id)
}

// List returns every experiment, newest first: the ledger. Failed ones are kept.
func (s *Service) List(ctx context.Context) ([]contracts.Experiment, error) {
	ids, err := listIDs(ctx, s.database)
	if err != nil {
		return nil, fmt.Errorf("list experiments: %w", err)
	}
	experiments := make([]contracts.Experiment, 0, len(ids))
	for _, id := range ids {
		exp, err := readExperiment(ctx, s.database, id)
		if err != nil {
			return nil, err
		}
		experiments = append(experiments, exp)
	}
	return experiments, nil
}

// Create proposes an experiment: one setup item moved by some clicks, against the setup the
// session was driven on. The variant is validated against the car's setup menu here, so an
// experiment that could not be driven is never stored.
func (s *Service) Create(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	if err := checkRequest(request); err != nil {
		return contracts.Experiment{}, err
	}
	base, err := baselineOf(ctx, s.database, int64(request.SessionId))
	if err != nil {
		return contracts.Experiment{}, err
	}
	catalog, ids, err := loadCatalog(ctx, s.database, base.carID)
	if err != nil {
		return contracts.Experiment{}, err
	}
	variant, err := setups.Step(catalog, base.values, request.Parameter, request.Clicks)
	if err != nil {
		return contracts.Experiment{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cameFrom, err := originOf(ctx, s.database, int64(request.SessionId), request.HypothesisKey)
	if err != nil {
		return contracts.Experiment{}, err
	}
	parameterID := ids[request.Parameter]
	id, err := s.store(ctx, draft{
		carID: base.carID, trackID: base.trackID, kind: contracts.ExperimentKindOfat,
		baselineVersion: base.versionID, variantValues: variant, parameterID: &parameterID,
		clicks: request.Clicks, statement: strings.TrimSpace(request.Statement), origin: cameFrom,
		predictions: request.Predictions,
	})
	if err != nil {
		return contracts.Experiment{}, err
	}
	s.logger.InfoContext(ctx, "experiment proposed", "experiment_id", id,
		"parameter", request.Parameter, "clicks", request.Clicks)
	return s.Get(ctx, id)
}

func checkRequest(request contracts.ExperimentCreate) error {
	if strings.TrimSpace(request.Statement) == "" {
		return fmt.Errorf("the hypothesis needs a statement: %w", ErrInvalid)
	}
	for _, prediction := range request.Predictions {
		if prediction.Low > prediction.High {
			return fmt.Errorf("prediction on %s: low %v is above high %v: %w",
				prediction.Channel, prediction.Low, prediction.High, ErrInvalid)
		}
	}
	return nil
}

func (s *Service) store(ctx context.Context, d draft) (int64, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit
	baselineLabel, variantLabel := randomLabels()
	id, err := insertDraft(ctx, tx, d, baselineLabel, variantLabel)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

// act runs work on the locked experiment and returns the experiment as it is afterwards.
// With publish, the run plan in the outbox is rewritten from what was just stored.
func (s *Service) act(ctx context.Context, id int64, publish bool, work func(tx *sql.Tx, exp record) error) (contracts.Experiment, error) {
	if err := inTx(ctx, s.database, id, work); err != nil {
		return contracts.Experiment{}, err
	}
	if publish {
		if err := s.publish(ctx, id); err != nil {
			return contracts.Experiment{}, err
		}
	}
	exp, err := s.Get(ctx, id)
	if err == nil {
		s.logger.InfoContext(ctx, "experiment changed", "experiment_id", id, "state", string(exp.State))
	}
	return exp, err
}

// publish writes the experiment's run plan and setup files into the outbox.
func (s *Service) publish(ctx context.Context, id int64) error {
	exp, err := loadRecord(ctx, s.database, id, false)
	if err != nil {
		return err
	}
	plan, arms, err := readRunPlan(ctx, s.database, id)
	if err != nil {
		return err
	}
	if err := s.outbox.Write(plan, exp.carACID, arms); err != nil {
		return fmt.Errorf("experiment %d: %w", id, err)
	}
	return nil
}

// Plan asks the engine what an A-B-A-B plan can detect and stores the plan. From here on the
// success test is fixed: nothing in the service changes it.
func (s *Service) Plan(ctx context.Context, id int64) (contracts.Experiment, error) {
	exp, err := loadRecord(ctx, s.database, id, false)
	if err != nil {
		return contracts.Experiment{}, err
	}
	if !CanMove(exp.state, Planned) {
		return contracts.Experiment{}, fmt.Errorf("experiment %d is %s: %w", id, exp.state, ErrConflict)
	}
	request, err := planRequest(ctx, s.database, exp, s.lapsPerBlock)
	if err != nil {
		return contracts.Experiment{}, err
	}
	plan, err := s.engine.Plan(ctx, request)
	if err != nil {
		return contracts.Experiment{}, fmt.Errorf("plan experiment %d: %w", id, err)
	}
	reason := fmt.Sprintf("%d blocks of %d laps; decides effects of %.3g or more on %s (scatter %s).",
		len(plan.Blocks), s.lapsPerBlock, plan.MdeDecide, exp.test.Channel, plan.SigmaSource)
	return s.act(ctx, id, false, func(tx *sql.Tx, current record) error {
		if err := move(ctx, tx, id, current.state, Planned, reason); err != nil {
			return err
		}
		return savePlan(ctx, tx, id, plan)
	})
}

// Start puts the run plan and both setups into the outbox for the agent. Only one experiment
// at a time can have its setups on the car, and it must start from the champion.
func (s *Service) Start(ctx context.Context, id int64) (contracts.Experiment, error) {
	return s.act(ctx, id, true, func(tx *sql.Tx, exp record) error {
		other, err := otherOccupying(ctx, tx, id)
		if err != nil {
			return err
		}
		if other != 0 {
			return fmt.Errorf("experiment %d is still on the car: %w", other, ErrConflict)
		}
		if err := anchor(ctx, tx, exp); err != nil {
			return err
		}
		if err := move(ctx, tx, id, exp.state, Running, "Run plan and setups written for the agent."); err != nil {
			return err
		}
		return bumpRevision(ctx, tx, id)
	})
}
