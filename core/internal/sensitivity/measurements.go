package sensitivity

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// judged is one finished one-item experiment and the stratum it was measured under.
type judged struct {
	car       int64
	id        int
	parameter string
	clicks    int
	stratum   stratum
}

// A finished one-factor experiment says something about one item. One still being driven,
// aborted, a champion re-check or a base setup does not; one judged at another interval level
// than the engine is told is left out rather than misread. The AI's planning share comes from
// the experiment's pin, else from its blocks' run context (ADR 0028).
const judgedSQL = `
SELECT h.car_id, e.id, sp.name, e.clicks, e.driver, t.ac_id, t.layout, t.name, e.ai_hint_share,
       hint.low, hint.high, (sv."values"->>sp.name)::float8, sp.step
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN track t ON t.id = h.track_id
JOIN setup_parameter sp ON sp.id = e.parameter_id
JOIN setup_version sv ON sv.id = e.baseline_version_id
LEFT JOIN LATERAL (
    SELECT min(b.ai_hint_min / s.ai_tyre_hint_base) AS low,
           max(b.ai_hint_max / s.ai_tyre_hint_base) AS high
    FROM run_block rb
    JOIN run_context b ON b.block_id = rb.id
    JOIN run_context s ON s.session_id = b.session_id AND s.block_id IS NULL
    WHERE rb.experiment_id = e.id AND s.ai_tyre_hint_base > 0) hint ON true
WHERE e.kind = 'ofat' AND e.clicks <> 0
  AND e.state IN ('accepted', 'rejected', 'tradeoff', 'inconclusive', 'reverted')
  AND (e.success_test->>'level')::float8 = $1
ORDER BY e.id`

func readJudged(ctx context.Context, database *sql.DB, level float64) ([]judged, error) {
	rows, err := queryAll(ctx, database, judgedSQL, func(rows *sql.Rows) (judged, error) {
		var j judged
		var acID, layout, name string
		var pinned, low, high, value, step sql.NullFloat64
		err := rows.Scan(&j.car, &j.id, &j.parameter, &j.clicks, &j.stratum.driver, &acID, &layout,
			&name, &pinned, &low, &high, &value, &step)
		j.stratum.track, j.stratum.trackName = trackKey(acID, layout, name)
		j.stratum.hint = hintBucket(j.stratum.driver, pinned, low, high)
		j.stratum.baselineValue, j.stratum.clickStep = optional(value), optional(step)
		return j, err
	}, level)
	if err != nil {
		return nil, fmt.Errorf("read judged experiments: %w", err)
	}
	return rows, nil
}

// The ledger's numbers of an experiment the current engine could not re-derive: the current
// result (ADR 0030) of the primary channel and the guardrails, with the latest engine finding.
const ledgerSQL = `
SELECT r.experiment_id, r.channel, r.corner_class, COALESCE(r.phase, 'corner'), r.estimate,
       r.ci_low, r.ci_high, r.n_passes, r.engine_version,
       (SELECT f.finding_class FROM verdict_finding f
        WHERE f.experiment_id = r.experiment_id AND f.decided_by = 'engine'
        ORDER BY f.id DESC LIMIT 1)
FROM current_result r
WHERE r.role IN ('primary', 'guardrail') AND r.corner_class IN ('all', 'slow', 'medium', 'fast')
ORDER BY r.experiment_id, r.id`

func readLedger(ctx context.Context, database *sql.DB) (map[int][]contracts.Measurement, error) {
	byExperiment := map[int][]contracts.Measurement{}
	_, err := queryAll(ctx, database, ledgerSQL, func(rows *sql.Rows) (contracts.Measurement, error) {
		var m contracts.Measurement
		var low, high float64
		var version, finding sql.NullString
		err := rows.Scan(&m.ExperimentId, &m.Channel, &m.CornerClass, &m.Phase, &m.Estimate,
			&low, &high, &m.NPasses, &version, &finding)
		m.CiLow, m.CiHigh, m.Stale = &low, &high, true
		m.EngineVersion, m.FindingClass = nullable[string](version), nullable[contracts.MeasurementFindingClass](finding)
		byExperiment[m.ExperimentId] = append(byExperiment[m.ExperimentId], m)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("read the ledger's results: %w", err)
	}
	return byExperiment, nil
}

func nullable[T ~string](value sql.NullString) *T {
	if !value.Valid {
		return nil
	}
	typed := T(value.String)
	return &typed
}

// readMeasurements is what the engine pools, per car: each experiment's re-derivation at the
// engine's version when it has one, else the ledger's numbers marked stale with the reason. An
// experiment re-derived as invalid sends the ledger's rows with that finding, so the engine
// lists it as excluded instead of it vanishing.
func readMeasurements(ctx context.Context, database *sql.DB, level float64, version string) (map[int64][]contracts.Measurement, error) {
	experiments, err := readJudged(ctx, database, level)
	if err != nil {
		return nil, err
	}
	derived, err := readDerivations(ctx, database, version)
	if err != nil {
		return nil, err
	}
	ledger, err := readLedger(ctx, database)
	if err != nil {
		return nil, err
	}
	byCar := map[int64][]contracts.Measurement{}
	for _, j := range experiments {
		for _, m := range measurementsOf(derived[j.id], ledger[j.id], version) {
			byCar[j.car] = append(byCar[j.car], j.stamp(m))
		}
	}
	return byCar, nil
}

func measurementsOf(d *derivation, ledger []contracts.Measurement, version string) []contracts.Measurement {
	switch {
	case d == nil:
		return markStale(ledger, "Not re-derived yet by engine "+version+".", nil)
	case d.status == "failed":
		return markStale(ledger, "The current engine could not re-derive it: "+d.reason, nil)
	case d.finding == string(contracts.MeasurementFindingClassInvalid):
		invalid := contracts.MeasurementFindingClassInvalid
		return markStale(ledger, d.reason, &invalid)
	default:
		return d.measurements(version)
	}
}

func markStale(rows []contracts.Measurement, note string, finding *contracts.MeasurementFindingClass) []contracts.Measurement {
	out := make([]contracts.Measurement, len(rows))
	for i, m := range rows {
		m.Note = note
		if finding != nil {
			m.FindingClass = finding
		}
		out[i] = m
	}
	return out
}

// stamp gives a measurement its experiment's item, clicks and stratum.
func (j judged) stamp(m contracts.Measurement) contracts.Measurement {
	m.ExperimentId, m.Parameter, m.Clicks = j.id, j.parameter, j.clicks
	m.Track, m.TrackName, m.HintBucket = j.stratum.track, j.stratum.trackName, j.stratum.hint
	m.Driver = contracts.MeasurementDriver(j.stratum.driver)
	m.BaselineValue, m.ClickStep = j.stratum.baselineValue, j.stratum.clickStep
	return m
}
