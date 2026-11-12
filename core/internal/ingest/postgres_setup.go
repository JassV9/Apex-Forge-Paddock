package ingest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
	"github.com/JassV9/Apex-Forge-Paddock/core/internal/setups"
)

// JobFeatures is the kind of job that computes the features of newly ingested passes.
const JobFeatures = "features"

// FeaturesJob is the payload of a JobFeatures job.
type FeaturesJob struct {
	SpoolID string `json:"spool_id"`
	Seqs    []int  `json:"seqs"`
}

const upsertParameterSQL = `
INSERT INTO setup_parameter (car_id, name, label, tab, kind, min, max, step, shown_scale, role, axle, excluded)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (car_id, name) DO UPDATE SET
    label = EXCLUDED.label, tab = EXCLUDED.tab, kind = EXCLUDED.kind, min = EXCLUDED.min,
    max = EXCLUDED.max, step = EXCLUDED.step, shown_scale = EXCLUDED.shown_scale,
    role = EXCLUDED.role, axle = EXCLUDED.axle, excluded = EXCLUDED.excluded`

// saveCatalog stores the car's setup menu. A later session of the same car refreshes it.
func saveCatalog(ctx context.Context, tx *sql.Tx, carID int64, catalog *contracts.SpoolCatalog) error {
	for _, item := range catalog.Parameters {
		_, err := tx.ExecContext(ctx, upsertParameterSQL, carID, item.Name, item.Label, item.Tab,
			optional(item.Kind), item.Min, item.Max, item.Step, item.ShownScale,
			optional(item.Role), optional(item.Axle), item.Excluded)
		if err != nil {
			return fmt.Errorf("save setup parameter %s: %w", item.Name, err)
		}
	}
	return nil
}

// saveSetup stores the setup that is on the car as a version. The same values are the same
// version: a variant that core created for an experiment gets the agent's fingerprint here,
// the moment the agent reports having it on the car.
func saveSetup(ctx context.Context, tx *sql.Tx, carID int64, setup *contracts.SpoolSetup) error {
	values := setups.Values(setup.Values)
	encoded, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode setup values: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO setup_version (car_id, content_hash, agent_hash, "values", created_by)
		 VALUES ($1, $2, $3, $4, 'agent')
		 ON CONFLICT (car_id, content_hash) DO UPDATE SET agent_hash = EXCLUDED.agent_hash`,
		carID, setups.Hash(values), setup.SetupHash, string(encoded))
	if err != nil {
		return fmt.Errorf("save setup %s: %w", setup.SetupHash, err)
	}
	return nil
}

// enqueueFeatures queues the engine job for the chunk's passes (docs/adr/0006: the jobs table
// is the queue).
func enqueueFeatures(ctx context.Context, tx *sql.Tx, chunk Chunk) error {
	if len(chunk.Passes) == 0 {
		return nil
	}
	job := FeaturesJob{SpoolID: chunk.SpoolID}
	for _, pass := range chunk.Passes {
		job.Seqs = append(job.Seqs, pass.Seq)
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode features job: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO jobs (kind, payload) VALUES ($1, $2)`, JobFeatures, string(payload))
	if err != nil {
		return fmt.Errorf("queue features job: %w", err)
	}
	return nil
}
