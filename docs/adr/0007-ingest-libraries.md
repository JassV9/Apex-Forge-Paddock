# ADR 0007: Libraries and file layout for ingest

Status: accepted (2026-11-11)

## Context

Core's ingest must validate every spool line against the contracts, store raw samples as
Parquet, and report metrics. Each needs a library; this records which and why, and how the
Parquet files are laid out.

## Decision

**Schema validation: `santhosh-tekuri/jsonschema` (v6).** Lines are validated against the
JSON Schema files themselves, not only decoded into the generated Go structs. The generated
`UnmarshalJSON` checks required fields and enums but not `additionalProperties` or every
bound, and a reject should name the exact rule that failed. The schemas are embedded in the
binary from `core/internal/ingest/schemas/`, a copy that `make gen` refreshes and
`make gen-check` guards (`go:embed` cannot reach `../contracts`).

**Parquet: `parquet-go/parquet-go`.** Pure Go, so the core binary stays static
(`CGO_ENABLED=0`, distroless image). Files are zstd-compressed.

**Metrics: OpenTelemetry metrics API with the Prometheus exporter.** Instruments are created
from a `metric.Meter` passed in by `main`, so there is no global state and tests read values
through a manual reader. `GET /metrics` on core serves them:
`paddock_ingest_lines_total{outcome}`, `paddock_ingest_rejects_total{reason}`,
`paddock_ingest_lag_seconds`.

**Parquet layout: one directory per session, one part per spool file.**

    <data dir>/sessions/<session-id>/000002.parquet
    <data dir>/sessions/<session-id>/000003.parquet

`session.parquet_uri` is the directory. A session grows while it is driven and a Parquet file
cannot be appended to, so "a Parquet file per session" from the brief becomes a dataset
directory per session; DuckDB reads `dir/*.parquet` as one table. One row is one raw sample;
`seq` and `corner` say which pass it belongs to. A part is written to a temporary name and
renamed, and re-ingesting a spool file rewrites the same part, so ingest stays idempotent.

**Ingest keeps its own SQL** (`core/internal/ingest/postgres.go`) behind the `ingest.Store`
interface; `core/internal/store` owns the connection, the migrations and the API's read
queries.

## Consequences

- Three new direct dependencies in core, each doing one job. `govulncheck` covers them in CI.
- Many small Parquet parts per session. Compacting a finished session into one file is a
  possible later optimisation; nothing needs it yet.
- Traces are not set up yet: only metrics. Spans around ingest, jobs and engine calls come
  with the observability Compose profile.

## Revisit when

Queries over a session's parts become slow, or a second producer of Parquet appears (Ghost
Driver at 333 Hz).
