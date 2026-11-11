# Demo script: phase 0 (skeleton)

What phase 0 proves: the repository builds, tests and lints in four languages from one set of
commands, every shared type is generated from one schema, and the three processes talk to each
other. About five minutes.

## 1. One command per job

```bash
make setup
make test
make lint
```

Point out: the agent tests execute the **real Lua files** under LuaJIT, including v0's setup
catalog and reply-validation cases on real spinner exports. No game, no Lua install.

## 2. One source of truth for shared types

Open `contracts/health.schema.json`. Add `"starting"` to the `status` enum, then:

```bash
make gen
git status --short
```

Three generated files change: `core/internal/contracts/contracts.go`,
`engine/paddock_engine/contracts/health_schema.py`, `web/src/api/schema.d.ts`. Revert with
`git checkout -- .`. In CI, `make gen-check` fails any change that forgets this step.

## 3. The stack

```bash
make dev
```

Open http://localhost:5173: "Core 0.1.0 is running." Then stop the database and reload:

```bash
docker compose -f deploy/docker-compose.yml stop postgres
```

The page now says core is running but its database is unavailable: core reports `degraded`
instead of failing, and the dashboard only repeats what core says.

```bash
curl http://localhost:8080/health
curl http://localhost:8000/health
make down
```

## 4. Where the project is going

Run the simulation that motivates everything after phase 0:

```bash
python tools/verdict_noise_sim.py
```

The v0 verdict rules call two **identical** setups a regression 39% of the time at 3 passes per
corner. Phase 2's acceptance test is to bring that under 10%.

## Verified and not verified

- Verified on Windows 11: `make gen`, `make test`, `make lint`, the migration up/down test
  against PostgreSQL 16, and the three processes running natively with the dashboard reaching
  core through its proxy.
- **Not verified:** `make dev` and the Dockerfiles (the development machine has no Docker yet),
  and anything in Assetto Corsa. No agent code from this repository has run in the game.
