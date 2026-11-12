package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// The defaults every experiment starts with. They are stored on the experiment when it is
// proposed and nothing changes them afterwards: the test is fixed before a lap is driven.
var (
	// 5 ms per corner pass at 90 %: the band and level of docs/adr/0008-effect-model.md.
	defaultTest = contracts.SuccessTest{
		Channel: "corner_time_s", Rope: 0.005, Level: 0.90, Better: contracts.SuccessTestBetterLower,
	}
	// The rear must not get looser: more peak rear slip or more time on opposite lock.
	defaultGuardrails = []contracts.GuardrailSpec{
		{Channel: "rear_slip_peak_deg", Tolerance: 0.5, Worse: contracts.GuardrailSpecWorseHigher},
		{Channel: "countersteer_s", Tolerance: 0.05, Worse: contracts.GuardrailSpecWorseHigher},
	}
	// If these shift between the setups, the driver drove them differently.
	driverChannels = []string{"brake_dist_m", "throttle_pickup_m", "steer_corrections_per_s"}
)

// Code names leave out A and B (they would suggest an order) and letters that read as digits.
const labelLetters = "CDEFGHJKLMNPQRSTUVWXYZ"

// randomLabels picks two different code names. The baseline gets either with equal chance,
// so a name says nothing about which setup it is.
func randomLabels() (baseline, variant string) {
	first := rand.IntN(len(labelLetters))             //nolint:gosec // a code name, not a secret
	second := rand.IntN(len(labelLetters) - 1)        //nolint:gosec // a code name, not a secret
	second = (first + 1 + second) % len(labelLetters) // never equal to first
	return "Setup " + string(labelLetters[first]), "Setup " + string(labelLetters[second])
}

// draft is an experiment about to be stored.
type draft struct {
	carID           int64
	trackID         int64
	kind            contracts.ExperimentKind
	baselineVersion int64
	variantVersion  int64         // 0: create it from variantValues
	variantValues   setups.Values // the variant's setup when variantVersion is 0
	parameterID     *int64
	clicks          int
	statement       string
	source          string // hypothesis.source: rules, llm or human
	predictions     []contracts.PredictionSpec
}

// baseline is the setup an experiment is measured against.
type baseline struct {
	carID     int64
	trackID   int64
	versionID int64
	values    setups.Values
}

// baselineOf is the setup a new experiment on the session's car and track starts from: the
// champion, or before the first experiment the setup the session's latest pass was driven on.
// The latest pass alone is not enough: after a rejected experiment it may be the rejected
// variant, because no lap is driven on the baseline that goes back on the car.
func baselineOf(ctx context.Context, q querier, sessionID int64) (baseline, error) {
	var base baseline
	var values []byte
	err := q.QueryRowContext(ctx,
		`SELECT s.car_id, s.track_id, v.id, v."values"
		 FROM session s
		 JOIN setup_version v ON v.id = COALESCE(
		     (SELECT c.setup_version_id FROM champion c
		      WHERE c.car_id = s.car_id AND c.track_id = s.track_id),
		     (SELECT p.setup_version_id FROM corner_pass p
		      WHERE p.session_id = s.id AND p.setup_version_id IS NOT NULL
		      ORDER BY p.seq DESC LIMIT 1))
		 WHERE s.id = $1`, sessionID).
		Scan(&base.carID, &base.trackID, &base.versionID, &values)
	if errors.Is(err, sql.ErrNoRows) {
		return baseline{}, fmt.Errorf("session %d has no pass with a recorded setup: %w", sessionID, ErrInvalid)
	}
	if err != nil {
		return baseline{}, fmt.Errorf("read baseline of session %d: %w", sessionID, err)
	}
	if err := json.Unmarshal(values, &base.values); err != nil {
		return baseline{}, fmt.Errorf("decode setup %d: %w", base.versionID, err)
	}
	return base, nil
}

type catalogItem struct {
	id        int64
	parameter setups.Parameter
}

// loadCatalog reads the car's setup menu, with the row id of every item.
func loadCatalog(ctx context.Context, q querier, carID int64) (setups.Catalog, map[string]int64, error) {
	items, err := queryAll(ctx, q,
		`SELECT id, name, label, min, max, step, excluded FROM setup_parameter WHERE car_id = $1`,
		func(rows *sql.Rows) (catalogItem, error) {
			var item catalogItem
			p := &item.parameter
			err := rows.Scan(&item.id, &p.Name, &p.Label, &p.Min, &p.Max, &p.Step, &p.Excluded)
			return item, err
		}, carID)
	if err != nil {
		return nil, nil, fmt.Errorf("read setup menu of car %d: %w", carID, err)
	}
	catalog, ids := setups.Catalog{}, map[string]int64{}
	for _, item := range items {
		catalog[item.parameter.Name] = item.parameter
		ids[item.parameter.Name] = item.id
	}
	return catalog, ids, nil
}

// insertDraft stores a proposed experiment with its hypothesis, arms and predictions, and
// returns its id.
func insertDraft(ctx context.Context, tx *sql.Tx, d draft, baselineLabel, variantLabel string) (int64, error) {
	var hypothesisID, id int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO hypothesis (car_id, track_id, statement, source) VALUES ($1, $2, $3, $4)
		 RETURNING id`, d.carID, d.trackID, d.statement, d.source).Scan(&hypothesisID)
	if err != nil {
		return 0, fmt.Errorf("save hypothesis: %w", err)
	}
	test, guardrails, err := encodeTest()
	if err != nil {
		return 0, err
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO experiment (hypothesis_id, kind, baseline_version_id, parameter_id, clicks,
		                         state, success_test, guardrails)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		hypothesisID, string(d.kind), d.baselineVersion, d.parameterID, d.clicks,
		string(Proposed), test, guardrails).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save experiment: %w", err)
	}
	variant := d.variantVersion
	if variant == 0 {
		if variant, err = insertVersion(ctx, tx, d, id); err != nil {
			return 0, err
		}
	}
	if err := insertArms(ctx, tx, id, d.baselineVersion, variant, baselineLabel, variantLabel); err != nil {
		return 0, err
	}
	if err := insertPredictions(ctx, tx, id, d.predictions); err != nil {
		return 0, err
	}
	return id, addEvent(ctx, tx, id, created, Proposed, d.statement)
}

func encodeTest() (test, guardrails string, err error) {
	encodedTest, err := json.Marshal(defaultTest)
	if err != nil {
		return "", "", fmt.Errorf("encode success test: %w", err)
	}
	encodedGuardrails, err := json.Marshal(defaultGuardrails)
	if err != nil {
		return "", "", fmt.Errorf("encode guardrails: %w", err)
	}
	return string(encodedTest), string(encodedGuardrails), nil
}

// insertVersion stores the variant's setup. The same values on the same car are the same
// version, so a setup that already exists is reused, never duplicated.
func insertVersion(ctx context.Context, tx *sql.Tx, d draft, experimentID int64) (int64, error) {
	encoded, err := json.Marshal(d.variantValues)
	if err != nil {
		return 0, fmt.Errorf("encode variant setup: %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO setup_version (car_id, content_hash, parent_id, "values", created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (car_id, content_hash) DO UPDATE SET content_hash = EXCLUDED.content_hash
		 RETURNING id`,
		d.carID, setups.Hash(d.variantValues), d.baselineVersion, string(encoded),
		fmt.Sprintf("experiment %d", experimentID)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save variant setup: %w", err)
	}
	return id, nil
}

func insertArms(ctx context.Context, tx *sql.Tx, id, baselineVersion, variantVersion int64, baselineLabel, variantLabel string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO experiment_arm (experiment_id, setup_version_id, label_code, is_baseline)
		 VALUES ($1, $2, $3, TRUE), ($1, $4, $5, FALSE)`,
		id, baselineVersion, baselineLabel, variantVersion, variantLabel)
	if err != nil {
		return fmt.Errorf("save arms of experiment %d: %w", id, err)
	}
	return nil
}

func insertPredictions(ctx context.Context, tx *sql.Tx, id int64, predictions []contracts.PredictionSpec) error {
	for _, prediction := range predictions {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO prediction (experiment_id, channel, direction, low, high)
			 VALUES ($1, $2, $3, $4, $5)`,
			id, prediction.Channel, string(prediction.Direction), prediction.Low, prediction.High)
		if err != nil {
			return fmt.Errorf("save prediction on %s: %w", prediction.Channel, err)
		}
	}
	return nil
}
