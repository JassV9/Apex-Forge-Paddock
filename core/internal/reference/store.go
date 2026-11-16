package reference

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// The passes a comparison counts: valid ones, never the out-lap of an experiment block.
const passColumnsSQL = `
SELECT p.session_id, p.seq, c.number, p.duration_s, COALESCE(p.setup_hash, ''),
       p.features->'metrics'
FROM corner_pass p
JOIN corner c ON c.id = p.corner_id
JOIN session s ON s.id = p.session_id
WHERE p.valid AND (p.block_id IS NULL OR COALESCE(p.block_lap, 0) > 0)`

const sessionPassesSQL = passColumnsSQL + `
  AND p.session_id = $1
ORDER BY p.seq, p.t_start`

const aiPassesSQL = passColumnsSQL + `
  AND s.source = 'ai' AND s.car_id = $1 AND s.track_id = $2 AND s.id <> $3
ORDER BY s.id, p.seq`

const aiSetupPassesSQL = passColumnsSQL + `
  AND s.source = 'ai' AND s.car_id = $1 AND s.track_id = $2 AND p.setup_hash = $3
ORDER BY s.id, p.seq`

const cornerNamesSQL = `
SELECT (m->>'number')::int, COALESCE(m->>'name', '')
FROM track t
CROSS JOIN LATERAL jsonb_array_elements(t.corner_map) AS m
WHERE t.id = $1 AND jsonb_typeof(m->'number') = 'number'`

// The newest version of a setup's values under one agent fingerprint.
const setupValuesSQL = `
SELECT DISTINCT ON (agent_hash) agent_hash, "values"
FROM setup_version
WHERE car_id = $1 AND agent_hash IS NOT NULL
ORDER BY agent_hash, id DESC`

// sessionInfo is what the comparison needs of a session row.
type sessionInfo struct {
	carID, trackID     int64
	car, track, layout string
}

func (s *Service) session(ctx context.Context, sessionID int64) (sessionInfo, error) {
	var info sessionInfo
	err := s.database.QueryRowContext(ctx, `
		SELECT s.car_id, s.track_id, c.ac_id, t.ac_id, t.layout
		FROM session s JOIN car c ON c.id = s.car_id JOIN track t ON t.id = s.track_id
		WHERE s.id = $1`, sessionID).Scan(&info.carID, &info.trackID, &info.car, &info.track, &info.layout)
	if errors.Is(err, sql.ErrNoRows) {
		return info, fmt.Errorf("session %d: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return info, fmt.Errorf("find session %d: %w", sessionID, err)
	}
	return info, nil
}

// carAndTrack finds the car and the newest corner map of a track layout by their game ids;
// ok is false when core has never seen them together.
func (s *Service) carAndTrack(ctx context.Context, car, track, layout string) (int64, int64, bool, error) {
	var carID, trackID int64
	err := s.database.QueryRowContext(ctx, `
		SELECT c.id, t.id FROM car c CROSS JOIN track t
		WHERE c.ac_id = $1 AND t.ac_id = $2 AND t.layout = $3
		ORDER BY t.corner_map_version DESC LIMIT 1`, car, track, layout).Scan(&carID, &trackID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("find car %s on %s %s: %w", car, track, layout, err)
	}
	return carID, trackID, true, nil
}

func (s *Service) passes(ctx context.Context, query string, args ...any) ([]Pass, error) {
	rows, err := s.database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query passes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var found []Pass
	for rows.Next() {
		pass, err := scanPass(rows)
		if err != nil {
			return nil, err
		}
		found = append(found, pass)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read passes: %w", err)
	}
	return found, nil
}

// scanPass reads one row; a metric stored as null (it does not apply) is left out.
func scanPass(rows *sql.Rows) (Pass, error) {
	var pass Pass
	var metrics []byte
	err := rows.Scan(&pass.SessionID, &pass.Seq, &pass.Corner, &pass.DurationS, &pass.SetupHash, &metrics)
	if err != nil {
		return pass, fmt.Errorf("scan pass: %w", err)
	}
	stored := map[string]*float64{}
	if metrics != nil {
		if err := json.Unmarshal(metrics, &stored); err != nil {
			return pass, fmt.Errorf("session %d pass %d metrics: %w", pass.SessionID, pass.Seq, err)
		}
	}
	pass.Metrics = map[string]float64{}
	for name, value := range stored {
		if value != nil {
			pass.Metrics[name] = *value
		}
	}
	return pass, nil
}

func (s *Service) cornerNames(ctx context.Context, trackID int64) (map[int]string, error) {
	rows, err := s.database.QueryContext(ctx, cornerNamesSQL, trackID)
	if err != nil {
		return nil, fmt.Errorf("query corner names of track %d: %w", trackID, err)
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
		return nil, fmt.Errorf("read corner names of track %d: %w", trackID, err)
	}
	return names, nil
}

func (s *Service) setupValues(ctx context.Context, carID int64) (map[string]setups.Values, error) {
	rows, err := s.database.QueryContext(ctx, setupValuesSQL, carID)
	if err != nil {
		return nil, fmt.Errorf("query setups of car %d: %w", carID, err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	values := map[string]setups.Values{}
	for rows.Next() {
		var hash string
		var raw []byte
		if err := rows.Scan(&hash, &raw); err != nil {
			return nil, fmt.Errorf("scan setup: %w", err)
		}
		var decoded setups.Values
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("setup %s values: %w", hash, err)
		}
		values[hash] = decoded
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read setups of car %d: %w", carID, err)
	}
	return values, nil
}
