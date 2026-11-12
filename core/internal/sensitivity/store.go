package sensitivity

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

const insertSQL = `
INSERT INTO sensitivity_effect (car_id, parameter_id, direction, clicks, channel, corner_class,
    estimate, ci_low, ci_high, n_passes, effect_class, nonlinear, note, is_prior,
    prior_source_car_id, experiment_ids, model_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::bigint[], $17)`

// save replaces every stored row with the profiles just built, in one transaction. The lock
// keeps two rebuilds from interleaving their rows.
func save(ctx context.Context, database *sql.DB, cars []car, profiles []built) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin saving sensitivity profiles: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	for _, clear := range []string{
		`LOCK TABLE sensitivity_effect IN SHARE ROW EXCLUSIVE MODE`,
		`DELETE FROM sensitivity_effect`,
	} {
		if _, err := tx.ExecContext(ctx, clear); err != nil {
			return fmt.Errorf("clear sensitivity profiles: %w", err)
		}
	}
	lenders := map[string]int64{}
	for _, c := range cars {
		lenders[c.label] = c.id
	}
	for _, profile := range profiles {
		for _, effect := range profile.reply.Effects {
			if err := insert(ctx, tx, profile, effect, lenders); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sensitivity profiles: %w", err)
	}
	return nil
}

func insert(ctx context.Context, tx *sql.Tx, profile built, effect contracts.SensitivityEffect, lenders map[string]int64) error {
	k, known := profile.car.knob(effect.Parameter)
	if !known || effect.Clicks == 0 {
		return fmt.Errorf("the engine returned an effect of %q by %d clicks, which car %d cannot have",
			effect.Parameter, effect.Clicks, profile.car.id)
	}
	direction, clicks := "increase", effect.Clicks
	if clicks < 0 {
		direction, clicks = "decrease", -clicks
	}
	var lender sql.NullInt64
	if effect.IsPrior {
		lender.Int64, lender.Valid = lenders[effect.PriorSource]
	}
	_, err := tx.ExecContext(ctx, insertSQL, profile.car.id, k.id, direction, clicks, effect.Channel,
		string(effect.CornerClass), effect.Estimate, effect.CiLow, effect.CiHigh, effect.NPasses,
		string(effect.EffectClass), effect.Nonlinear, effect.Note, effect.IsPrior, lender,
		idList(effect.ExperimentIds), profile.reply.ModelVersion)
	if err != nil {
		return fmt.Errorf("save effect of %s on car %d: %w", effect.Parameter, profile.car.id, err)
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
