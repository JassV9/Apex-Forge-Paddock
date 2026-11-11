# Demo script: phase 1 (capture and ingest)

What phase 1 proves: a corner pass travels from the in-game agent to a database row and a
Parquet file, with its conditions, through one contract that both ends are tested against.
About five minutes, no game needed.

## 1. The agent, offline

```bash
cd agent && uv run pytest -q
```

72 tests. Point out three:

- `test_agent.py::test_two_laps_produce_a_valid_spool`: the real `PaddockAgent.lua` runs on a
  mocked CSP API, a synthetic car drives two laps, and every line it writes is validated
  against `contracts/spool_*.schema.json`.
- `test_agent.py::test_restart_teleport_pits_and_replay_never_leave_a_partial_pass`: the v0
  behaviour that makes restart-heavy hot-lapping free.
- `test_agent.py::test_frame_cost_stays_under_budget`: mean cost per frame, budget 0.05 ms
  (measured about 0.002 ms on the development PC).

## 2. One pass, as the agent writes it

```bash
head -c 700 core/internal/ingest/testdata/spool/sample-session/000002.jsonl
```

A pass has its corner, wall-clock start, duration, lap, setup hash, `conditions` (fuel, four
tyre core and surface temperatures, pressures, wear, grip, air and road temperature) and
`samples`: 24 raw channels as column arrays. That file was written by the real agent
(`agent/tests/export_spool_sample.py`) and is what core's Go tests read.

## 3. The real session, replayed

```bash
make demo
```

Open http://localhost:5173. The sessions table shows the FA26 at COTA: **136 corner passes,
17 corners**, the same counts Apex Forge v0 logged for that session. Then:

```bash
curl -s http://localhost:8080/metrics | grep "^paddock_ingest_lines"
docker compose -p paddock-demo restart core
curl -s http://localhost:8080/sessions
```

137 lines ingested, none rejected. After the restart the counts are unchanged: core remembers
which spool files it has ingested, and passes are unique on (session, corner, start time).

Say what this dataset is missing, because it is the point of the project: no conditions and no
time per pass. v0 never recorded them (`datasets/demo/README.md`).

```bash
make demo-down
```

## 4. A bad line is counted, not swallowed

```bash
cd core && go test -run TestScanCountsRejectedLinesByReason -v ./internal/ingest/
```

A truncated line, a line that breaks the schema and a line for another session are rejected
with a reason each, the good line in the same file is still ingested, and
`paddock_ingest_rejects_total{reason}` counts them.

## Verified and not verified

Acceptance criteria of phase 1:

| Criterion | Result |
| --- | --- |
| Replaying the fixture spool gives the same pass count and corner ids as v0 | 136 passes, corners 1 to 17 with 8 passes each: unit test, database test, and `make demo` in Docker |
| Agent frame cost under 0.05 ms in the mocked benchmark | About 0.002 ms mean over 37,000 frames |
| Spool writes at most 5 per second | Tested: smallest gap between writes is at least 0.2 s |
| Restart, teleport and pit tests from v0 pass | Ported and passing, plus replay and AI control |

Also verified: core in Docker (non-root) writes Parquet parts for the agent's sample and lists
the session; the corner map from `corners.lua` matches the one v0 recorded in game at COTA
(local test, needs the game's track files).

**Not verified:** anything in Assetto Corsa. The agent has never run in the game; the
procedure and checklist for its first real run are in `docs/runbooks/agent-in-game.md`.
