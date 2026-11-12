package experiments

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// SetupItems lists what an experiment on the session could change: every item of the car's
// setup menu that an experiment may move, with its value on the setup the experiment would
// start from. Items the setup has no value for are left out.
func (s *Service) SetupItems(ctx context.Context, sessionID int64) ([]contracts.SetupItem, error) {
	base, err := baselineOf(ctx, s.database, sessionID)
	if err != nil {
		return nil, err
	}
	catalog, _, err := loadCatalog(ctx, s.database, base.carID)
	if err != nil {
		return nil, err
	}
	items := []contracts.SetupItem{}
	for name, parameter := range catalog {
		value, present := base.values[name]
		if parameter.Excluded || !present {
			continue
		}
		items = append(items, contracts.SetupItem{
			Name: name, Label: parameter.Label, Min: parameter.Min, Max: parameter.Max,
			Step: parameter.Step, Value: value,
		})
	}
	slices.SortFunc(items, func(a, b contracts.SetupItem) int {
		return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.Name, b.Name))
	})
	return items, nil
}

// knob is what an item does, as the agent's catalog classified it.
type knob struct {
	name       string
	role       string
	axle       string
	shownScale float64
}

// DiagnoseItems is the same list for the engine's diagnosis: each item with what it does,
// and the id of the setup version the values are read from. A session without setup data
// has no items and version 0; that is not an error, the diagnosis then names no item.
func (s *Service) DiagnoseItems(ctx context.Context, sessionID int64) (int64, []contracts.DiagnoseItem, error) {
	base, err := baselineOf(ctx, s.database, sessionID)
	if errors.Is(err, ErrInvalid) {
		return 0, []contracts.DiagnoseItem{}, nil
	}
	if err != nil {
		return 0, nil, err
	}
	changeable, err := s.SetupItems(ctx, sessionID)
	if err != nil {
		return 0, nil, err
	}
	knobs, err := queryAll(ctx, s.database,
		`SELECT name, COALESCE(role, ''), COALESCE(axle, ''), shown_scale
		 FROM setup_parameter WHERE car_id = $1`,
		func(rows *sql.Rows) (knob, error) {
			var k knob
			return k, rows.Scan(&k.name, &k.role, &k.axle, &k.shownScale)
		}, base.carID)
	if err != nil {
		return 0, nil, fmt.Errorf("read item roles of car %d: %w", base.carID, err)
	}
	byName := map[string]knob{}
	for _, k := range knobs {
		byName[k.name] = k
	}
	items := make([]contracts.DiagnoseItem, 0, len(changeable))
	for _, item := range changeable {
		k := byName[item.Name]
		items = append(items, contracts.DiagnoseItem{
			Name: item.Name, Label: item.Label, Role: k.role, Axle: k.axle, ShownScale: k.shownScale,
			Min: item.Min, Max: item.Max, Step: item.Step, Value: item.Value,
		})
	}
	return base.versionID, items, nil
}
