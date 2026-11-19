package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// The latest verdict, with the finding of the latest engine verdict: the driver's call on a
// trade-off and a training program's revert decide, the finding stays the engine's. A verdict
// judged before ADR 0029 has a finding derived from its decision and reason (verdict_finding).
const verdictSQL = `
SELECT v.decision, v.reason, v.decided_by, v.decided_at, v.prediction_hits, v.prediction_total,
       f.finding_class, f.derived, e.recommendation, array_to_json(e.flags), e.confounders,
       e.driver_adapted, e.engine_version, e.passes_used, e.passes_excluded, e.look, e.margin
FROM verdict v
LEFT JOIN LATERAL (
    SELECT vf.id, vf.finding_class, vf.derived FROM verdict_finding vf
    WHERE vf.experiment_id = v.experiment_id AND vf.decided_by = 'engine'
    ORDER BY vf.id DESC LIMIT 1) f ON true
LEFT JOIN verdict e ON e.id = f.id
WHERE v.experiment_id = $1
ORDER BY v.id DESC LIMIT 1`

// findingRow is the nullable finding part of a verdict row.
type findingRow struct {
	class, recommendation, version sql.NullString
	derived, driverAdapted         sql.NullBool
	flags, confounders             []byte
	used, excluded, look           sql.NullInt64
	margin                         sql.NullFloat64
}

// readVerdict is the latest verdict: an extended experiment is judged more than once, and the
// driver's decision on a trade-off comes after the engine's.
func readVerdict(ctx context.Context, q querier, id int64) (*contracts.VerdictSummary, error) {
	var verdict contracts.VerdictSummary
	var decidedAt time.Time
	var f findingRow
	err := q.QueryRowContext(ctx, verdictSQL, id).Scan(&verdict.Decision, &verdict.Reason,
		&verdict.DecidedBy, &decidedAt, &verdict.PredictionHits, &verdict.PredictionTotal,
		&f.class, &f.derived, &f.recommendation, &f.flags, &f.confounders, &f.driverAdapted,
		&f.version, &f.used, &f.excluded, &f.look, &f.margin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // no verdict yet is not an error
	}
	if err != nil {
		return nil, err
	}
	verdict.DecidedAt = decidedAt.UTC().Format(time.RFC3339)
	if err := f.fill(&verdict); err != nil {
		return nil, fmt.Errorf("decode finding of experiment %d: %w", id, err)
	}
	return &verdict, nil
}

func (f findingRow) fill(v *contracts.VerdictSummary) error {
	if f.class.Valid {
		class := contracts.VerdictSummaryFindingClass(f.class.String)
		v.FindingClass, v.FindingDerived = &class, &f.derived.Bool
	}
	if f.recommendation.Valid {
		recommendation := contracts.VerdictSummaryRecommendation(f.recommendation.String)
		v.Recommendation = &recommendation
	}
	v.DriverAdapted = nullable(f.driverAdapted.Bool, f.driverAdapted.Valid)
	v.EngineVersion = nullable(f.version.String, f.version.Valid)
	v.PassesUsed = nullable(int(f.used.Int64), f.used.Valid)
	v.PassesExcluded = nullable(int(f.excluded.Int64), f.excluded.Valid)
	v.Look = nullable(int(f.look.Int64), f.look.Valid)
	v.Margin = nullable(f.margin.Float64, f.margin.Valid)
	if f.flags != nil {
		if err := json.Unmarshal(f.flags, &v.Flags); err != nil {
			return err
		}
	}
	if f.confounders != nil {
		return json.Unmarshal(f.confounders, &v.Confounders)
	}
	return nil
}

func nullable[T any](value T, valid bool) *T {
	if !valid {
		return nil
	}
	return &value
}
