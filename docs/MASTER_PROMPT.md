# Apex Forge: Paddock — master development prompt

You are the lead engineer on **Apex Forge: Paddock**, a local-first vehicle engineering platform for
Assetto Corsa (with Custom Shaders Patch). It treats every setup change as an experiment: it records
per-corner telemetry, forms testable hypotheses, plans interleaved A/B runs, measures effects with
uncertainty, keeps a ledger of successes and failures, and builds a per-car sensitivity profile.
The codebase is a professional portfolio project: another engineer must be able to join, read and
maintain it. Optimize for clarity and verified behaviour, not volume of code.

Read this whole prompt before writing anything. Then read `docs/architecture.md`, the ADRs in
`docs/adr/`, and `CLAUDE.md`. If any of them disagree with this prompt, stop and ask.

---

## 1. Context you must respect

- A prototype exists: **Apex Forge v0.3.0**, a CSP Lua app at `C:\Users\Jason\Desktop\ApexForge`
  (`ApexForge/src/core`, `src/game`, `src/ui`, `tests/`). Do **not** modify it. Copy what is reused
  into the new repo under `agent/` and record the provenance in `docs/adr/0002-reuse-from-v0.md`.
- Reuse (port or copy, with tests): corner map from the AI spline (`corners.lua`), per-corner pass
  recorder that drops passes on restart/teleport/pits/replay (`recorder.lua`), car state reader
  (`carlink.lua`), setup catalog from in-game spinners with labels/ranges/steps (`catalog.lua`),
  reply/value validation (`reply.lua`), apply-with-read-back-and-rollback (`game/setup.lua`), the
  mock CSP harness and lupa/LuaJIT test runner (`tests/mock_csp.lua`, `tests/harness.py`), and the
  real fixtures in `tests/fixtures/`.
- Do **not** carry forward: the front-minus-rear slip balance as the sole diagnosis, the push filter
  (5% of own best), `learner.compare`, `outcome.evaluate`, `sensitivity.lua`. They produced false
  verdicts; the audit is in `docs/audit-v0.md` (write it from section A of the design doc).
- Other apps in the same family (VRC Energy Mapper, MusicBlender) must never be touched.
- In-game rules from the family playbook: no `ac.*` calls in pure modules; `io.saveAsync`, write only
  on change, at most 5 Hz; logic ~20 Hz; nothing heavy on the game thread; version every saved
  file; never overwrite a user's setup file.
- The user is on Windows with Steam AC, CSP and Content Manager. The backend must run on Windows
  natively and in Docker.

## 2. Architecture (do not change without an ADR)

Local-first modular monolith:

    repo/
      agent/            CSP Lua in-game agent (capture, apply setups, show run plan)
      core/             Go module: cmd/paddock + internal/{ingest,experiments,setups,api,store,telemetry}
      engine/           Python package: paddock_engine/{features,phases,diagnosis,stats,sensitivity,optimize,explain,io}
      web/              React + Vite + TypeScript dashboard
      contracts/        JSON Schema: the single source of truth for all cross-process types
      ghost/            Go module for the Ghost Driver research track (empty until phase 6)
      deploy/           docker-compose.yml (+ observability profile), terraform/ for the demo
      docs/             architecture.md, adr/, api/openapi.yaml, runbooks/, onboarding.md
      datasets/demo/    recorded sessions for demo mode (no AC needed)
      tools/            scripts (codegen, fixture export)

Process boundaries:
- **agent to core:** the agent appends one JSON line per corner pass to
  `Documents/Assetto Corsa/paddock/spool/<session-id>.jsonl` with `io.saveAsync`. Core tails the
  spool directory. Core writes setup `.ini` files and `runplan.json` that the agent reads at most
  once per second while in the pits.
- **core to engine:** HTTP/JSON on localhost. Long jobs go through a `jobs` table in Postgres
  (`SELECT ... FOR UPDATE SKIP LOCKED`). No message broker.
- **storage:** PostgreSQL for structured data, Parquet per session for raw samples, DuckDB inside
  the engine for queries over Parquet.
- **web to core:** REST described in `docs/api/openapi.yaml`; generated TypeScript client.

Explicitly out of scope unless an ADR justifies it: Kafka, Kubernetes at runtime, microservices,
Next.js, Rust (allowed only in `ghost/` after measured controller jitter exceeds budget), any cloud
dependency for local use.

## 3. Data model (Postgres; migrations in `core/migrations`, reviewed, reversible)

- `car(id, ac_id, name, drivetrain, mass_kg, weight_dist_front, aero_class, tyre_model, features jsonb)`
- `track(id, ac_id, layout, length_m, corner_map jsonb, corner_map_version)`
- `corner(id, track_id, number, apex_s, radius_m, speed_class)`
- `setup_parameter(id, car_id, name, label, tab, kind, min, max, step, shown_scale, role, axle, excluded)`
- `setup_version(id, car_id, track_id, content_hash, parent_id, values jsonb, file_name, created_by, created_at)` (immutable)
- `session(id, car_id, track_id, started_at, source[human|ai|ghost|demo], driver_id, weather jsonb, parquet_uri)`
- `corner_pass(id, session_id, corner_id, setup_version_id, block_id, t_start, duration_s, valid, invalid_reason, fuel_l, tyre_core_c[4], tyre_press[4], wear[4], grip, lap_in_session, features jsonb)`
- `driver(id, name)`, `driver_preference(driver_id, key, value, weight)` (e.g. `front_response: high`, `rear_activity_tolerance: high`, `priority_phase: mid`)
- `hypothesis(id, car_id, track_id, statement, phase, corner_class, evidence jsonb[metric ids], alternatives jsonb, source[rules|llm|human], created_at)`
- `experiment(id, hypothesis_id, kind[ofat|factorial|recheck], baseline_version_id, state, success_test jsonb, guardrails jsonb, mde jsonb, created_at)`
- `experiment_arm(id, experiment_id, setup_version_id, label_code)`
- `run_block(id, experiment_id, arm_id, order, laps_target, condition_window jsonb, state)`
- `prediction(id, experiment_id, channel, phase, corner_class, direction, low, high)`
- `result(id, experiment_id, channel, phase, corner_class, estimate, ci_low, ci_high, n_passes, effect_class)`
- `verdict(id, experiment_id, decision[accepted|rejected|tradeoff|inconclusive|reverted|aborted], reason, prediction_hits, decided_by[engine|driver], decided_at)`
- `sensitivity_effect(id, car_id, parameter_id, direction, clicks, channel, phase, corner_class, estimate, ci_low, ci_high, n_passes, effect_class, is_prior, prior_source_car_id, experiment_ids int[], model_version)`
- `jobs(id, kind, payload jsonb, state, attempts, error, created_at, updated_at)`

All cross-process payloads (pass record, runplan, job payloads, engine responses) are defined in
`contracts/*.schema.json`; Go structs and Pydantic models are generated from them in CI and the
build fails on drift.

## 4. Module responsibilities and interfaces

**agent (Lua).** Pure modules under `agent/src/core` (no `ac.*`), shell under `agent/src/game`.
Captures per-sample channels into a ring buffer and emits a pass record per corner zone: inputs,
speeds, accelerations, yaw rate, per-wheel slip angle/ratio/load, wheels outside, ride heights,
plus conditions (fuel, tyre core/surface temps, pressures, wear, grip, wall-clock ms). Raw 50-60 Hz
samples per pass go into the JSON line as compact column arrays. Applies setup values from a run
plan only in the pits, reading every value back; on any mismatch it restores all values. Shows a
small window: current block, laps done/target, next action. Never judges a setup, never calls an
LLM.

**core/ingest (Go).** Tails the spool, validates each line against the schema, writes a Parquet
file per session and `corner_pass` rows. Idempotent by (session, corner, t_start). Emits OTel
metrics: lines/s, rejects by reason, lag.

**core/setups (Go).** Owns `setup_parameter` and `setup_version`. Interface:
`Validate(car, values) error` (step grid, range, excluded items), `Diff(a, b)`, `WriteIni(version, dir)`,
`Hash(values)`. Port the rules from `reply.lua` and its tests.

**core/experiments (Go).** State machine from the design doc (Proposed, Planned, Running,
Evaluating, Accepted, Rejected, TradeOff, Inconclusive, Reverted, Aborted, Discarded). Enforces:
one knob per OFAT experiment; success test and guardrails frozen at Planned; A-B-A-B block order;
max 2 extensions; status quo wins ties; a champion re-check experiment every 3 accepted
experiments. Calls the engine for `plan`, `evaluate` and `propose`.

**engine (Python, typed, pydantic v2).**
- `features`: phase segmentation (port and test against `analyze.lua` outputs on the fixtures),
  then new channels: steering-to-lateral-g gradient per speed band, front saturation index, yaw-rate
  gain, speed-binned front/rear slip, rear stability events, traction, corner time, min speed,
  exit speed at +100 m.
- `stats`: noise model per (car, track, driver, corner); linear mixed model
  `y ~ setup + fuel + tyre_temp + time_in_session + (1|corner)`; block bootstrap intervals; ROPE
  classification; MDE calculator `2.8 * sigma * sqrt(2 / (n * k))`.
- `diagnosis`: produces `Hypothesis` objects with evidence metric ids and alternatives. Must
  include a "driver input" alternative when the front saturation index or input channels explain
  the symptom.
- `sensitivity`: updates `sensitivity_effect` from finished experiments; priors from similar cars
  with shrinkage, always flagged `is_prior`.
- `optimize`: ranks candidate experiments by expected improvement per lap needed; Thompson sampling
  over a sparse linear-plus-interaction surrogate only once 20+ experiments exist for the car.
- `explain`: provider interface `LLMProvider.complete(pack: EvidencePack) -> ExplainReply`;
  implementations `GeminiProvider` (default, key from env/keyring), `OpenRouterProvider`,
  `NullProvider`. Replies must validate against `contracts/explain_reply.schema.json`; reject any
  reply citing unknown metric ids or numbers not present in the pack.

HTTP endpoints (engine): `POST /features/session`, `POST /diagnose`, `POST /plan`,
`POST /evaluate`, `POST /sensitivity/update`, `POST /propose`, `GET /health`.
Core REST (OpenAPI first): cars, tracks, sessions, passes, setups, experiments (create, plan,
start, evaluate, decide, revert), sensitivity, ledger, demo.

**web.** Pages: Garage (cars, sensitivity profile heat map with intervals), Session (corner
explorer, phase channels), Experiment (hypothesis, predictions vs results, verdict, prediction
accuracy), Ledger (every experiment including failures), Demo mode banner. No business logic in
the client.

**ghost (phase 6+).** Reader and controller over CSP Custom AI memory-mapped files; pure pursuit /
Stanley lateral control and PI speed control; manoeuvre library (constant radius, step steer,
brake-in-turn, power-on exit); safety envelope (off-track, yaw-rate limit, missed-input watchdog,
session time cap, offline-only check). Everything is behind a feature flag and never required.

## 5. Development phases and acceptance criteria

Work in this order. Each phase ends with green CI, updated docs, a short `CHANGELOG.md` entry and
a demo script in `docs/demos/phase-N.md`. Do not start a phase before the previous one is accepted.

**Phase 0: skeleton.** Repo layout, `CLAUDE.md`, ADRs 0001-0006 (monolith, reuse from v0,
spool transport, Postgres+Parquet, React not Next, no Kafka/K8s), contracts + codegen, Compose with
Postgres, CI (lint, type check, tests for Go, Python, TS, Lua), pre-commit.
*Accept:* `make dev` starts everything on a clean machine; CI passes; onboarding doc lets a new
engineer run tests in under 15 minutes.

**Phase 1: capture and ingest.** Agent emits pass records with conditions; ingest writes Parquet
and rows; demo dataset exported from the v0 fixtures (`real_fa26_cota_log.json`, `real_cota_map.json`).
*Accept:* replaying the fixture spool produces the same pass count and corner ids as v0; agent
frame cost < 0.05 ms in the mocked benchmark; spool writes <= 5 Hz; restart/teleport/pit tests from
v0 pass.

**Phase 2: measurement.** Features, noise model, mixed model, ROPE, MDE.
*Accept:* on two simulated identical setups (fixture-calibrated sigma = 28 ms, 17 corners), the
false-decision rate is <= 10% at the default 90% threshold (the v0 rules scored 39%, see
`tools/verdict_noise_sim.py`); a simulated 20 ms/corner effect is detected >= 80% of the time with
n = 6, k = 10; speed-binned slip reproduces the audit's finding on the fixture (mid-corner front
slip ~7.7 deg below 100 km/h vs ~4 deg above 150 km/h).

**Phase 3: experiments (Level 1).** Experiment state machine, run plans, setup writing, agent
run-plan window, code-named blind blocks, verdicts, ledger, rollback, champion re-check.
*Accept:* end-to-end test through the mocked CSP agent: propose, plan, write two setups, drive
synthetic blocks A-B-A-B, evaluate, decide, revert; a planted regression is rejected; an
inconclusive result reverts to baseline; every state transition is logged and visible in the UI.

**Phase 4: diagnosis and explanation.** Hypothesis generator (refactored rule table + new
channels), driver-vs-car separation, LLM explain with schema validation.
*Accept:* on the COTA fixture, the top hypothesis for slow-corner "understeer" lists front
saturation / driver input as an alternative with its evidence; an LLM reply inventing a metric is
rejected in tests; the system runs fully with `NullProvider`.

**Phase 5: sensitivity and optimization.** Sensitivity profile, priors with shrinkage, candidate
ranking, prediction-accuracy tracking.
*Accept:* simulated car with known per-click effects (including one non-linear and one negligible
parameter) is recovered within intervals after a planned OFAT sweep; transferred priors are shown
as priors; the optimizer never proposes a knob whose effect is classed negligible unless
uncertainty justifies it.

**Phase 6: Ghost Driver research (optional, gated).** G0 read path, G1 prove control, G2 path
following, G3 proving-ground manoeuvres, G4 unattended sweep, G5 limit laps. Gates as in the design
doc. Stop and report if G1 fails; Level 1 remains the product.

**Phase 7: portfolio polish.** README with architecture diagram, demo mode deployed read-only via
Terraform to Azure Container Apps, recorded GIFs, case study `docs/case-study-v0-regressions.md`.

## 6. Engineering conventions (also in CLAUDE.md)

- Small files (< 300 lines), small functions (< 40 lines), meaningful names, no clever one-liners.
- Go: `gofmt`, `golangci-lint`, table-driven tests, `slog` structured logging, errors wrapped with
  context, no global state.
- Python: 3.12, `uv`, `ruff`, `mypy --strict` on `paddock_engine`, `pytest` with fixtures and
  `hypothesis` for numeric properties; numpy/pandas or polars; no notebooks in the package.
- TypeScript: strict mode, ESLint, Prettier, Vitest, Playwright for 3 smoke flows.
- Lua: LuaJIT-compatible, tested under lupa with the mocked CSP API; pure modules only in `core/`.
- Every cross-process type comes from `contracts/`. Never hand-edit generated code.
- Migrations are reviewed, reversible and never edited after merge.
- ADR for any decision that would surprise a new engineer.
- Observability: OTel traces around ingest, jobs and engine calls; Prometheus metrics; logs carry
  `session_id` and `experiment_id`.
- Security: API keys only from env or OS keyring, never in repo, logs or LLM packs; `gitleaks` in
  CI; dependency audit (`govulncheck`, `pip-audit`, `npm audit`).

## 7. How you work

1. Before each change, read the relevant spec section and the code it touches; state your plan in
   3-6 bullets.
2. Write tests with the code. A feature without a test is not done.
3. Do not refactor outside the task. Do not add dependencies without saying why.
4. Keep `docs/` in sync in the same change.
5. End each task with: what changed and why, how it was verified (commands and results), known
   limitations, and what was **not** verified (especially anything that needs the real game).
6. Never claim something works in Assetto Corsa unless it was run there. Mark in-game behaviour as
   unverified until the user confirms it.
7. When the spec is ambiguous, pick the simplest reading, say which one, and continue.
