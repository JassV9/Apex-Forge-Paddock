package sensitivity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Ledger is what a base setup needs from the experiments. *experiments.Service implements it.
type Ledger interface {
	BaselineVersion(ctx context.Context, sessionID int64) (int64, error)
	SetupItems(ctx context.Context, sessionID int64) ([]contracts.SetupItem, error)
	CreateBase(ctx context.Context, request experiments.BaseRequest) (contracts.Experiment, error)
}

// BaseEngine chooses a base setup's changes. *engine.Client implements it.
type BaseEngine interface {
	BaseSetup(ctx context.Context, request contracts.BaseSetupRequest) (contracts.BaseSetup, error)
}

// BaseSetups builds a base setup from what a car measured and proposes the experiment that
// judges it (docs/adr/0018-base-setup.md). The choice is the engine's; nothing here or there
// asks a language model.
type BaseSetups struct {
	garage *Service
	ledger Ledger
	engine BaseEngine
	logger *slog.Logger
}

// NewBaseSetups wires base setups to the profiles, the experiments and the engine.
func NewBaseSetups(garage *Service, ledger Ledger, engine BaseEngine, logger *slog.Logger) *BaseSetups {
	return &BaseSetups{garage: garage, ledger: ledger, engine: engine, logger: logger}
}

// Build asks the engine which of the car's measured changes make a base setup against the
// session's baseline and, when at least one does, proposes the experiment that compares the
// two. When none does the reply has no experiment id and its error says what the car has
// measured; that is an answer, not a failure.
func (b *BaseSetups) Build(ctx context.Context, sessionID int64, driver contracts.BaseSetupCreateDriver) (contracts.BaseSetupReply, error) {
	baseline, err := b.ledger.BaselineVersion(ctx, sessionID)
	if err != nil {
		return contracts.BaseSetupReply{}, err
	}
	request, carName, err := b.request(ctx, sessionID)
	if err != nil {
		return contracts.BaseSetupReply{}, err
	}
	chosen, err := b.engine.BaseSetup(ctx, request)
	if err != nil {
		return contracts.BaseSetupReply{}, fmt.Errorf("base setup for session %d: %w", sessionID, err)
	}
	reply := contracts.BaseSetupReply{CarName: carName, BaseSetup: chosen}
	if len(chosen.Changes) == 0 || chosen.Prediction == nil {
		refusal := nothingQualifies(chosen)
		reply.Error = &refusal
		b.logger.InfoContext(ctx, "no base setup", "session_id", sessionID, "measured", len(chosen.Measured))
		return reply, nil
	}
	exp, err := b.ledger.CreateBase(ctx, experiments.BaseRequest{
		SessionID: sessionID, BaselineVersion: baseline, Changes: clicksOf(chosen),
		Statement: statement(len(chosen.Changes), carName), Evidence: evidenceOf(chosen),
		Prediction: predictionSpec(*chosen.Prediction), Driver: contracts.ExperimentCreateDriver(driver),
	})
	if err != nil {
		return contracts.BaseSetupReply{}, err
	}
	reply.ExperimentId = &exp.Id
	return reply, nil
}

// request is the engine's question: the car's profile and the baseline's changeable items,
// each marked when it is one of a pair.
func (b *BaseSetups) request(ctx context.Context, sessionID int64) (contracts.BaseSetupRequest, string, error) {
	carID, err := carOfSession(ctx, b.garage.database, sessionID)
	if err != nil {
		return contracts.BaseSetupRequest{}, "", err
	}
	items, err := b.ledger.SetupItems(ctx, sessionID)
	if err != nil {
		return contracts.BaseSetupRequest{}, "", err
	}
	profiles, err := b.garage.Profiles(ctx)
	if err != nil {
		return contracts.BaseSetupRequest{}, "", err
	}
	cars, err := readCars(ctx, b.garage.database)
	if err != nil {
		return contracts.BaseSetupRequest{}, "", err
	}
	test := experiments.DefaultTest()
	request := contracts.BaseSetupRequest{
		Effects: []contracts.SensitivityEffect{}, Items: baseItems(items, carByID(cars, carID)),
		Channel: test.Channel, Better: contracts.BaseSetupRequestBetter(test.Better), Rope: test.Rope,
		Level: test.Level, Guardrails: experiments.DefaultGuardrails(),
	}
	carName := ""
	for _, profile := range profiles {
		if int64(profile.CarId) == carID {
			request.Effects, carName = profile.Effects, profile.CarName
		}
	}
	return request, carName, nil
}

func carOfSession(ctx context.Context, database *sql.DB, sessionID int64) (int64, error) {
	var carID int64
	err := database.QueryRowContext(ctx, `SELECT car_id FROM session WHERE id = $1`, sessionID).Scan(&carID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("session %d: %w", sessionID, experiments.ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("find session %d: %w", sessionID, err)
	}
	return carID, nil
}

func carByID(cars []car, id int64) car {
	for _, c := range cars {
		if c.id == id {
			return c
		}
	}
	return car{id: id}
}

// baseItems are the baseline's items as the engine reads them. An item is paired when
// another item of the car does the same job (a left and a right damper, a camber per wheel):
// ADR 0011 never moves one side alone.
func baseItems(items []contracts.SetupItem, c car) []contracts.BaseSetupItem {
	answer := make([]contracts.BaseSetupItem, 0, len(items))
	for _, item := range items {
		k, known := c.knob(item.Name)
		answer = append(answer, contracts.BaseSetupItem{
			Name: item.Name, Label: item.Label, Min: item.Min, Max: item.Max, Step: item.Step,
			Value: item.Value, Paired: known && len(sameJob(c, k)) > 1,
		})
	}
	return answer
}
