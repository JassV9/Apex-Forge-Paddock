// Package config reads core's settings from environment variables.
package config

import (
	"errors"
	"fmt"
)

const (
	envHTTPAddr    = "PADDOCK_HTTP_ADDR"
	envDatabaseURL = "PADDOCK_DATABASE_URL"
	envSpoolDir    = "PADDOCK_SPOOL_DIR"
	envDataDir     = "PADDOCK_DATA_DIR"
	envEngineURL   = "PADDOCK_ENGINE_URL"
	envOutboxDir   = "PADDOCK_OUTBOX_DIR"

	defaultHTTPAddr = ":8080"
	defaultDataDir  = "data"
)

// ErrMissing reports a required variable that is unset or empty.
var ErrMissing = errors.New("required environment variable is not set")

// Config is everything core needs to start.
type Config struct {
	// HTTPAddr is the listen address of the REST API, for example ":8080".
	HTTPAddr string
	// DatabaseURL is a PostgreSQL connection URL.
	DatabaseURL string
	// SpoolDir is where the in-game agent writes pass records, normally
	// Documents/Assetto Corsa/paddock/spool. Empty turns ingest off.
	SpoolDir string
	// DataDir is where core keeps files it owns (Parquet files of raw samples).
	DataDir string
	// EngineURL is where the analysis engine listens, for example http://engine:8000. Empty
	// leaves pass features uncomputed and experiments unable to be planned or judged.
	EngineURL string
	// OutboxDir is where core writes the run plan and setup files for the in-game agent,
	// normally Documents/Assetto Corsa/paddock/outbox. Empty writes nothing.
	OutboxDir string
}

// FromEnv builds a Config. getenv is os.Getenv in production and a map lookup in tests.
func FromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:    getenv(envHTTPAddr),
		DatabaseURL: getenv(envDatabaseURL),
		SpoolDir:    getenv(envSpoolDir),
		DataDir:     getenv(envDataDir),
		EngineURL:   getenv(envEngineURL),
		OutboxDir:   getenv(envOutboxDir),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("%s: %w", envDatabaseURL, ErrMissing)
	}
	return cfg, nil
}
