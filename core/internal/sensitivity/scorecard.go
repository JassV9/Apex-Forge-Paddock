package sensitivity

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// An experiment's prediction score is the one on its newest engine verdict: an extended
// experiment is judged again, and only the last judgement counts.
const bySourceSQL = `
WITH scored AS (
    SELECT DISTINCT ON (v.experiment_id) v.experiment_id, v.prediction_hits, v.prediction_total
    FROM verdict v WHERE v.decided_by = 'engine'
    ORDER BY v.experiment_id, v.id DESC)
SELECT h.source, count(*), count(s.experiment_id),
       COALESCE(sum(s.prediction_total), 0), COALESCE(sum(s.prediction_hits), 0)
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
LEFT JOIN scored s ON s.experiment_id = e.id
GROUP BY h.source ORDER BY h.source`

const byStateSQL = `SELECT state, count(*) FROM experiment GROUP BY state ORDER BY state`

// Scorecard counts how often the predictions written down before an experiment came true,
// overall and by where the hypothesis came from. It is arithmetic on the ledger, nothing
// more: every experiment counts, the failed ones too.
func (s *Service) Scorecard(ctx context.Context) (contracts.Scorecard, error) {
	card := contracts.Scorecard{}
	var err error
	card.BySource, err = queryAll(ctx, s.database, bySourceSQL, func(rows *sql.Rows) (contracts.SourceScore, error) {
		var score contracts.SourceScore
		var judged int
		err := rows.Scan(&score.Source, &score.Experiments, &judged, &score.PredictionsTotal, &score.PredictionsHit)
		card.Experiments += score.Experiments
		card.Judged += judged
		card.PredictionsTotal += score.PredictionsTotal
		card.PredictionsHit += score.PredictionsHit
		return score, err
	})
	if err != nil {
		return contracts.Scorecard{}, fmt.Errorf("read prediction scores: %w", err)
	}
	card.ByState, err = queryAll(ctx, s.database, byStateSQL, func(rows *sql.Rows) (contracts.StateCount, error) {
		var count contracts.StateCount
		return count, rows.Scan(&count.State, &count.Count)
	})
	if err != nil {
		return contracts.Scorecard{}, fmt.Errorf("count experiments by state: %w", err)
	}
	return card, nil
}
