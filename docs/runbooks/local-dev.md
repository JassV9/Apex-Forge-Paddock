# Runbook: local development

## Run the processes natively (no Docker for the code)

You need a PostgreSQL to point core at. Either start only the database from Compose:

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
```

or use any local PostgreSQL 16+. Then, in three terminals:

```bash
# 1. core (PowerShell: $env:PADDOCK_DATABASE_URL = "..." and so on)
cd core
export PADDOCK_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock?sslmode=disable"
export PADDOCK_SPOOL_DIR="$HOME/Documents/Assetto Corsa/paddock/spool"   # optional: ingest
export PADDOCK_DATA_DIR="../data"                                          # Parquet files
export PADDOCK_ENGINE_URL="http://localhost:8000"                          # features, plans, verdicts
export PADDOCK_OUTBOX_DIR="$HOME/Documents/Assetto Corsa/paddock/outbox"  # optional: run plans for the agent
go run ./cmd/paddock

# 2. engine
cd engine
uv run uvicorn paddock_engine.app:create_app --factory --port 8000

# 3. web
cd web
npm run dev
```

Check:

```bash
curl http://localhost:8080/health      # {"service":"core","status":"ok","version":"dev"}
curl http://localhost:8000/health      # {"service":"engine","version":"0.1.0","status":"ok"}
curl http://localhost:5173/api/health  # core's answer, through the dashboard's proxy
curl http://localhost:8080/sessions    # recorded sessions
curl http://localhost:8080/metrics     # Prometheus metrics (paddock_ingest_*)
```

To ingest the demo dataset natively, point `PADDOCK_SPOOL_DIR` at `datasets/demo/spool`.

## Run core's database tests

They are skipped when the variable is unset. Each test package works in its own schema, which
it creates and drops; the migration test in `internal/store` uses `public` and needs it empty.

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
docker compose -f deploy/docker-compose.yml exec postgres createdb -U paddock paddock_test
cd core
PADDOCK_TEST_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock_test?sslmode=disable" go test -count=1 ./...
```

Without Docker, the `pgserver` Python package bundles PostgreSQL binaries and starts a
throwaway server:

```bash
uv run --no-project --with pgserver python -c "import pgserver, tempfile, subprocess, os; s = pgserver.get_server(tempfile.mkdtemp()); subprocess.run(['go', 'test', '-count=1', './...'], cwd='core', env=dict(os.environ, PADDOCK_TEST_DATABASE_URL=s.get_uri())); s.cleanup()"
```

## Run the end-to-end test

```bash
export PADDOCK_TEST_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock_test?sslmode=disable"
make test-e2e
```

It builds core, starts it and the engine on free ports with a fresh database (`paddock_e2e`,
next to the test database), and drives three experiments with the Lua agent on the mocked
game. Logs of both processes are printed when a step times out.

## Common problems

| Symptom | Cause | Fix |
| --- | --- | --- |
| `go`, `make` or `uv` "not found" right after installing | The terminal still has the old `PATH` | Open a new terminal |
| `docker` works but `make dev` cannot start | Docker Desktop is not running, or WSL 2 is missing | Start Docker Desktop; `wsl --install` and restart if it asks |
| core exits with `PADDOCK_DATABASE_URL: required environment variable is not set` | The variable is unset | Set it (see above) |
| core exits with `ping database: ...` | PostgreSQL is not up yet or the URL is wrong | Start the database; core fails fast by design and Compose waits for the health check |
| Dashboard says "Core is not reachable" | core is not on port 8080, or `PADDOCK_CORE_URL` points elsewhere | Start core; check the variable |
| Core logs `spool line rejected` | A line in the spool breaks the contract | The log has the session, file, line number and the rule; `paddock_ingest_rejects_total{reason}` counts them |
| A session never appears | `PADDOCK_SPOOL_DIR` (native) or `PADDOCK_SPOOL_HOST` (Docker) points at the wrong folder, or its first file `000001.jsonl` is missing | Core logs the spool folder at start; later files without the session line are rejected as `unknown_session` |
| `POST /sessions/{id}/diagnosis` answers 409 | The session's features are still being computed, or it has no valid pass on its baseline setup | Wait and retry; check `PADDOCK_ENGINE_URL` and core's log |
| The Garage or "What to test next" says it could not be loaded | The engine did not answer: the profile is rebuilt by the engine on every read | Check the engine is up and `PADDOCK_ENGINE_URL`; core's log has the reason |
| An experiment is finished but the Garage does not show it | Only one-item experiments that reached a verdict count; aborted ones and re-checks do not | Nothing to fix |
| The diagnosis says "gemini gave no reply" or "The reply was rejected" | The hosted model failed or wrote something that is not in the evidence | Nothing to fix: the template text is shown. Set `PADDOCK_LLM_PROVIDER=none` to stop asking |
| `POST /experiments/{id}/evaluate` answers 409 "features of N passes are not computed yet" | The features worker has not caught up, or the engine is down | Wait and retry; check `PADDOCK_ENGINE_URL` and core's log |
| `POST /experiments/{id}/start` answers 409 "no longer the champion" | Another experiment was accepted or rolled back since this one was proposed | Discard it and propose it again |
| The end-to-end test needs two minutes to start on Windows | `localhost` resolves to IPv6 first and Docker's PostgreSQL only listens on IPv4 | Already handled in `agent/tests/e2e/stack.py`; use `127.0.0.1` in your own scripts |
| `make gen-check` fails | A schema or `openapi.yaml` changed without regenerating | `make gen` and commit the result |
| "Windows fatal exception: code 0xe24c4a02" in agent test output | LuaJIT raises Lua errors as Windows SEH exceptions and Python's faulthandler prints handled ones | Harmless; `agent/pyproject.toml` disables faulthandler for this reason. Keep `-p no:faulthandler` |
| npm refuses to install with a TypeScript peer conflict | `openapi-typescript` 7 needs TypeScript 5 | Keep `typescript` on `^5.9` until it supports a newer major |
