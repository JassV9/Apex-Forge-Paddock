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
| Docker Desktop | any recent | `make dev` only (PostgreSQL and the full stack) |

Windows, in PowerShell:

```powershell
winget install GoLang.Go astral-sh.uv OpenJS.NodeJS.LTS ezwinports.make Docker.DockerDesktop
```

Open a new terminal afterwards so `PATH` is refreshed. Docker Desktop needs WSL 2
(`wsl --install`, then restart). You do **not** need Docker, Assetto Corsa or a Lua install to
run the tests: the agent's Lua runs under LuaJIT inside the Python test environment.

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

Expected from `make test`: 39 agent tests, three Go packages `ok`, 4 engine tests, 4 web tests.

Core's database tests are skipped unless `PADDOCK_TEST_DATABASE_URL` points at an **empty**
PostgreSQL database; CI always runs them. See `docs/runbooks/local-dev.md` for two ways to run
them locally.

## 3. Start the stack

```bash
make dev     # PostgreSQL + core + engine + web in Docker; open http://localhost:5173
make down
```

The page says "Core dev is running." (local builds report version `dev`) when the dashboard reaches core and core reaches its
database. To run the processes natively instead (no Docker except for PostgreSQL), see the
runbook.

## 4. Find your way around

Read in this order: `docs/architecture.md`, the ADRs in `docs/adr/`, then `CLAUDE.md` for the
conventions. `docs/audit-v0.md` explains why the project exists.

| Directory | What it is | Start reading at |
| --- | --- | --- |
| `contracts/` | JSON Schema for every cross-process type | `health.schema.json` |
| `core/` | Go monolith | `cmd/paddock/main.go` |
| `engine/` | Python analysis engine | `paddock_engine/app.py` |
| `web/` | React dashboard | `src/App.tsx` |
| `agent/` | CSP Lua agent and its offline tests | `tests/harness.py`, then `src/core/recorder.lua` |
| `docs/api/openapi.yaml` | Core's REST API, written before the code | — |
| `tools/` | Scripts, including the v0 verdict-noise simulation | `verdict_noise_sim.py` |

## 5. Make a change

- **A type that crosses a process boundary:** edit or add a schema in `contracts/`, run
  `make gen`, commit the schema and the generated files together. Never edit generated code.
- **A core REST endpoint:** edit `docs/api/openapi.yaml` first, then `make gen`, then the
  handler and its table-driven test.
- **A database change:** add a new numbered file in `core/migrations/` with `-- +goose Up` and
  `-- +goose Down`. Never edit a merged migration.
- **Agent Lua:** pure logic goes in `agent/src/core` (no `ac.*`; a test enforces it), CSP calls
  in `agent/src/game`. Anything not run in Assetto Corsa is reported as "unverified in game".
- Tests ship with the change. Docs change in the same commit.

Optional, runs the fast checks on every commit:

```bash
uvx pre-commit install
```
