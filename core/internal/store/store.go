// Package store owns core's PostgreSQL connection and schema migrations.
package store

import (
	"context"
	"database/sql"
	"fmt"

	// Registers the "pgx" database/sql driver.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/JassV9/Apex-Forge-Paddock/core/migrations"
)

// Open connects to PostgreSQL and checks that the connection works.
func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		closeErr := database.Close()
		return nil, fmt.Errorf("ping database: %w", joinClose(err, closeErr))
	}
	return database, nil
}

// MigrateUp applies every migration that has not been applied yet and returns how many ran.
func MigrateUp(ctx context.Context, database *sql.DB) (int, error) {
	provider, err := newProvider(database)
	if err != nil {
		return 0, err
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	return len(results), nil
}

// MigrateDownTo rolls the schema back to the given version (0 removes everything).
func MigrateDownTo(ctx context.Context, database *sql.DB, version int64) error {
	provider, err := newProvider(database)
	if err != nil {
		return err
	}
	if _, err := provider.DownTo(ctx, version); err != nil {
		return fmt.Errorf("roll back migrations to %d: %w", version, err)
	}
	return nil
}

func newProvider(database *sql.DB) (*goose.Provider, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return provider, nil
}

func joinClose(err, closeErr error) error {
	if closeErr == nil {
		return err
	}
	return fmt.Errorf("%w (and closing failed: %w)", err, closeErr)
}
