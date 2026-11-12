package experiments

import (
	"cmp"
	"context"
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
