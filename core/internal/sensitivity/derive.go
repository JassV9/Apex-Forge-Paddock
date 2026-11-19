package sensitivity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/engine"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/experiments"
)

// Replays builds the evaluate request a finished experiment was judged on.
// *experiments.Service implements it.
type Replays interface {
	ReplayRequest(ctx context.Context, id int64) (contracts.EvaluateRequest, error)
}

// derivation is one experiment re-derived by one engine version: its finding and, per pooled
// channel and speed class, the estimate with its block-level standard error and degrees of
// freedom; or why it could not be.
type derivation struct {
	status, finding, reason string
	effects                 []contracts.MeasuredEffect
}

func (d *derivation) measurements(version string) []contracts.Measurement {
	finding := contracts.MeasurementFindingClass(d.finding)
	out := make([]contracts.Measurement, 0, len(d.effects))
	for _, e := range d.effects {
		se, df := e.StandardError, float64(e.Df)
		out = append(out, contracts.Measurement{
			Channel: e.Channel, CornerClass: contracts.MeasurementCornerClass(e.CornerClass),
			Phase: wholeCorner, Estimate: e.Estimate, StandardError: &se, Df: &df,
			NPasses: e.NPasses, FindingClass: &finding, EngineVersion: &version,
		})
	}
	return out
}

const wholeCorner = "corner"

const derivationsSQL = `
SELECT d.experiment_id, d.status, COALESCE(d.finding_class, ''), d.reason, m.channel,
       m.corner_class, m.estimate, m.standard_error, m.df, m.n_passes, m.blocks, m.corners
FROM sensitivity_derivation d
LEFT JOIN sensitivity_measurement m ON m.derivation_id = d.id
WHERE d.engine_version = $1
ORDER BY d.experiment_id, m.id`

func readDerivations(ctx context.Context, database *sql.DB, version string) (map[int]*derivation, error) {
	byExperiment := map[int]*derivation{}
	_, err := queryAll(ctx, database, derivationsSQL, func(rows *sql.Rows) (int, error) {
		var id int
		var d derivation
		var channel, class sql.NullString
		var e contracts.MeasuredEffect
		var estimate, se sql.NullFloat64
		var df, passes, blocks, corners sql.NullInt64
		err := rows.Scan(&id, &d.status, &d.finding, &d.reason, &channel, &class, &estimate, &se,
			&df, &passes, &blocks, &corners)
		if byExperiment[id] == nil {
			byExperiment[id] = &d
		}
		if channel.Valid {
			e.Channel, e.CornerClass = channel.String, contracts.MeasuredEffectCornerClass(class.String)
			e.Estimate, e.StandardError, e.Df = estimate.Float64, se.Float64, int(df.Int64)
			e.NPasses, e.Blocks, e.Corners = int(passes.Int64), int(blocks.Int64), int(corners.Int64)
			byExperiment[id].effects = append(byExperiment[id].effects, e)
		}
		return id, err
	}, version)
	if err != nil {
		return nil, fmt.Errorf("read re-derived experiments: %w", err)
	}
	return byExperiment, nil
}

// derive re-derives every judged experiment that has no derivation at the engine's version yet:
// once per experiment and engine version, never on every read.
func (s *Service) derive(ctx context.Context, version string, level float64) error {
	judged, err := readJudged(ctx, s.database, level)
	if err != nil {
		return err
	}
	done, err := readDerivations(ctx, s.database, version)
	if err != nil {
		return err
	}
	for _, j := range judged {
		if done[j.id] != nil {
			continue
		}
		if err := s.deriveOne(ctx, int64(j.id), version); err != nil {
			return err
		}
	}
	return nil
}

// deriveOne asks the engine to judge the experiment's passes again. An experiment that cannot be
// replayed (not fully driven, features pending) or that the engine refuses is kept as failed,
// with the reason, so it is read from the ledger and marked stale. An engine that is down stops
// the rebuild: nothing is stored for it.
func (s *Service) deriveOne(ctx context.Context, id int64, version string) error {
	request, err := s.replays.ReplayRequest(ctx, id)
	if errors.Is(err, experiments.ErrConflict) || errors.Is(err, experiments.ErrNotFound) {
		return storeDerivation(ctx, s.database, id, version, nil, err.Error())
	}
	if err != nil {
		return fmt.Errorf("replay experiment %d: %w", id, err)
	}
	reply, err := s.engine.Measure(ctx, request)
	if errors.Is(err, engine.ErrRejected) {
		return storeDerivation(ctx, s.database, id, version, nil, err.Error())
	}
	if err != nil {
		return fmt.Errorf("re-derive experiment %d: %w", id, err)
	}
	s.logger.InfoContext(ctx, "experiment re-derived", "experiment_id", id,
		"engine_version", reply.EngineVersion, "finding", reply.FindingClass)
	return storeDerivation(ctx, s.database, id, reply.EngineVersion, &reply, "")
}

const (
	insertDerivationSQL = `
INSERT INTO sensitivity_derivation (experiment_id, engine_version, status, finding_class, reason)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (experiment_id, engine_version) DO NOTHING
RETURNING id`
	insertMeasurementSQL = `
INSERT INTO sensitivity_measurement (derivation_id, channel, corner_class, estimate,
    standard_error, df, n_passes, blocks, corners)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
)

func storeDerivation(ctx context.Context, database *sql.DB, id int64, version string, reply *contracts.SensitivityMeasure, failure string) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin storing the derivation of experiment %d: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // a no-op after Commit
	status, finding, reason := "failed", sql.NullString{}, failure
	if reply != nil {
		status, reason = "derived", reply.Reason
		finding = sql.NullString{String: string(reply.FindingClass), Valid: true}
	}
	var derivationID int64
	err = tx.QueryRowContext(ctx, insertDerivationSQL, id, version, status, finding, reason).Scan(&derivationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // another rebuild stored it first
	}
	if err != nil {
		return fmt.Errorf("store the derivation of experiment %d: %w", id, err)
	}
	for _, e := range effectsOf(reply) {
		if _, err := tx.ExecContext(ctx, insertMeasurementSQL, derivationID, e.Channel, string(e.CornerClass),
			e.Estimate, e.StandardError, e.Df, e.NPasses, e.Blocks, e.Corners); err != nil {
			return fmt.Errorf("store a measurement of experiment %d: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit the derivation of experiment %d: %w", id, err)
	}
	return nil
}

func effectsOf(reply *contracts.SensitivityMeasure) []contracts.MeasuredEffect {
	if reply == nil {
		return nil
	}
	return reply.Effects
}
