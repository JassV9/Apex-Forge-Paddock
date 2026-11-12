# Changelog

One short entry per phase. Dates are when the phase was finished, not accepted.

## Phase 4: diagnosis and explanation (2026-11-12)

- Contracts: `diagnose_request`, `diagnosis`, `explain_reply`; `experiment` gains `source`
  and `experiment_create` an optional `hypothesis_key`. OpenAPI with the diagnosis routes.
- engine: `POST /diagnose`. An evidence pack (metric means per corner class); symptoms from
  v0's thresholds; a driver-input and a conditions alternative on every hypothesis, with
  driver input marked supported by front saturation or by front slip that depends on corner
  speed while rear slip does not; v0's rule table as candidate experiments on the car's own
  setup items, with a reason for every knob that cannot be tested.
- engine: explanations. `NullProvider` (templates), `GeminiProvider`, `OpenRouterProvider`
  behind one interface; a reply that ranks an unknown hypothesis, cites unknown evidence,
  names an identifier or states a number that is not in the pack is rejected and the
  template text is shown with the reason. Keys from the environment or the OS keyring.
- core: `diagnosis` table (migration 00004), `POST` and `GET /sessions/{id}/diagnosis`. An
  experiment proposed from a candidate stores where its hypothesis came from, with the
  evidence and alternatives as shown.
- web: diagnosis page per session; "Test this" fills the propose form; the experiment page
  says where the hypothesis came from.
- The end-to-end test now diagnoses the baseline session first. It found that a session
  could be diagnosed while its features were still being computed, which stored an empty
  diagnosis as if it were the whole one; core now answers 409 until they are done.

Not verified: anything in Assetto Corsa; any real call to Gemini or OpenRouter (recorded
transport only). The driver-input thresholds are judgement. Candidates have a direction and
no size.

## Phase 3: experiments (2026-11-12)

- Contracts: setup catalog and setup lines in the spool, `runplan`, `experiment` with its
  create and decision requests, `setup_item`, and the engine's plan and evaluate requests and
  responses. OpenAPI 0.3.0 with the experiment routes.
- engine: `POST /plan` (what an A-B-A-B plan can detect) and `POST /evaluate` (effect and
  interval per channel, guardrail checks, prediction score, decision). A channel that never
  moves has an effect of exactly zero instead of failing the fit.
- core: setup catalog and content-hashed setup versions; features worker on the `jobs` table;
  experiment state machine with an event per move; outbox (run plan and setup files, never
  overwritten); champion per car and track, rollback, and a re-check against the original
  after three accepted experiments. Routes: `/experiments`, `/experiments/{id}`, its actions
  and decision, `/sessions/{id}/setup`.
- agent: reads the run plan in the pits, loads a block's setup through the setup spinners
  with read-back and rollback, tags passes with block and lap only while that setup is on the
  car, and keeps its position across restarts.
- web: propose form, experiment page (test, blind setups, run plan, results, verdict, full
  history) and ledger.
- End-to-end test (`make test-e2e`, also in CI): the Lua agent on the mocked game, core and
  the engine through three experiments. It found that a new experiment's baseline was taken
  from the latest pass, which can be a rejected variant; every experiment now starts from the
  champion (ADR 0009).
- Compose: core calls the engine and writes the outbox; the engine reads core's Parquet
  volume.

Not verified: anything in Assetto Corsa; effect sizes in the end-to-end test are planted in a
simulation. Not done: OpenTelemetry traces. The demo dataset has no setup data, so the demo
stack has no experiments to show.

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
