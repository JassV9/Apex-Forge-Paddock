// Package sensitivity keeps what each setup item does on each car, and asks the engine which
// experiment to run next. Core gathers and stores; the pooling, the priors and the ranking
// are the engine's (docs/adr/0011-sensitivity-and-ranking.md).
package sensitivity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Engine pools effects and ranks candidates. *engine.Client implements it.
type Engine interface {
	Sensitivity(ctx context.Context, request contracts.SensitivityRequest) (contracts.SensitivityResponse, error)
	Propose(ctx context.Context, request contracts.ProposeRequest) (contracts.Proposals, error)
}

// Diagnoses knows what a session's passes say about the car. *diagnosis.Service
// implements it.
type Diagnoses interface {
	Latest(ctx context.Context, sessionID int64) (contracts.Diagnosis, error)
}

// Service builds sensitivity profiles, proposals and the scorecard. Its errors are the ones
// the experiments use, so the API answers all of them the same way.
type Service struct {
	database  *sql.DB
	engine    Engine
	diagnoses Diagnoses
	logger    *slog.Logger
}

// NewService wires the service to its database, the engine and the diagnoses.
func NewService(database *sql.DB, engine Engine, diagnoses Diagnoses, logger *slog.Logger) *Service {
	return &Service{database: database, engine: engine, diagnoses: diagnoses, logger: logger}
}

// built is one car's profile as the engine made it.
type built struct {
	car   car
	reply contracts.SensitivityResponse
}

// Profiles rebuilds every car's profile from the finished experiments, stores the rows and
// returns them. Nothing is updated in place: the profile is always what today's results and
// today's method say, and the stored rows record which method that was.
func (s *Service) Profiles(ctx context.Context) ([]contracts.SensitivityProfile, error) {
	cars, err := readCars(ctx, s.database)
	if err != nil {
		return nil, err
	}
	measurements, err := readMeasurements(ctx, s.database, experiments.DefaultTest().Level)
	if err != nil {
		return nil, err
	}
	profiles, err := s.build(ctx, cars, measurements)
	if err != nil {
		return nil, err
	}
	if err := save(ctx, s.database, cars, profiles); err != nil {
		return nil, err
	}
	answer := make([]contracts.SensitivityProfile, 0, len(profiles))
	for _, profile := range profiles {
		answer = append(answer, profileOf(profile))
	}
	s.logger.InfoContext(ctx, "sensitivity profiles rebuilt", "cars", len(cars))
	return answer, nil
}

// build asks the engine twice: first for what each car measured itself, then, for a car
// another car can lend to, again with those measurements as transfers.
func (s *Service) build(ctx context.Context, cars []car, measurements map[int64][]contracts.Measurement) ([]built, error) {
	measured := map[int64]contracts.SensitivityResponse{}
	for _, c := range cars {
		reply, err := s.ask(ctx, c, measurements[c.id], nil)
		if err != nil {
			return nil, err
		}
		measured[c.id] = reply
	}
	profiles := make([]built, 0, len(cars))
	for _, c := range cars {
		reply := measured[c.id]
		if borrowed := transfersFor(c, cars, measured); len(borrowed) > 0 {
			var err error
			if reply, err = s.ask(ctx, c, measurements[c.id], borrowed); err != nil {
				return nil, err
			}
		}
		profiles = append(profiles, built{car: c, reply: reply})
	}
	return profiles, nil
}

func (s *Service) ask(ctx context.Context, c car, measurements []contracts.Measurement, transfers []contracts.Transfer) (contracts.SensitivityResponse, error) {
	request := contracts.SensitivityRequest{
		Measurements: measurements, Transfers: transfers,
		Ropes: ropes(), Level: experiments.DefaultTest().Level,
	}
	if request.Measurements == nil {
		request.Measurements = []contracts.Measurement{} // [] in JSON, never null
	}
	if request.Transfers == nil {
		request.Transfers = []contracts.Transfer{}
	}
	reply, err := s.engine.Sensitivity(ctx, request)
	if err != nil {
		return reply, fmt.Errorf("sensitivity profile of car %d: %w", c.id, err)
	}
	return reply, nil
}

// ropes are the bands of practical equivalence the experiments are judged with: the success
// test's on its channel, and each guardrail's tolerance on its own.
func ropes() contracts.SensitivityRequestRopes {
	test := experiments.DefaultTest()
	bands := contracts.SensitivityRequestRopes{test.Channel: test.Rope}
	for _, guardrail := range experiments.DefaultGuardrails() {
		bands[guardrail.Channel] = guardrail.Tolerance
	}
	return bands
}

func profileOf(profile built) contracts.SensitivityProfile {
	labels := contracts.SensitivityProfileLabels{}
	for _, effect := range profile.reply.Effects {
		if k, known := profile.car.knob(effect.Parameter); known {
			labels[effect.Parameter] = k.label
		}
	}
	effects := profile.reply.Effects
	if effects == nil {
		effects = []contracts.SensitivityEffect{}
	}
	return contracts.SensitivityProfile{
		CarId: int(profile.car.id), CarName: profile.car.label, Experiments: profile.car.judged,
		ModelVersion: profile.reply.ModelVersion, Labels: labels, Effects: effects,
	}
}
