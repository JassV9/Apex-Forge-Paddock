package features

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/ingest"
)

// drivenAxle maps a car's drivetrain to the axle the engine looks at for wheelspin. The agent
// does not report the drivetrain yet, so unknown means rear-wheel drive.
func drivenAxle(drivetrain string) contracts.FeaturesRequestDrivenAxle {
	switch drivetrain {
	case "fwd":
		return contracts.FeaturesRequestDrivenAxleFront
	case "awd":
		return contracts.FeaturesRequestDrivenAxleAll
	default:
		return contracts.FeaturesRequestDrivenAxleRear
	}
}

// buildRequest reads what the engine needs for the job's passes: where the corners are, where
// the raw samples are, and the passes themselves.
func buildRequest(ctx context.Context, database *sql.DB, work ingest.FeaturesJob) (int64, contracts.FeaturesRequest, error) {
	var sessionID int64
	var parquetURI, drivetrain string
	var cornerMap []byte
	request := contracts.FeaturesRequest{Passes: []contracts.FeaturePassInput{}}
	err := database.QueryRowContext(ctx,
		`SELECT s.id, s.parquet_uri, t.length_m, t.corner_map, COALESCE(c.drivetrain, '')
		 FROM session s JOIN track t ON t.id = s.track_id JOIN car c ON c.id = s.car_id
		 WHERE s.spool_id = $1`, work.SpoolID).
		Scan(&sessionID, &parquetURI, &request.TrackLengthM, &cornerMap, &drivetrain)
	if err != nil {
		return 0, request, fmt.Errorf("find session %s: %w", work.SpoolID, err)
	}
	var corners []contracts.SpoolCorner
	if err := json.Unmarshal(cornerMap, &corners); err != nil {
		return 0, request, fmt.Errorf("decode corner map of session %s: %w", work.SpoolID, err)
	}
	for _, corner := range corners {
		request.Corners = append(request.Corners, contracts.FeatureCorner{
			Number: corner.Number, ZoneStartM: corner.ZoneStartM, StartM: corner.StartM,
			ApexM: corner.ApexM, EndM: corner.EndM, ZoneEndM: corner.ZoneEndM,
		})
	}
	request.DrivenAxle = drivenAxle(drivetrain)
	withSamples, err := readPasses(ctx, database, sessionID, work.Seqs, &request)
	if err != nil {
		return 0, request, err
	}
	if withSamples {
		request.ParquetDir = &parquetURI
	}
	return sessionID, request, nil
}

// readPasses adds the job's passes to the request and reports whether any has raw samples.
func readPasses(ctx context.Context, database *sql.DB, sessionID int64, seqs []int, request *contracts.FeaturesRequest) (bool, error) {
	wanted := make([]int64, len(seqs))
	for i, seq := range seqs {
		wanted[i] = int64(seq)
	}
	rows, err := database.QueryContext(ctx,
		`SELECT p.seq, c.number, p.duration_s, p.n_samples, p.features->'legacy'
		 FROM corner_pass p JOIN corner c ON c.id = p.corner_id
		 WHERE p.session_id = $1 AND p.seq = ANY($2) ORDER BY p.seq`, sessionID, wanted)
	if err != nil {
		return false, fmt.Errorf("query passes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	withSamples := false
	for rows.Next() {
		var pass contracts.FeaturePassInput
		var samples int
		var legacy []byte
		if err := rows.Scan(&pass.Seq, &pass.Corner, &pass.DurationS, &samples, &legacy); err != nil {
			return false, fmt.Errorf("scan pass: %w", err)
		}
		if len(legacy) > 0 {
			if err := json.Unmarshal(legacy, &pass.Legacy); err != nil {
				return false, fmt.Errorf("decode legacy summary of pass %d: %w", pass.Seq, err)
			}
		}
		withSamples = withSamples || samples > 0
		request.Passes = append(request.Passes, pass)
	}
	return withSamples, rows.Err()
}

const saveFeaturesSQL = `
UPDATE corner_pass SET valid = $3, invalid_reason = $4,
    features = features || jsonb_build_object(
        'metrics', $5::jsonb, 'flags', $6::jsonb, 'speed_class', $7::text)
WHERE session_id = $1 AND seq = $2`

// saveFeatures writes the engine's answer onto the passes, all or nothing.
func saveFeatures(ctx context.Context, database *sql.DB, sessionID int64, passes []contracts.PassFeatures) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit
	for _, pass := range passes {
		metrics, err := json.Marshal(pass.Metrics)
		if err != nil {
			return fmt.Errorf("encode metrics of pass %d: %w", pass.Seq, err)
		}
		flags, err := json.Marshal(pass.Flags)
		if err != nil {
			return fmt.Errorf("encode flags of pass %d: %w", pass.Seq, err)
		}
		_, err = tx.ExecContext(ctx, saveFeaturesSQL, sessionID, pass.Seq, pass.Valid,
			pass.InvalidReason, string(metrics), string(flags), string(pass.SpeedClass))
		if err != nil {
			return fmt.Errorf("save features of pass %d: %w", pass.Seq, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit features: %w", err)
	}
	return nil
}
