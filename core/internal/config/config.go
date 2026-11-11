// Package config reads core's settings from environment variables.
package config

import (
	"errors"
	"fmt"
)

const (
	envHTTPAddr    = "PADDOCK_HTTP_ADDR"
	envDatabaseURL = "PADDOCK_DATABASE_URL"

	defaultHTTPAddr = ":8080"
)

// ErrMissing reports a required variable that is unset or empty.
var ErrMissing = errors.New("required environment variable is not set")

// Config is everything core needs to start.
type Config struct {
	// HTTPAddr is the listen address of the REST API, for example ":8080".
	HTTPAddr string
	// DatabaseURL is a PostgreSQL connection URL.
	DatabaseURL string
}

// FromEnv builds a Config. getenv is os.Getenv in production and a map lookup in tests.
func FromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:    getenv(envHTTPAddr),
		DatabaseURL: getenv(envDatabaseURL),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("%s: %w", envDatabaseURL, ErrMissing)
	}
	return cfg, nil
}
