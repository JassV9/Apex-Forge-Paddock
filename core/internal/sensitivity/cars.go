package sensitivity

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// knob is one item of a car's setup menu and what the agent's catalog says it does.
type knob struct {
	id         int64
	name       string
	label      string
	kind       string
	role       string
	axle       string
	shownScale float64
}

type car struct {
	id         int64
	label      string // the name, made unique among the cars
	drivetrain string
	judged     int // experiments the engine has given a verdict on
	knobs      []knob
	links      setups.Links
}

func (c car) knob(name string) (knob, bool) {
	for _, k := range c.knobs {
		if k.name == name {
			return k, true
		}
	}
	return knob{}, false
}

func queryAll[T any](ctx context.Context, database *sql.DB, query string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Two cars may share a display name; then the game's id tells them apart.
const carsSQL = `
SELECT c.id,
       CASE WHEN count(*) OVER (PARTITION BY c.name) > 1 THEN c.name || ' (' || c.ac_id || ')'
            ELSE c.name END,
       COALESCE(c.drivetrain, ''),
       (SELECT count(*) FROM experiment e JOIN hypothesis h ON h.id = e.hypothesis_id
        WHERE h.car_id = c.id
          AND EXISTS (SELECT 1 FROM verdict v
                      WHERE v.experiment_id = e.id AND v.decided_by = 'engine'))
FROM car c ORDER BY 2, c.id`

const knobsSQL = `
SELECT car_id, id, name, label, COALESCE(kind, ''), COALESCE(role, ''), COALESCE(axle, ''),
       shown_scale
FROM setup_parameter ORDER BY car_id, name`

func readCars(ctx context.Context, database *sql.DB) ([]car, error) {
	cars, err := queryAll(ctx, database, carsSQL, func(rows *sql.Rows) (car, error) {
		var c car
		return c, rows.Scan(&c.id, &c.label, &c.drivetrain, &c.judged)
	})
	if err != nil {
		return nil, fmt.Errorf("read cars: %w", err)
	}
	byCar := map[int64][]knob{}
	_, err = queryAll(ctx, database, knobsSQL, func(rows *sql.Rows) (knob, error) {
		var carID int64
		var k knob
		err := rows.Scan(&carID, &k.id, &k.name, &k.label, &k.kind, &k.role, &k.axle, &k.shownScale)
		byCar[carID] = append(byCar[carID], k)
		return k, err
	})
	if err != nil {
		return nil, fmt.Errorf("read setup items: %w", err)
	}
	for i := range cars {
		if err := cars[i].oneKnobPerSetting(ctx, database, byCar[cars[i].id]); err != nil {
			return nil, err
		}
	}
	return cars, nil
}

// oneKnobPerSetting keeps a linked group as one knob, its leader's, labelled with its members:
// the group is one setting, so it does one job (docs/adr/0026-linked-setup-items.md).
func (c *car) oneKnobPerSetting(ctx context.Context, database *sql.DB, knobs []knob) error {
	menu, err := experiments.CarMenu(ctx, database, c.id)
	if err != nil {
		return err
	}
	c.links = menu.Links
	for _, k := range knobs {
		if c.links.Leader(k.name) != k.name {
			continue
		}
		if _, linked := c.links[k.name]; linked {
			k.label = c.links.Label(menu.Catalog, k.name)
		}
		c.knobs = append(c.knobs, k)
	}
	return nil
}

// fileUnderLeaders files every measurement of a linked item under its group's leader: testing
// one member tested the group.
func fileUnderLeaders(cars []car, measurements map[int64][]contracts.Measurement) {
	for _, c := range cars {
		for i := range measurements[c.id] {
			measurements[c.id][i].Parameter = c.links.Leader(measurements[c.id][i].Parameter)
		}
	}
}
