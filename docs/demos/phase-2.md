# Demo script: phase 2 (measurement)

What phase 2 proves: Paddock can tell a real setup effect from noise, knows what a run plan
can detect before it is driven, and reproduces the finding that sank v0. About five minutes,
no game and no Docker needed.

## 1. The problem, in ten seconds

```bash
python tools/verdict_noise_sim.py
```

v0's rules flag two identical setups as "slower" in 39 % of comparisons at 3 passes per corner.

## 2. The same question, asked of Paddock

```bash
cd engine
uv run pytest tests/test_stats_acceptance.py -v
```

Six tests, about 30 seconds, fixed seeds. They simulate A-B-A-B experiments with the scatter
measured on the real session (28 ms) and check:

| Scenario | Paddock | v0 |
| --- | --- | --- |
| Identical setups: a decision is made | 0.0 % of 200 experiments | 39 % |
| 20 ms per corner effect, 6 passes per arm in 10 corners: detected | 91.5 % | — |
| ... and decided (interval clears the 5 ms equivalence band) | 72.5 % | — |
| 90 % intervals cover the truth (2 or 3 laps per block, with and without lap-level noise) | 88 to 92 % | no intervals |
| The track gaining 10 ms per lap is not credited to the setup | yes | no |

Say the 72.5 % out loud: detecting an effect and being sure it is big enough to matter are two
different bars, and the second one needs more laps. `docs/adr/0008-effect-model.md` has the
table of interval designs that were measured and rejected.

## 3. What a plan can answer, before driving

```bash
uv run python -c "from paddock_engine.stats.mde import *; print(minimum_detectable_effect(28, 6, 10), passes_needed(28, 10, 10))"
```

14.3 ms per corner is detectable with 6 passes per arm in 10 corners; detecting 10 ms would
need 13 passes per arm.

## 4. The finding that started the project, reproduced

```bash
uv run pytest tests/test_features.py -k real_session -v
```

On the real FA26 COTA session, mid-corner front slip is 7.7 degrees in corners below 100 km/h
and 3.7 degrees above 150 km/h, while rear slip is 3.3 and 2.9. One car-wide balance number
hid that; binned by speed it is obvious that the rear is not the story.

## 5. The port is exact

```bash
uv run pytest tests/test_phases.py -q
```

The phase cuts and driver-input metrics are ported from v0's `analyze.lua`. On 14 reference
passes the port reproduces every number v0 computed, to nine decimals.

## 6. The whole chain

```bash
uv run pytest tests/test_features.py -k endpoint_from_agent -v
```

Samples written by the Lua agent, converted to Parquet by the Go ingest code, read with
DuckDB and turned into features by `POST /features/session`.

## Verified and not verified

Acceptance criteria of phase 2:

| Criterion | Result |
| --- | --- |
| Identical setups (sigma 28 ms, 17 corners): false-decision rate at most 10 % at the 90 % threshold | 0.0 % |
| 20 ms per corner effect detected at least 80 % with n = 6, k = 10 | 91.5 % detected; 72.5 % also clear the 5 ms band |
| Speed-binned slip reproduces the audit (about 7.7 degrees below 100 km/h, about 4 above 150) | 7.67 and 3.71 degrees |

**Not verified, and it matters:** every number above is from simulation or from v0's stored
averages. No real A-B-A-B experiment has been driven. The 28 ms scatter is what v0's "push"
passes showed; over all passes of that session it is 82 ms. The new channels (front saturation
index, steering per g, yaw gain) are tested on constructed passes, not yet against a driver's
own judgement of the car.
