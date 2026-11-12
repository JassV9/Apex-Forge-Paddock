# Demo script: phase 3 (experiments)

What phase 3 proves: a setup change runs as an experiment from proposal to verdict through
every process, a regression is rejected and undone without anyone deciding to, and the ledger
shows how each experiment got where it is. About five minutes. Needs Docker (for PostgreSQL),
Go and uv; no game.

## 1. The whole chain, in one test

```bash
docker compose -f deploy/docker-compose.yml up -d postgres
docker compose -f deploy/docker-compose.yml exec postgres createdb -U paddock paddock_test
export PADDOCK_TEST_DATABASE_URL="postgres://paddock:paddock@localhost:5432/paddock_test?sslmode=disable"
make test-e2e
```

One test, about 90 seconds. What runs is real: `PaddockAgent.lua` under LuaJIT, core built
from source with its PostgreSQL and Parquet files, and the engine. Only the game is
simulated: a car whose grip rises 5 % and whose rear slip rises 1.5 degrees per click of
softer front anti-roll bar, driven with lap-to-lap scatter.

The test plays the driver. It reads the agent's window, pits, clicks "Load Setup X", and
drives the out-lap and the push laps of each block. Three experiments:

| Experiment | What the simulation does | What must happen |
| --- | --- | --- |
| Front bar one click softer | Faster, and the rear slides more | Trade-off; the driver accepts; then rolls back, and the agent is told to load the baseline |
| Front bar one click stiffer (the planted regression) | Slower | Rejected and reverted in one step; the baseline goes back on the car |
| Rear bar one click softer | Nothing | Inconclusive; the baseline is kept |

It also checks that core wrote one setup file per arm, that the prediction was scored, and
that every event in the ledger has a reason.

## 2. The rules, one test each

```bash
cd core && go test -count=1 ./internal/experiments/ -v 2>&1 | grep -E "^(=== RUN|--- )" | grep -v "/"
```

Read the test names aloud; they are the specification. The ones worth pointing at:

- `TestRejectedExperimentPutsTheBaselineBackOnTheCar`
- `TestTheNextExperimentStartsFromTheChampionNotTheLastPass`
- `TestThreeAcceptedExperimentsProposeAReCheckAgainstTheOriginal`
- `TestFailedReCheckReturnsTheChampionToTheOriginal`
- `TestOnlyTheCurrentChampionCanBeRolledBack`

## 3. The bug the end-to-end test found

The first version took a new experiment's baseline from the session's latest pass. Every unit
test passed. The end-to-end test failed on its second experiment: after a rejected experiment
the baseline goes back on the car without a lap being driven, so the latest pass was still on
the rejected variant, and the next experiment started from it. The fix is in
[ADR 0009](../adr/0009-experiments.md): every experiment starts from the champion.

## 4. The agent's side

```bash
cd agent && uv run pytest tests/test_runplan.py tests/test_plan_agent.py -q
```

Twenty tests on the real Lua: a pass is tagged only while the block's setup is on the car, a
value the game refuses undoes the whole load, the position in the plan survives closing the
game, and a broken run plan is ignored while recording goes on.

## 5. The dashboard

```bash
make dev    # http://localhost:5173
```

- **Propose** lists the changeable items of the setup the next experiment would start from.
- **Experiment** shows the test that was fixed before driving, the run plan with what it can
  detect, the blocks as they fill up, the effect of every channel with its interval, the
  verdict, and under **History** every state the experiment was in with when and why.
- While an experiment is being driven the page does not say which code name is the baseline.
- **Ledger** lists every experiment, whatever its outcome.

The demo dataset (`make demo`) is a v0 session without setup data, so experiments cannot be
proposed against it; the pages were checked against the end-to-end run's database.

## Verified and not verified

Acceptance criteria of phase 3:

| Criterion | Result |
| --- | --- |
| End-to-end through the mocked CSP agent: propose, plan, write two setups, drive A-B-A-B, evaluate, decide, revert | `make test-e2e` passes |
| A planted regression is rejected | Rejected, +0.128 s per corner pass, interval [+0.120, +0.137] |
| An inconclusive result reverts to the baseline | Reverted; the knob is back at its value |
| Every state transition is logged and visible in the UI | `experiment_event` rows; History section, tested in `ExperimentPage.test.tsx` |

**Not verified:** anything in Assetto Corsa. The agent's setup loading, its run-plan window
and the pit detection have only run against the mocked CSP API. The effect sizes are planted
in a simulation, not measured on a car. OpenTelemetry traces are still not implemented
(metrics only).
