package passes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Conditions are stored column by column; a pass has them only when every column is set
// (data imported from Apex Forge v0 recorded none). The block is the experiment block the
// pass was tagged with.
const listPassesSQL = `
SELECT p.seq, c.number, p.lap_in_session, p.t_start, p.duration_s, p.valid, p.invalid_reason,
       p.n_samples, p.setup_hash, p.features,
       CASE WHEN b.id IS NOT NULL THEN jsonb_build_object(
           'experiment_id', b.experiment_id, 'order', b."order", 'lap', COALESCE(p.block_lap, 0))
       END,
       CASE WHEN num_nulls(p.fuel_l, p.tyre_core_c, p.tyre_surface_c, p.tyre_press, p.wear,
                           p.grip, p.air_c, p.road_c) = 0 THEN jsonb_build_object(
           'fuel_l', p.fuel_l, 'tyre_core_c', p.tyre_core_c, 'tyre_surface_c', p.tyre_surface_c,
           'tyre_press_psi', p.tyre_press, 'tyre_wear', p.wear, 'grip', p.grip,
           'air_c', p.air_c, 'road_c', p.road_c)
       END
FROM corner_pass p
JOIN corner c ON c.id = p.corner_id
LEFT JOIN run_block b ON b.id = p.block_id
WHERE p.session_id = $1
ORDER BY p.seq, p.t_start`

const cornerNamesSQL = `
SELECT (m->>'number')::int, COALESCE(m->>'name', '')
FROM session s
JOIN track t ON t.id = s.track_id
CROSS JOIN LATERAL jsonb_array_elements(t.corner_map) AS m
WHERE s.id = $1 AND jsonb_typeof(m->'number') = 'number'`

// storedFeatures is the part of corner_pass.features the dashboard reads. A metric that does
// not apply to a pass is stored as null.
type storedFeatures struct {
	Flags      []string            `json:"flags"`
	Metrics    map[string]*float64 `json:"metrics"`
	SpeedClass string              `json:"speed_class"`
}

// session is what the read path needs of a session row.
type session struct {
	spoolID      string
	trackLengthM float64
}

func (s *Service) session(ctx context.Context, sessionID int64) (session, error) {
	var found session
	err := s.database.QueryRowContext(ctx, `
		SELECT s.spool_id, t.length_m FROM session s JOIN track t ON t.id = s.track_id
		WHERE s.id = $1`, sessionID).Scan(&found.spoolID, &found.trackLengthM)
	if errors.Is(err, sql.ErrNoRows) {
		return found, fmt.Errorf("session %d: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return found, fmt.Errorf("find session %d: %w", sessionID, err)
	}
	return found, nil
}

func (s *Service) passCorner(ctx context.Context, sessionID int64, seq int) (int, error) {
	var corner int
	err := s.database.QueryRowContext(ctx, `
		SELECT c.number FROM corner_pass p JOIN corner c ON c.id = p.corner_id
		WHERE p.session_id = $1 AND p.seq = $2 ORDER BY p.t_start LIMIT 1`, sessionID, seq).Scan(&corner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("session %d has no pass %d: %w", sessionID, seq, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("find pass %d of session %d: %w", seq, sessionID, err)
	}
	return corner, nil
}

func (s *Service) records(ctx context.Context, sessionID int64) ([]contracts.PassRecord, error) {
	rows, err := s.database.QueryContext(ctx, listPassesSQL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query passes of session %d: %w", sessionID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	records := []contracts.PassRecord{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("session %d: %w", sessionID, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read passes of session %d: %w", sessionID, err)
	}
	return records, nil
}

func scanRecord(rows *sql.Rows) (contracts.PassRecord, error) {
	var record contracts.PassRecord
	var lap sql.NullInt64
	var started time.Time
	var reason, hash sql.NullString
	var features, block, conditions []byte
	err := rows.Scan(&record.Seq, &record.Corner, &lap, &started, &record.DurationS, &record.Valid,
		&reason, &record.NSamples, &hash, &features, &block, &conditions)
	if err != nil {
		return record, fmt.Errorf("scan pass: %w", err)
	}
	record.TStart = started.UTC().Format(time.RFC3339Nano)
	record.Lap = nullableInt(lap)
	record.InvalidReason = nullableString(reason)
	record.SetupHash = nullableString(hash)
	if err := decodeOptional(block, &record.Block); err != nil {
		return record, fmt.Errorf("pass %d block: %w", record.Seq, err)
	}
	if err := decodeOptional(conditions, &record.Conditions); err != nil {
		return record, fmt.Errorf("pass %d conditions: %w", record.Seq, err)
	}
	return record, applyFeatures(&record, features)
}

// applyFeatures copies the engine's flags, metrics and corner class; null metrics are left out.
func applyFeatures(record *contracts.PassRecord, raw []byte) error {
	var stored storedFeatures
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("pass %d features: %w", record.Seq, err)
	}
	record.Flags = stored.Flags
	if record.Flags == nil {
		record.Flags = []string{}
	}
	record.Metrics = contracts.PassRecordMetrics{}
	for name, value := range stored.Metrics {
		if value != nil {
			record.Metrics[name] = *value
		}
	}
	if stored.SpeedClass != "" {
		record.SpeedClass = &stored.SpeedClass
	}
	return nil
}

func (s *Service) cornerNames(ctx context.Context, sessionID int64) (map[int]string, error) {
	rows, err := s.database.QueryContext(ctx, cornerNamesSQL, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query corner names of session %d: %w", sessionID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	names := map[int]string{}
	for rows.Next() {
		var number int
		var name string
		if err := rows.Scan(&number, &name); err != nil {
			return nil, fmt.Errorf("scan corner name: %w", err)
		}
		names[number] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read corner names of session %d: %w", sessionID, err)
	}
	return names, nil
}

// decodeOptional leaves *target nil when the column is NULL.
func decodeOptional[T any](raw []byte, target **T) error {
	if raw == nil {
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int64)
	return &converted
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
