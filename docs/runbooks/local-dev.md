# Runbook: local development

## Run the processes natively (no Docker for the code)

You need a PostgreSQL to point core at. Either start only the database from Compose:

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
```

or use any local PostgreSQL 16+. Then, in three terminals:

```bash
# 1. core (PowerShell: $env:PADDOCK_DATABASE_URL = "...")
cd core
PADDOCK_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock?sslmode=disable" go run ./cmd/paddock

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
```

## Run core's database tests

They need an empty database and are skipped when the variable is unset.

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
docker compose -f deploy/docker-compose.yml exec postgres createdb -U paddock paddock_test
cd core
PADDOCK_TEST_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock_test?sslmode=disable" go test -count=1 ./internal/store/
```

Without Docker, the `pgserver` Python package bundles PostgreSQL binaries and starts a
throwaway server (this is how the phase 0 migration test was run on Windows):

```bash
uv run --no-project --with pgserver python -c "import pgserver, tempfile, subprocess, os; s = pgserver.get_server(tempfile.mkdtemp()); subprocess.run(['go', 'test', '-count=1', './internal/store/'], cwd='core', env=dict(os.environ, PADDOCK_TEST_DATABASE_URL=s.get_uri())); s.cleanup()"
```

## Common problems

| Symptom | Cause | Fix |
| --- | --- | --- |
| `go`, `make` or `uv` "not found" right after installing | The terminal still has the old `PATH` | Open a new terminal |
| `docker` works but `make dev` cannot start | Docker Desktop is not running, or WSL 2 is missing | Start Docker Desktop; `wsl --install` and restart if it asks |
| core exits with `PADDOCK_DATABASE_URL: required environment variable is not set` | The variable is unset | Set it (see above) |
| core exits with `ping database: ...` | PostgreSQL is not up yet or the URL is wrong | Start the database; core fails fast by design and Compose waits for the health check |
| Dashboard says "Core is not reachable" | core is not on port 8080, or `PADDOCK_CORE_URL` points elsewhere | Start core; check the variable |
| `make gen-check` fails | A schema or `openapi.yaml` changed without regenerating | `make gen` and commit the result |
| "Windows fatal exception: code 0xe24c4a02" in agent test output | LuaJIT raises Lua errors as Windows SEH exceptions and Python's faulthandler prints handled ones | Harmless; `agent/pyproject.toml` disables faulthandler for this reason. Keep `-p no:faulthandler` |
| npm refuses to install with a TypeScript peer conflict | `openapi-typescript` 7 needs TypeScript 5 | Keep `typescript` on `^5.9` until it supports a newer major |
