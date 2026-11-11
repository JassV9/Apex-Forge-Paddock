# Architecture

Apex Forge: Paddock is a **local-first modular monolith**: three processes and one database on
one Windows PC, plus a thin agent inside Assetto Corsa. The reasoning is in the ADRs
(`docs/adr/`); the full design is `docs/design.md`; the brief is `docs/MASTER_PROMPT.md`.

This file describes the target shape and marks what exists today. Changing the shape needs an
ADR.

## Processes and boundaries

```mermaid
flowchart LR
  subgraph AC["Assetto Corsa + CSP"]
    AG["agent (CSP Lua)<br/>capture passes, apply setups,<br/>show run plan"]
  end
  SP[("spool dir<br/>one .jsonl per session")]
  subgraph CORE["core (Go)"]
    ING[ingest]
    EXP[experiments]
    SET[setups]
    API[REST API]
  end
  subgraph ENG["engine (Python)"]
    FEAT[features]
    STAT[stats]
    DIAG[diagnosis]
    SENS[sensitivity / optimize]
  end
  PG[(PostgreSQL)]
  PQ[(Parquet<br/>raw samples)]
  UI["web (React + TS)"]

  AG -- "io.saveAsync, one line per corner pass" --> SP
  SP --> ING
  ING --> PQ
  ING --> PG
  SET -- "setup .ini + runplan.json" --> AG
  EXP <--> PG
  EXP -- "HTTP/JSON + jobs table" --> ENG
  ENG --> PQ
  API <--> UI
```

| Boundary | Transport | Contract |
| --- | --- | --- |
| agent to core | Files: the agent appends JSON lines to `Documents/Assetto Corsa/paddock/spool/<session-id>.jsonl`; core tails the directory ([ADR 0003](adr/0003-spool-transport.md)) | `contracts/*.schema.json` |
| core to agent | Files: setup `.ini` files and `runplan.json`, read by the agent at most once per second, in the pits only | `contracts/*.schema.json` |
| core to engine | HTTP/JSON on localhost; long work goes through the `jobs` table, claimed with `SELECT ... FOR UPDATE SKIP LOCKED` ([ADR 0006](adr/0006-no-kafka-no-kubernetes.md)) | `contracts/*.schema.json` |
| web to core | REST | `docs/api/openapi.yaml` |

Storage: PostgreSQL for structured data, one Parquet file per session for raw samples, DuckDB
inside the engine for queries over Parquet ([ADR 0004](adr/0004-postgres-and-parquet.md)).

## Who owns what

| Module | Language | Owns | Must never |
| --- | --- | --- | --- |
| `agent/` | CSP Lua | Corner map lookup, pass capture with conditions, applying setup values with read-back and rollback, showing the run plan | Judge a setup, call an LLM, write anything but the spool file while driving |
| `core/internal/ingest` | Go | Tailing the spool, schema validation, Parquet files and `corner_pass` rows | Compute engineering metrics |
| `core/internal/setups` | Go | Parameter catalog, setup versions, `.ini` writer, diffing | Accept a value off the step grid or out of range |
| `core/internal/experiments` | Go | Experiment state machine, run plans, verdict ledger, rollback | Do statistics (it asks the engine) |
| `core/internal/api` | Go | REST API for the dashboard | Hold logic that belongs to another package |
| `core/internal/store` | Go | Database connection and migrations | — |
| `engine/` | Python | Features, phases, noise model, verdict statistics, diagnosis, sensitivity, optimizer, LLM explanations | Talk to the game; let an LLM number enter the ledger |
| `web/` | TypeScript | Dashboard | Hold business logic |
| `ghost/` | Go | Ghost Driver research (phase 6, optional, behind a flag) | Run without its safety envelope |

Rules that hold everywhere:

- **Facts come from deterministic code.** The LLM explains and ranks; it never produces a number
  that is stored.
- **A recommendation is a hypothesis with a prediction**, tested as an interleaved A-B-A-B
  experiment against a re-driven baseline. Failed experiments are kept.
- **In-game agent:** pure modules (`agent/src/core`) make no `ac.*` calls; file writes use
  `io.saveAsync`, only on change, at most 5 Hz; nothing heavy runs on the game thread; a user's
  setup file is never overwritten.

## Contracts and generated code

`contracts/*.schema.json` is the single source of truth for every type that crosses a process
boundary. `docs/api/openapi.yaml` describes core's REST API and refers to those schemas.

| Source | Generator | Output (never hand-edited) |
| --- | --- | --- |
| `contracts/*.schema.json` | `go-jsonschema` | `core/internal/contracts/contracts.go` |
| `contracts/*.schema.json` | `datamodel-code-generator` | `engine/paddock_engine/contracts/` |
| `docs/api/openapi.yaml` | `openapi-typescript` | `web/src/api/schema.d.ts` |

`make gen` regenerates all three. `make gen-check` (run in CI) fails when the committed output
differs from what the generators produce.

## What exists today (phase 0)

| Piece | State |
| --- | --- |
| `contracts/health.schema.json` and the three generators | Working, checked in CI |
| core: `GET /health`, configuration, PostgreSQL connection, migration runner, `jobs` table | Working, tested (migration test runs against a real PostgreSQL) |
| engine: `GET /health` | Working, tested, `mypy --strict` |
| web: a page that reports whether core is reachable | Working, tested |
| agent: v0 modules copied under `agent/src`, with the LuaJIT test harness and v0's catalog and reply tests | Tests pass offline; **unverified in game** (nothing in this repo has run in Assetto Corsa yet) |
| ingest, setups, experiments, features, stats, diagnosis, sensitivity, optimizer, dashboard pages, ghost | Not started (phases 1 to 6) |

The data model (`docs/MASTER_PROMPT.md` section 3) is introduced table by table, each in the
phase that first uses it. Phase 0 has one migration, `jobs`.

## Configuration and ports

| Process | Port | Environment variables |
| --- | --- | --- |
| core | 8080 | `PADDOCK_DATABASE_URL` (required), `PADDOCK_HTTP_ADDR` (default `:8080`) |
| engine | 8000 | — |
| web (dev server) | 5173 | `PADDOCK_CORE_URL` (default `http://localhost:8080`), target of the `/api` proxy |
| PostgreSQL | 5432 | Set in `deploy/docker-compose.yml` |

Every port is bound to `127.0.0.1`. Nothing needs a cloud service to run locally.
