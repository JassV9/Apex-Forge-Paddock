# ADR 0004: PostgreSQL for structured data, Parquet for raw samples

Status: accepted (2026-11-11)

## Context

Paddock stores two different kinds of data:

- **Relational facts** with integrity rules: cars, setup versions with lineage, experiments,
  predictions, verdicts, sensitivity effects.
- **Raw samples**: 50 to 60 Hz channel arrays per corner pass (up to 333 Hz for Ghost Driver),
  written once, read in bulk by the analysis engine.

## Decision

- **PostgreSQL** (Docker Compose locally) for everything relational, including the `jobs`
  table that carries long-running work to the engine.
- **One Parquet file per session** for raw samples, written by core's ingest.
- **DuckDB inside the engine** for queries over Parquet. It is a library, not a service.

Supporting choices in `core`:

- `pgx` (through `database/sql`) as the PostgreSQL driver: the maintained standard driver.
- `goose` for migrations: plain SQL files with `Up` and `Down`, embedded in the binary so core
  migrates its own database at start. Migrations are reviewed, reversible, and never edited
  after merge.

## Consequences

- Foreign keys and transactions protect the ledger; the experiment state machine can rely on
  them.
- Raw telemetry stays out of the database, so the database stays small and backups are cheap.
- A profile can be re-derived from raw data when the statistical model changes, because the
  Parquet files are immutable.
- No time-series database to run.
- Tests that need a real database run when `PADDOCK_TEST_DATABASE_URL` is set (CI sets it) and
  are skipped otherwise.

## Revisit when

Queries across many sessions become slow in DuckDB over files, or more than one machine needs
to write.
