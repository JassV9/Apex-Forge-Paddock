package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// Reader answers the API's read-only questions about stored data.
type Reader struct {
	database *sql.DB
}

// NewReader wraps an open database.
func NewReader(database *sql.DB) *Reader {
	return &Reader{database: database}
}

const listSessionsSQL = `
SELECT s.id, s.spool_id, c.name, t.name, t.layout, s.started_at, s.source,
       count(p.id), count(DISTINCT p.corner_id)
FROM session s
JOIN car c ON c.id = s.car_id
JOIN track t ON t.id = s.track_id
LEFT JOIN corner_pass p ON p.session_id = s.id
GROUP BY s.id, c.name, t.name, t.layout
ORDER BY s.started_at DESC, s.id DESC`

// ListSessions returns every session, newest first, with its pass and corner counts.
func (r *Reader) ListSessions(ctx context.Context) ([]contracts.SessionSummary, error) {
	rows, err := r.database.QueryContext(ctx, listSessionsSQL)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	var sessions []contracts.SessionSummary
	for rows.Next() {
		var session contracts.SessionSummary
		var started time.Time
		var source string
		err := rows.Scan(&session.Id, &session.SpoolId, &session.CarName, &session.TrackName,
			&session.Layout, &started, &source, &session.Passes, &session.Corners)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		session.StartedAt = started.UTC().Format(time.RFC3339)
		session.Source = contracts.SessionSummarySource(source)
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sessions: %w", err)
	}
	return sessions, nil
}
