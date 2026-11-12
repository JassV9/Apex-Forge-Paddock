package experiments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JassV9/Apex-Forge-Paddock/core/internal/contracts"
)

// querier is what *sql.DB and *sql.Tx have in common.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// queryAll collects every row of a query. The result is never nil: an empty list is [] in
// JSON, for the API and for the engine alike.
func queryAll[T any](ctx context.Context, q querier, query string, scan func(*sql.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows.Err below reports any failure
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// record is what the service needs to know about an experiment to act on it.
type record struct {
	id              int64
	kind            contracts.ExperimentKind
	state           State
	carID           int64
	trackID         int64
	carACID         string
	baselineVersion int64
	variantVersion  int64
	extensions      int
	test            contracts.SuccessTest
	guardrails      []contracts.GuardrailSpec
}

const recordSQL = `
SELECT e.id, e.kind, e.state, h.car_id, h.track_id, c.ac_id, e.baseline_version_id,
       (SELECT a.setup_version_id FROM experiment_arm a
        WHERE a.experiment_id = e.id AND NOT a.is_baseline),
       e.extensions, e.success_test, e.guardrails
FROM experiment e
JOIN hypothesis h ON h.id = e.hypothesis_id
JOIN car c ON c.id = h.car_id
WHERE e.id = $1`

// loadRecord reads an experiment. In a transaction pass lock = true: the row stays locked
// until the transaction ends, so two actions on one experiment never interleave.
func loadRecord(ctx context.Context, q querier, id int64, lock bool) (record, error) {
	query := recordSQL
	if lock {
		query += ` FOR UPDATE OF e`
	}
	var exp record
	var test, guardrails []byte
	err := q.QueryRowContext(ctx, query, id).Scan(&exp.id, &exp.kind, &exp.state, &exp.carID,
		&exp.trackID, &exp.carACID, &exp.baselineVersion, &exp.variantVersion, &exp.extensions,
		&test, &guardrails)
	if errors.Is(err, sql.ErrNoRows) {
		return record{}, fmt.Errorf("experiment %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return record{}, fmt.Errorf("read experiment %d: %w", id, err)
	}
	if err := json.Unmarshal(test, &exp.test); err != nil {
		return record{}, fmt.Errorf("decode success test of experiment %d: %w", id, err)
	}
	if err := json.Unmarshal(guardrails, &exp.guardrails); err != nil {
		return record{}, fmt.Errorf("decode guardrails of experiment %d: %w", id, err)
	}
	return exp, nil
}

// move changes an experiment's state and writes the event that makes the change visible in
// the ledger. It refuses moves the state machine does not allow.
func move(ctx context.Context, tx *sql.Tx, id int64, from, to State, reason string) error {
	if !CanMove(from, to) {
		return fmt.Errorf("experiment %d is %s and cannot become %s: %w", id, from, to, ErrConflict)
	}
	_, err := tx.ExecContext(ctx, `UPDATE experiment SET state = $2 WHERE id = $1`, id, string(to))
	if err != nil {
		return fmt.Errorf("set experiment %d to %s: %w", id, to, err)
	}
	return addEvent(ctx, tx, id, from, to, reason)
}

func addEvent(ctx context.Context, tx *sql.Tx, id int64, from, to State, reason string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO experiment_event (experiment_id, from_state, to_state, reason, at)
		 VALUES ($1, $2, $3, $4, clock_timestamp())`, id, string(from), string(to), reason)
	if err != nil {
		return fmt.Errorf("log %s -> %s of experiment %d: %w", from, to, id, err)
	}
	return nil
}

// inTx runs work in one transaction on the locked experiment.
func inTx(ctx context.Context, database *sql.DB, id int64, work func(tx *sql.Tx, exp record) error) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit
	exp, err := loadRecord(ctx, tx, id, true)
	if err != nil {
		return err
	}
	if err := work(tx, exp); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
