# Onboarding

From a fresh clone to a green test run takes under 15 minutes, most of it downloads.

## 1. Install the tools

| Tool | Version | Used for |
| --- | --- | --- |
| Git | any recent | — |
| Go | 1.27+ | `core/` |
| uv | 0.12+ | Python 3.12 environments for `engine/` and the agent tests |
| Node.js | 24+ (with npm) | `web/` |
| GNU make | 4+ | Entry points (`make test`, `make dev`, ...) |
| Docker Desktop | any recent | `make dev` and `make demo` (PostgreSQL and the full stack) |

Windows, in PowerShell:

```powershell
winget install GoLang.Go astral-sh.uv OpenJS.NodeJS.LTS ezwinports.make Docker.DockerDesktop
```

Open a new terminal afterwards so `PATH` is refreshed. Docker Desktop needs WSL 2
(`wsl --install` in an administrator terminal, then restart Windows). You do **not** need
Docker, Assetto Corsa or a Lua install to run the tests: the agent's Lua runs under LuaJIT
inside the Python test environment.

One more Go tool, needed only for `make lint`:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

## 2. Get the dependencies and run everything

```bash
make setup   # go mod download, uv sync (engine, agent), npm ci
make test    # agent (Lua under LuaJIT), core (Go), engine (Python), web (TypeScript)
make lint    # go vet + golangci-lint, ruff + mypy --strict, tsc + eslint + prettier
```

`make test` runs four suites and each must end without failures. One agent test is skipped
unless Assetto Corsa with the COTA track is installed. Core's database tests are skipped unless
`PADDOCK_TEST_DATABASE_URL` points at a PostgreSQL database; CI always runs them. See
`docs/runbooks/local-dev.md` for how to run them locally.

With that variable set, the agent suite also runs the end-to-end test: the Lua agent on the
mocked game, core built from source and the engine, through three experiments (about 90
seconds). `make test-e2e` runs only that test and fails instead of skipping when something it
needs is missing.

## 3. Start the stack

```bash
make demo        # the full stack on a recorded session; open http://localhost:5173
make demo-down
make demo-replica        # the read-only public demo (docs/runbooks/deploy-demo.md)
make demo-replica-check  # reads are answered, writes are refused
make demo-replica-down
```

The page says "Core dev is running." (local builds report version `dev`) and lists one
session: the FA26 at COTA, 136 corner passes over 17 corners. No game is needed.

```bash
make dev         # the same stack, ingesting what the in-game agent writes
make down
```

`make dev` reads the agent's spool from the folder in `PADDOCK_SPOOL_HOST`. Installing the
agent and pointing the stack at it is in `docs/runbooks/agent-in-game.md`. To run the
processes natively instead of in Docker, see `docs/runbooks/local-dev.md`.

## 4. Find your way around

Read in this order: `docs/architecture.md`, the ADRs in `docs/adr/`, then `CLAUDE.md` for the
conventions. `docs/audit-v0.md` explains why the project exists.

| Directory | What it is | Start reading at |
| --- | --- | --- |
| `contracts/` | JSON Schema for every cross-process type | `spool_pass.schema.json` |
| `agent/` | CSP Lua agent and its offline tests | `src/game/agent.lua`, then `tests/test_agent.py` |
| `core/` | Go monolith | `cmd/paddock/main.go`, then `internal/ingest/service.go` and `internal/experiments/service.go` |
| `engine/` | Python analysis engine | `paddock_engine/app.py` |
| `web/` | React dashboard | `src/App.tsx` |
| `docs/api/openapi.yaml` | Core's REST API, written before the code | — |
| `datasets/demo/` | Recorded sessions for demo mode | `README.md` |
| `tools/` | Scripts: agent installer, demo export, the v0 verdict-noise simulation | `verdict_noise_sim.py` |

## 5. Make a change

- **A type that crosses a process boundary:** edit or add a schema in `contracts/`, run
  `make gen`, commit the schema and the generated files together. Never edit generated code.
- **A core REST endpoint:** edit `docs/api/openapi.yaml` first, then `make gen`, then the
  handler and its table-driven test.
- **A database change:** add a new numbered file in `core/migrations/` with `-- +goose Up` and
  `-- +goose Down`. Never edit a merged migration.
- **Agent Lua:** pure logic goes in `agent/src/core` (no `ac.*`; a test enforces it), CSP calls
  in `agent/src/game`. Anything not run in Assetto Corsa is reported as "unverified in game".
- **A spool format change:** bump `v` in the schema, teach ingest both versions, and
  regenerate the sample the Go tests read
  (`cd agent && uv run python tests/export_spool_sample.py ../core/internal/ingest/testdata/spool`).
- Tests ship with the change. Docs change in the same commit.

Optional, runs the fast checks on every commit:

```bash
uvx pre-commit install
```
