package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// PostgresStore is the Store backed by core's database.
type PostgresStore struct {
	database *sql.DB
}

// NewPostgresStore wraps an open database.
func NewPostgresStore(database *sql.DB) *PostgresStore {
	return &PostgresStore{database: database}
}

// sessionRef is the stored session a chunk belongs to.
type sessionRef struct {
	id      int64
	carID   int64
	trackID int64
}

// DoneChunks implements Store.
func (p *PostgresStore) DoneChunks(ctx context.Context, spoolID string) (map[string]bool, error) {
	rows, err := p.database.QueryContext(ctx,
		`SELECT file_name FROM spool_chunk WHERE spool_id = $1`, spoolID)
	if err != nil {
		return nil, fmt.Errorf("query ingested files: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	done := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan ingested file: %w", err)
		}
		done[name] = true
	}
	return done, rows.Err()
}

// StoreChunk implements Store: the session, its setups and passes, the job that computes
// their features and the "file ingested" mark are written in one transaction, so a crash can
// never leave a file half ingested.
func (p *PostgresStore) StoreChunk(ctx context.Context, chunk Chunk) (ChunkResult, error) {
	tx, err := p.database.BeginTx(ctx, nil)
	if err != nil {
		return ChunkResult{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	result := ChunkResult{Rejects: map[string]int{}}
	if chunk.Header != nil {
		if err := saveSession(ctx, tx, chunk.Header, chunk.ParquetURI); err != nil {
			return ChunkResult{}, err
		}
	}
	if err := saveContent(ctx, tx, chunk, &result); err != nil {
		return ChunkResult{}, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO spool_chunk (spool_id, file_name, lines, rejected) VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		chunk.SpoolID, chunk.FileName, chunk.Lines, total(chunk.Rejects)+total(result.Rejects))
	if err != nil {
		return ChunkResult{}, fmt.Errorf("mark file ingested: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ChunkResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// saveContent stores everything in the chunk that hangs off the session.
func saveContent(ctx context.Context, tx *sql.Tx, chunk Chunk, result *ChunkResult) error {
	dependents := len(chunk.Setups) + len(chunk.Passes)
	if chunk.Catalog != nil {
		dependents++
	}
	if dependents == 0 {
		return nil
	}
	session, found, err := findSession(ctx, tx, chunk.SpoolID)
	if err != nil {
		return err
	}
	if !found {
		result.Rejects[ReasonUnknownSession] += dependents
		return nil
	}
	if chunk.Catalog != nil {
		if err := saveCatalog(ctx, tx, session.carID, chunk.Catalog); err != nil {
			return err
		}
	}
	for _, setup := range chunk.Setups {
		if err := saveSetup(ctx, tx, session.carID, setup); err != nil {
			return err
		}
	}
	if err := savePasses(ctx, tx, session, chunk.Passes, result); err != nil {
		return err
	}
	return enqueueFeatures(ctx, tx, chunk)
}

func findSession(ctx context.Context, tx *sql.Tx, spoolID string) (sessionRef, bool, error) {
	var session sessionRef
	err := tx.QueryRowContext(ctx,
		`SELECT id, car_id, track_id FROM session WHERE spool_id = $1`, spoolID).
		Scan(&session.id, &session.carID, &session.trackID)
	if errors.Is(err, sql.ErrNoRows) {
		return sessionRef{}, false, nil
	}
	if err != nil {
		return sessionRef{}, false, fmt.Errorf("find session: %w", err)
	}
	return session, true, nil
}

func saveSession(ctx context.Context, tx *sql.Tx, s *contracts.SpoolSession, parquetURI string) error {
	var carID, trackID int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO car (ac_id, name) VALUES ($1, $2)
		 ON CONFLICT (ac_id) DO UPDATE SET name = EXCLUDED.name RETURNING id`,
		s.Car.AcId, s.Car.Name).Scan(&carID)
	if err != nil {
		return fmt.Errorf("save car: %w", err)
	}
	cornerMap, err := json.Marshal(s.Track.Corners)
	if err != nil {
		return fmt.Errorf("encode corner map: %w", err)
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO track (ac_id, layout, name, length_m, corner_map, corner_map_version)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (ac_id, layout, corner_map_version) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		s.Track.AcId, s.Track.Layout, s.Track.Name, s.Track.LengthM, string(cornerMap),
		s.Track.CornerMapVersion).Scan(&trackID)
	if err != nil {
		return fmt.Errorf("save track: %w", err)
	}
	for _, corner := range s.Track.Corners {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO corner (track_id, number, apex_s, radius_m) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (track_id, number) DO NOTHING`,
			trackID, corner.Number, corner.ApexM, corner.RadiusM)
		if err != nil {
			return fmt.Errorf("save corner %d: %w", corner.Number, err)
		}
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO session (spool_id, car_id, track_id, started_at, source, agent_version, parquet_uri)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (spool_id) DO NOTHING`,
		s.SessionId, carID, trackID, time.UnixMilli(int64(s.StartedAtMs)).UTC(),
		string(s.Source), s.AgentVersion, parquetURI)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func savePasses(ctx context.Context, tx *sql.Tx, session sessionRef, passes []*contracts.SpoolPass, result *ChunkResult) error {
	if len(passes) == 0 {
		return nil
	}
	corners, err := cornerIDs(ctx, tx, session.trackID)
	if err != nil {
		return err
	}
	for _, pass := range passes {
		cornerID, known := corners[pass.Corner]
		if !known {
			result.Rejects[ReasonUnknownCorner]++
			continue
		}
		inserted, err := insertPass(ctx, tx, session, cornerID, pass)
		if err != nil {
			return fmt.Errorf("save pass %d: %w", pass.Seq, err)
		}
		if inserted {
			result.Inserted++
		} else {
			result.Duplicates++
		}
	}
	return nil
}

func cornerIDs(ctx context.Context, tx *sql.Tx, trackID int64) (map[int]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT number, id FROM corner WHERE track_id = $1`, trackID)
	if err != nil {
		return nil, fmt.Errorf("query corners: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	ids := map[int]int64{}
	for rows.Next() {
		var number int
		var id int64
		if err := rows.Scan(&number, &id); err != nil {
			return nil, fmt.Errorf("scan corner: %w", err)
		}
		ids[number] = id
	}
	return ids, rows.Err()
}
