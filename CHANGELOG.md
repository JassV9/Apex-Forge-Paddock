# Changelog

One short entry per phase. Dates are when the phase was finished, not accepted.

## Phase 2: measurement (2026-11-12)

- engine: phase segmentation and driver-input metrics ported from v0's `analyze.lua`
  (reproduces v0's own numbers on 14 reference passes), per-phase statistics with confidence,
  and new channels: steering per g, front saturation index, yaw gain, exit speed at +100 m,
  rear slip peak, wheelspin time, speed class.
- engine: measurement model `y ~ arm + covariates + (1 | corner)` by feasible GLS, block
  bootstrap over laps with calibrated intervals, ROPE classification, MDE calculator, robust
  noise model (ADR 0008).
- engine: `POST /features/session` reads a session's Parquet parts with DuckDB; contracts
  `features_request`, `features_response`. The spool's corner map gains each corner's start
  and end.
- core: `cmd/spool2parquet` converts a spool session to Parquet without a database.
- Acceptance simulations: identical setups produce a decision in 0.0 % of 200 experiments
  (v0: 39 %); a 20 ms effect at n = 6, k = 10 is detected in 91.5 %. Speed-binned slip on the
  real session: 7.67 degrees front below 100 km/h, 3.71 above 150.

Known limits: all results are simulated or from v0's stored averages; no real experiment has
been driven. Scatter over all passes of the real session is 82 ms, not the 28 ms of v0's push
passes. Core does not call the engine yet (phase 3).

## Phase 1: capture and ingest (2026-11-11)

- Contracts for the spool: `spool_session`, `spool_pass` (conditions, 24 raw channels as
  column arrays, optional legacy summary for v0 imports), and `session_summary` for the API.
- agent: `PaddockAgent.lua` records every corner pass with its conditions and writes it to the
  spool as numbered chunk files (CSP has no append; ADR 0003 amended). A pass is encoded one
  column per frame. `carlink.lua` now reads raw per-wheel values. Installer:
  `tools/install_agent.py`.
- core: ingest tails the spool, validates each line against the JSON Schema, writes Parquet
  parts and `corner_pass` rows idempotently, and reports OpenTelemetry metrics at
  `GET /metrics`. New tables: car, track, corner, session, corner_pass, spool_chunk.
  `GET /sessions`.
- web: list of recorded sessions.
- Demo dataset: the real v0 FA26 COTA session converted to a spool; `make demo`.
- Tests: 72 agent tests on a synthetic track (restart, teleport, pits, replay, write rate,
  frame cost), Go tests that read what the real agent wrote, database tests in per-package
  schemas.
- Docs: ADR 0007 (ingest libraries and Parquet layout), agent-in-game runbook, phase 1 demo.

Not verified: anything in Assetto Corsa. Not done: OpenTelemetry traces (metrics only).

## Phase 0: skeleton (2026-11-11)

- Repository layout: `agent/`, `core/`, `engine/`, `web/`, `contracts/`, `deploy/`, `docs/`,
  `tools/`.
- `contracts/health.schema.json` with generated Go structs, Pydantic models and TypeScript
  types; `make gen` and a drift check (`make gen-check`).
- core (Go): `GET /health`, environment configuration, PostgreSQL connection, embedded goose
  migrations, first migration (`jobs`).
- engine (Python): FastAPI app with `GET /health`, `mypy --strict`.
- web (React + Vite + TypeScript): a page that reports whether core is reachable, through a
  typed client generated from `docs/api/openapi.yaml`.
- agent: reusable v0 Lua modules, the mock CSP API and the real fixtures copied in
  (ADR 0002); 39 offline tests under LuaJIT, including a check that pure modules make no CSP
  calls.
- Docker Compose stack, Makefile, GitHub Actions CI (generated-code drift, Go, Python, Lua,
  TypeScript, dependency audits, secret scan), pre-commit.
- Docs: architecture, v0 audit, ADRs 0001 to 0006, onboarding, local-dev runbook, demo script.

`make dev` and the Docker images were verified on Docker Desktop after the first commit.
Not verified: anything in Assetto Corsa.
