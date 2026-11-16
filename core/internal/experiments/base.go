package experiments

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// BaseRequest is a base setup to be judged against its baseline: several changes this car
// measured one at a time, now driven together (docs/adr/0018-base-setup.md).
type BaseRequest struct {
	SessionID int64
	// BaselineVersion is the setup the changes were chosen against; the experiment is refused
	// when the session's baseline has moved since.
	BaselineVersion int64
	Changes         map[string]int // item to clicks
	Statement       string
	Evidence        []string // what each change rests on, kept with the hypothesis
	Prediction      contracts.PredictionSpec
	Driver          contracts.ExperimentCreateDriver
}

// BaselineVersion is the setup version a new experiment on the session would start from. A
// base setup is built against it.
func (s *Service) BaselineVersion(ctx context.Context, sessionID int64) (int64, error) {
	base, err := baselineOf(ctx, s.database, sessionID)
	if err != nil {
		return 0, err
	}
	return base.versionID, nil
}

// CreateBase proposes the experiment that judges a base setup: the baseline against the
// baseline with every change applied. Each change is a validated step on the car's setup
// menu, so a base setup that could not be driven is never stored.
func (s *Service) CreateBase(ctx context.Context, request BaseRequest) (contracts.Experiment, error) {
	if len(request.Changes) == 0 || strings.TrimSpace(request.Statement) == "" {
		return contracts.Experiment{}, fmt.Errorf("a base setup needs a change and a statement: %w", ErrInvalid)
	}
	base, err := baselineOf(ctx, s.database, request.SessionID)
	if err != nil {
		return contracts.Experiment{}, err
	}
	if base.versionID != request.BaselineVersion {
		return contracts.Experiment{}, fmt.Errorf("the baseline of session %d moved from setup %d to %d; build the base setup again: %w",
			request.SessionID, request.BaselineVersion, base.versionID, ErrConflict)
	}
	values, err := applyChanges(ctx, s, base, request.Changes)
	if err != nil {
		return contracts.Experiment{}, err
	}
	cameFrom, err := baseOrigin(request.Evidence)
	if err != nil {
		return contracts.Experiment{}, err
	}
	id, err := s.store(ctx, draft{
		carID: base.carID, trackID: base.trackID, kind: contracts.ExperimentKindBase,
		baselineVersion: base.versionID, variantValues: values,
		statement: strings.TrimSpace(request.Statement), origin: cameFrom,
		predictions: []contracts.PredictionSpec{request.Prediction}, driver: request.Driver,
	})
	if err != nil {
		return contracts.Experiment{}, err
	}
	s.logger.InfoContext(ctx, "base setup proposed", "experiment_id", id,
		"session_id", request.SessionID, "changes", len(request.Changes), "driver", driverOf(request.Driver))
	return s.Get(ctx, id)
}

// applyChanges steps the baseline by every change, in item order.
func applyChanges(ctx context.Context, s *Service, base baseline, changes map[string]int) (setups.Values, error) {
	catalog, _, err := loadCatalog(ctx, s.database, base.carID)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(changes))
	for name := range changes {
		names = append(names, name)
	}
	slices.Sort(names)
	values := base.values
	for _, name := range names {
		if values, err = setups.Step(catalog, values, name, changes[name]); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	return values, nil
}

func baseOrigin(evidence []string) (origin, error) {
	if evidence == nil {
		evidence = []string{}
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return origin{}, fmt.Errorf("encode base setup evidence: %w", err)
	}
	cameFrom := rulesOrigin()
	cameFrom.evidence = string(encoded)
	return cameFrom, nil
}
