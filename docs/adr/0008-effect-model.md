# ADR 0008: How a setup effect is estimated and judged

Status: accepted (2026-11-12)

## Context

The brief asks for a linear mixed model `y ~ setup + fuel + tyre_temp + time_in_session +
(1|corner)` fitted with statsmodels, block bootstrap intervals, and a ROPE classification,
with two acceptance numbers: at most 10 % false decisions on identical setups, and a 20 ms
effect detected at least 80 % of the time with 6 passes per arm in 10 corners.

Two things were measured while building it, and both changed the design.

**1. statsmodels is too slow to bootstrap.** A block bootstrap refits the model hundreds of
times per evaluation, and the acceptance tests run hundreds of evaluations.

**2. A plain percentile bootstrap over laps is dishonest with few laps.** A run block has two
or three laps. Resampling three laps from three underestimates the variance, and percentiles
ignore how few laps there are. Simulated 90 % intervals covered the truth only 64 to 80 % of
the time.

| Interval | 3 laps per block | 2 laps per block | 3 laps, noise shared within a lap |
| --- | --- | --- | --- |
| Percentile bootstrap | 80 % | 64 % | 76 % |
| Rao-Wu resampling, percentiles | 89 % | 74 % | 84 % |
| Rao-Wu resampling, t interval (chosen) | 92 % | 89 % | 88 % |

(200 simulated experiments each, sigma 28 ms, 12 corners, nominal 90 %.)

## Decision

**Model.** The same random-intercept model, fitted by feasible GLS: Swamy-Arora variance
components, then ordinary least squares on quasi-demeaned data
(`engine/paddock_engine/stats/model.py`). This is the GLS solution of the mixed model and
takes microseconds. A test checks it against statsmodels' `MixedLM` (REML): the coefficients
agree to five decimals. statsmodels is a test dependency only.

**Covariates.** Fuel, tyre temperature and time in session enter when every pass has them,
they vary within corners, and they are not a near copy of another covariate (fuel and time are
almost collinear; one is kept). The effect row records which were used.

**Interval.** Block bootstrap over laps within run blocks, with two corrections:

- each resample draws one lap fewer than the block has (Rao-Wu rescaling);
- the interval is `estimate +- t x bootstrap standard error`, Student's t with
  `laps - blocks - 1` degrees of freedom.

The seed is fixed: the same passes always give the same interval and verdict.

**Judgement.** The interval is classified against a region of practical equivalence (5 ms per
corner for corner time): positive, negative, negligible or inconclusive
(`stats/rope.py`). Only positive and negative are decisions.

## What the acceptance simulations measure

| Scenario | Result | Brief |
| --- | --- | --- |
| Identical setups, 17 corners: a decision is made | 0.0 % | at most 10 % (v0: 39 %) |
| Identical setups: 90 % interval excludes zero | 8.0 % | nominal 10 % |
| 20 ms effect, n = 6, k = 10: detected (interval excludes zero, right side) | 91.5 % | at least 80 % |
| 20 ms effect, n = 6, k = 10: decided (interval clears the 5 ms band) | 72.5 % | not specified |
| Track gaining 10 ms per lap, identical setups: a decision is made | at most 10 %, mean estimate under 3 ms | — |

"Detected" and "decided" are different questions. The brief's MDE formula
(`2.8 x sigma x sqrt(2 / (n x k))`, 14 ms at n = 6, k = 10) answers the first. Clearing the
equivalence band as well needs a larger effect or more laps: at this budget a 20 ms effect is
decided in about 7 experiments out of 10. The planner has to show both numbers, and an
experiment that detects but does not decide is extended or reverted, never accepted.

## Consequences

- An evaluation takes about 50 ms; the acceptance simulations run in half a minute in CI.
- Calibrated intervals are wider than naive ones. That costs power, and it is the honest price.
- scipy is a runtime dependency of the engine (Student's t quantile).

## Known limits

- **The 28 ms figure is optimistic for unfiltered data.** It is the scatter of the passes v0
  called "push" on the real FA26 COTA session. Over all 136 passes of that session, the robust
  per-corner scatter is 82 ms (median), because most are not flat-out. Paddock does not carry
  v0's push filter; it relies on run plans that ask for push laps and on condition windows
  (phase 3). Until a real A-B-A-B experiment is driven, the true sigma under those rules is
  unknown, and at 82 ms the same plan detects only about 40 ms per corner.
- The simulations use Gaussian noise. Real corner times have a slow tail (mistakes, traffic).
- A-B-A-B leaves the variant arm later in the session on average, so the time covariate costs
  some precision. A-B-B-A would not; the brief specifies A-B-A-B.

## Revisit when

Real experiments exist: check interval coverage on repeated baseline-versus-baseline runs, and
replace the Gaussian simulation with resampled real passes.
