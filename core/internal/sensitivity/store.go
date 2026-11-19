package sensitivity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const insertEffectSQL = `
INSERT INTO sensitivity_effect (car_id, parameter_id, direction, clicks, channel, corner_class,
    phase, track, track_name, driver, hint_bucket, baseline_value, click_step, estimate, ci_low,
    ci_high, standard_error, p_increase, pi_low, pi_high, tau, margin, method, experiments,
    knowledge_level, effect_class, n_passes, nonlinear, note, stale, experiment_ids,
    engine_versions, model_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
    $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31::bigint[], $32::text[], $33)`

const insertPriorSQL = `
INSERT INTO sensitivity_prior (car_id, parameter_id, direction, clicks, channel, corner_class,
    phase, estimate, ci_low, ci_high, standard_error, source_car_id, source_track,
    source_experiments, weight, note, model_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

// save stores each car's pooled rows and priors, replacing the car's rows only when its profile
// changed since the last read (the hash of the engine's reply). The rows are derived, never
// edited in place (ADR 0011); the lock keeps two rebuilds from interleaving.
func save(ctx context.Context, database *sql.DB, cars []car, profiles []built) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin saving sensitivity profiles: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	if _, err := tx.ExecContext(ctx, `LOCK TABLE sensitivity_profile IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock sensitivity profiles: %w", err)
	}
	lenders := map[string]int64{}
	for _, c := range cars {
		lenders[c.label] = c.id
	}
	for _, profile := range profiles {
		if err := saveCar(ctx, tx, profile, lenders); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sensitivity profiles: %w", err)
	}
	return nil
}

func saveCar(ctx context.Context, tx *sql.Tx, profile built, lenders map[string]int64) error {
	encoded, err := json.Marshal(profile.reply)
	if err != nil {
		return fmt.Errorf("encode the profile of car %d: %w", profile.car.id, err)
	}
	sum := sha256.Sum256(encoded)
	hash := hex.EncodeToString(sum[:])
	var stored string
	err = tx.QueryRowContext(ctx, `SELECT inputs_hash FROM sensitivity_profile WHERE car_id = $1`, profile.car.id).Scan(&stored)
	if err == nil && stored == hash {
		return nil // nothing changed: a read writes nothing
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read the profile hash of car %d: %w", profile.car.id, err)
	}
	for _, clear := range []string{`DELETE FROM sensitivity_effect WHERE car_id = $1`, `DELETE FROM sensitivity_prior WHERE car_id = $1`} {
		if _, err := tx.ExecContext(ctx, clear, profile.car.id); err != nil {
			return fmt.Errorf("clear the profile of car %d: %w", profile.car.id, err)
		}
	}
	if err := insertRows(ctx, tx, profile, lenders); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO sensitivity_profile (car_id, inputs_hash, model_version) VALUES ($1, $2, $3)
ON CONFLICT (car_id) DO UPDATE SET inputs_hash = $2, model_version = $3, built_at = now()`,
		profile.car.id, hash, profile.reply.ModelVersion)
	if err != nil {
		return fmt.Errorf("store the profile hash of car %d: %w", profile.car.id, err)
	}
	return nil
}

func insertRows(ctx context.Context, tx *sql.Tx, profile built, lenders map[string]int64) error {
	for _, effect := range profile.reply.Effects {
		if err := insertEffect(ctx, tx, profile, effect); err != nil {
			return err
		}
	}
	for _, prior := range profile.reply.Priors {
		if err := insertPrior(ctx, tx, profile, prior, lenders); err != nil {
			return err
		}
	}
	return nil
}

// stored is how a row names its item and clicks: the item's id, and a direction of the raw value
// with a positive count.
func stored(profile built, parameter string, clicks int) (int64, string, int, error) {
	k, known := profile.car.knob(parameter)
	if !known || clicks == 0 {
		return 0, "", 0, fmt.Errorf("the engine returned an effect of %q by %d clicks, which car %d cannot have",
			parameter, clicks, profile.car.id)
	}
	if clicks < 0 {
		return k.id, "decrease", -clicks, nil
	}
	return k.id, "increase", clicks, nil
}

func insertEffect(ctx context.Context, tx *sql.Tx, profile built, e contracts.SensitivityEffect) error {
	id, direction, clicks, err := stored(profile, e.Parameter, e.Clicks)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, insertEffectSQL, profile.car.id, id, direction, clicks, e.Channel,
		string(e.CornerClass), e.Phase, e.Track, e.TrackName, string(e.Driver), e.HintBucket,
		e.BaselineValue, e.ClickStep, e.Estimate, e.CiLow, e.CiHigh, e.StandardError, e.PIncrease,
		e.PiLow, e.PiHigh, e.Tau, e.Margin, string(e.Method), e.Experiments, string(e.KnowledgeLevel),
		string(e.EffectClass), e.NPasses, e.Nonlinear, e.Note, e.Stale, idList(e.ExperimentIds),
		textList(e.EngineVersions), profile.reply.ModelVersion)
	if err != nil {
		return fmt.Errorf("save effect of %s on car %d: %w", e.Parameter, profile.car.id, err)
	}
	return nil
}

func insertPrior(ctx context.Context, tx *sql.Tx, profile built, p contracts.SensitivityPrior, lenders map[string]int64) error {
	id, direction, clicks, err := stored(profile, p.Parameter, p.Clicks)
	if err != nil {
		return err
	}
	lender, known := lenders[p.SourceCar]
	if !known {
		return fmt.Errorf("prior on car %d from %q, which is not a car", profile.car.id, p.SourceCar)
	}
	_, err = tx.ExecContext(ctx, insertPriorSQL, profile.car.id, id, direction, clicks, p.Channel,
		string(p.CornerClass), p.Phase, p.Estimate, p.CiLow, p.CiHigh, p.StandardError, lender,
		p.SourceTrack, p.SourceExperiments, p.Weight, p.Note, profile.reply.ModelVersion)
	if err != nil {
		return fmt.Errorf("save prior of %s on car %d: %w", p.Parameter, profile.car.id, err)
	}
	return nil
}

// idList writes ids as a PostgreSQL array literal.
func idList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// textList writes strings as a PostgreSQL array literal, each quoted.
func textList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconv.Quote(item)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
