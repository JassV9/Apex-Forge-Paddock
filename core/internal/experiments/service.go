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

// Create proposes an experiment: one setting moved by some clicks, against the setup the
// session was driven on. The variant is validated against the car's setup menu here, so an
// experiment that could not be driven is never stored.
func (s *Service) Create(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	return s.create(ctx, request, contracts.ExperimentPurposeImprove)
}

// CreateSweepStep proposes one step of a sensitivity sweep: an experiment like any other, but
// measured and never promoted, so the baseline is the same for every step
// (docs/adr/0032-sensitivity-discovery-sweeps.md).
func (s *Service) CreateSweepStep(ctx context.Context, request contracts.ExperimentCreate) (contracts.Experiment, error) {
	return s.create(ctx, request, contracts.ExperimentPurposeSweep)
}

func (s *Service) create(ctx context.Context, request contracts.ExperimentCreate, purpose contracts.ExperimentPurpose) (contracts.Experiment, error) {
	if err := checkRequest(request); err != nil {
		return contracts.Experiment{}, err
	}
	base, err := baselineOf(ctx, s.database, int64(request.SessionId))
	if err != nil {
		return contracts.Experiment{}, err
	}
	variant, parameterID, err := oneSetting(ctx, s.database, base, request.Parameter, request.Clicks)
	if err != nil {
		return contracts.Experiment{}, err
	}
	cameFrom, err := originOf(ctx, s.database, int64(request.SessionId), request.HypothesisKey)
	if err != nil {
		return contracts.Experiment{}, err
	}
	id, err := s.store(ctx, draft{
		carID: base.carID, trackID: base.trackID, kind: contracts.ExperimentKindOfat,
		baselineVersion: base.versionID, variantValues: variant, parameterID: &parameterID,
		clicks: request.Clicks, statement: strings.TrimSpace(request.Statement), origin: cameFrom,
		predictions: request.Predictions, driver: request.Driver, aiHintShare: request.AiHintShare,
		purpose: purpose,
	})
	if err != nil {
		return contracts.Experiment{}, err
	}
	s.logger.InfoContext(ctx, "experiment proposed", "experiment_id", id,
		"parameter", request.Parameter, "clicks", request.Clicks, "driver", driverOf(request.Driver),
		"purpose", string(purpose))
	return s.Get(ctx, id)
}

// oneSetting is the variant of a one-factor experiment and the id of the item it is filed
// under. A linked item moves with its group, which is filed under its leader, whichever member
// was asked for (docs/adr/0026-linked-setup-items.md).
func oneSetting(ctx context.Context, q querier, base baseline, name string, clicks int) (setups.Values, int64, error) {
	catalog, ids, err := loadCatalog(ctx, q, base.carID)
	if err != nil {
		return nil, 0, err
	}
	links, err := loadLinks(ctx, q, base.carID, catalog)
	if err != nil {
		return nil, 0, err
	}
	variant, err := setups.Step(catalog, links, base.values, name, clicks)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return variant, ids[links.Leader(name)], nil
}

func checkRequest(request contracts.ExperimentCreate) error {
	if strings.TrimSpace(request.Statement) == "" {
		return fmt.Errorf("the hypothesis needs a statement: %w", ErrInvalid)
	}
	if err := checkHintPin(request.AiHintShare, request.Driver); err != nil {
		return err
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
	if err := s.outbox.Write(plan, exp.carACID, exp.kind, arms); err != nil {
		return fmt.Errorf("experiment %d: %w", id, err)
	}
	return nil
}

// PublishRunning writes the run plan of every experiment that is on its car again. Core calls
// it at startup, so an outbox that was emptied or moved, or plans in an older layout, are
// put back where the agent reads them.
func (s *Service) PublishRunning(ctx context.Context) error {
	states := make([]string, len(occupying))
	for i, state := range occupying {
		states[i] = string(state)
	}
	ids, err := queryAll(ctx, s.database, `SELECT id FROM experiment WHERE state = ANY($1) ORDER BY id`,
		func(rows *sql.Rows) (int64, error) {
			var id int64
			return id, rows.Scan(&id)
		}, states)
	if err != nil {
		return fmt.Errorf("list running experiments: %w", err)
	}
	for _, id := range ids {
		if err := s.publish(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Plan asks the engine for an A-B-A-B plan sized for the baseline's noise (ADR 0025) and stores
// it. From here on the success test is fixed: nothing in the service changes it.
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
	reason := planReason(plan, exp.test.Channel)
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
