package sensitivity

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
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
		cars[i].knobs = byCar[cars[i].id]
	}
	return cars, nil
}

// A measurement is one result of a finished one-factor experiment. An experiment that is
// still being driven, was aborted, or re-checks a champion says nothing about one item. The
// intervals must all be at the level the engine is told, so an experiment judged at another
// level is left out rather than misread.
const measurementsSQL = `
SELECT h.car_id, e.id, sp.name, e.clicks, r.channel, r.corner_class, r.estimate, r.ci_low,
       r.ci_high, r.n_passes
FROM result r
JOIN experiment e ON e.id = r.experiment_id
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN setup_parameter sp ON sp.id = e.parameter_id
WHERE e.kind = 'ofat' AND e.clicks <> 0
  AND e.state IN ('accepted', 'rejected', 'tradeoff', 'inconclusive', 'reverted')
  AND (e.success_test->>'level')::float8 = $1
  AND r.corner_class IN ('all', 'slow', 'medium', 'fast')
ORDER BY e.id, r.id`

func readMeasurements(ctx context.Context, database *sql.DB, level float64) (map[int64][]contracts.Measurement, error) {
	byCar := map[int64][]contracts.Measurement{}
	_, err := queryAll(ctx, database, measurementsSQL, func(rows *sql.Rows) (contracts.Measurement, error) {
		var carID int64
		var m contracts.Measurement
		err := rows.Scan(&carID, &m.ExperimentId, &m.Parameter, &m.Clicks, &m.Channel,
			&m.CornerClass, &m.Estimate, &m.CiLow, &m.CiHigh, &m.NPasses)
		byCar[carID] = append(byCar[carID], m)
		return m, err
	}, level)
	if err != nil {
		return nil, fmt.Errorf("read measured effects: %w", err)
	}
	return byCar, nil
}
