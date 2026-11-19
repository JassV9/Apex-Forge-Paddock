package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// saveResults appends the measured effects of one engine verdict. Results are never replaced:
// an extended experiment is judged again on all its passes and gets a new set under its new
// verdict, and each set carries the engine version that computed it
// (docs/adr/0030-append-only-results-and-engine-version.md). The view current_result is the
// latest set.
func saveResults(ctx context.Context, tx *sql.Tx, id, verdictID int64, version *string, results []contracts.ChannelResult) error {
	for _, r := range results {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO result (experiment_id, verdict_id, engine_version, channel, role, corner_class,
			                     estimate, ci_low, ci_high, n_passes, effect_class)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, verdictID, version, r.Channel, string(r.Role), string(r.CornerClass), r.Estimate, r.CiLow,
			r.CiHigh, r.NPasses, string(r.EffectClass))
		if err != nil {
			return fmt.Errorf("save result on %s: %w", r.Channel, err)
		}
	}
	return nil
}

// saveResponses appends the engine's response findings of one verdict as result rows of role
// response: the metric is the channel, the regime the corner class
// (docs/adr/0034-vehicle-response-in-verdicts.md). A cell the engine could not estimate has no
// number to store and is left out; the verdict's reply still lists it.
func saveResponses(ctx context.Context, tx *sql.Tx, id, verdictID int64, version *string, findings []contracts.ResponseFinding) error {
	for _, f := range findings {
		if f.Estimate == nil || f.CiLow == nil || f.CiHigh == nil {
			continue
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO result (experiment_id, verdict_id, engine_version, channel, role, phase,
			                     corner_class, estimate, ci_low, ci_high, n_passes, effect_class,
			                     response_finding, expected, margin)
			 VALUES ($1, $2, $3, $4, 'response', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			id, verdictID, version, f.Metric, string(f.Phase), string(f.Regime), *f.Estimate, *f.CiLow,
			*f.CiHigh, f.NPasses, responseEffectClass(*f.CiLow, *f.CiHigh, f.Margin), string(f.Finding),
			string(f.Expected), f.Margin)
		if err != nil {
			return fmt.Errorf("save response %s in %s (%s): %w", f.Metric, f.Phase, f.Regime, err)
		}
	}
	return nil
}

// responseEffectClass is the interval read against the margin as every result row reads it
// (ADR 0008): positive or negative beyond it, negligible inside it, inconclusive otherwise.
func responseEffectClass(low, high, margin float64) string {
	switch {
	case low > margin:
		return "positive"
	case high < -margin:
		return "negative"
	case low > -margin && high < margin:
		return "negligible"
	}
	return "inconclusive"
}

// verdict is one decision about an experiment, by the engine or by the driver. finding is set on
// the engine's verdicts only.
type verdict struct {
	decision  contracts.VerdictSummaryDecision
	reason    string
	hits      int
	total     int
	decidedBy contracts.VerdictSummaryDecidedBy
	finding   *contracts.EvaluateResponse
}

// findingColumns are the engine's finding on a verdict row (ADR 0029); all NULL without one.
func findingColumns(reply *contracts.EvaluateResponse) ([]any, error) {
	if reply == nil {
		return []any{nil, nil, []string{}, nil, nil, nil, nil, nil, nil, nil}, nil
	}
	confounders, err := json.Marshal(reply.Confounders)
	if err != nil {
		return nil, fmt.Errorf("encode confounders: %w", err)
	}
	flags := []string{}
	for _, flag := range reply.Flags {
		flags = append(flags, string(flag))
	}
	return []any{text(reply.FindingClass), text(reply.Recommendation), flags, confounders,
		reply.DriverAdapted, reply.EngineVersion, reply.PassesUsed, reply.PassesExcluded, reply.Look,
		reply.Margin}, nil
}

// text is an optional enum as the database stores it: its string, or NULL.
func text[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	out := string(*value)
	return &out
}

// addVerdict appends a verdict and returns its id. Verdicts are never changed: the driver's call
// on a trade-off is a new row after the engine's.
func addVerdict(ctx context.Context, tx *sql.Tx, id int64, v verdict) (int64, error) {
	finding, err := findingColumns(v.finding)
	if err != nil {
		return 0, err
	}
	args := append([]any{id, string(v.decision), v.reason, v.hits, v.total, string(v.decidedBy)}, finding...)
	var verdictID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO verdict (experiment_id, decision, reason, prediction_hits, prediction_total,
		                      decided_by, finding_class, recommendation, flags, confounders,
		                      driver_adapted, engine_version, passes_used, passes_excluded, look,
		                      margin, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		         clock_timestamp())
		 RETURNING id`, args...).Scan(&verdictID)
	if err != nil {
		return 0, fmt.Errorf("save verdict of experiment %d: %w", id, err)
	}
	return verdictID, nil
}

// driverVerdict records a decision the driver made; the prediction score of the engine's
// last verdict is carried over, so it stays on the latest row.
func driverVerdict(ctx context.Context, tx *sql.Tx, id int64, decision contracts.VerdictSummaryDecision, reason string) error {
	return decidedVerdict(ctx, tx, id, decision, reason, contracts.VerdictSummaryDecidedByDriver)
}

// decidedVerdict records a decision taken after the engine's verdict, by whom it was taken,
// keeping the prediction score of the engine's verdict. The finding stays the engine's: it is
// read from the latest engine verdict, never copied.
func decidedVerdict(ctx context.Context, tx *sql.Tx, id int64, decision contracts.VerdictSummaryDecision,
	reason string, by contracts.VerdictSummaryDecidedBy) error {
	last, err := readVerdict(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("read last verdict of experiment %d: %w", id, err)
	}
	v := verdict{decision: decision, reason: reason, decidedBy: by}
	if last != nil {
		v.hits, v.total = last.PredictionHits, last.PredictionTotal
	}
	_, err = addVerdict(ctx, tx, id, v)
	return err
}
