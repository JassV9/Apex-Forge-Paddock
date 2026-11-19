# ADR 0033: A stratified vehicle sensitivity profile, random-effects pooling, priors apart

Status: accepted (2026-11-19). Amends [ADR 0011](0011-sensitivity-and-ranking.md) (pooling, the
profile key, priors, the stored rows, what rules a change out) and
[ADR 0018](0018-base-setup.md) (the inclusion rule and the prediction). Implements I1, I11, I12
and I13 of the [reliability audit](../investigations/2026-11-18-ghost-driver-reliability.md).
**Unverified in game**: tested on simulations with known truth, against PostgreSQL, and replayed
offline on the real rows (read-only exports).

## Context

- **I1.** Pooling across experiments was fixed-effect with z quantiles: no between-experiment
  variance, no small-k correction; `from_interval` read a standard error back from a t interval
  with z.
- **I11.** Priors from another car were rows of `sensitivity_effect` with their own effect class
  (a "negligible" from the Ferrari lent to the FH26); a consumer reading the class without
  `is_prior` would treat it as measured.
- **I12.** The profile key was item, clicks, channel and corner class: tracks, drivers, AI hints,
  the baseline's value and engine versions were pooled together; the table was deleted and
  rebuilt on every read. The four rear ARB +1 runs on the FH26 were driven at unknown hints and
  judged by an engine before ADR 0029.
- **I13.** Base setup took every change with a one-sided fixed-effect interval below zero, with
  no multiplicity control, and promised the sum of the winners' estimates.

## Decision

### 1. Every experiment is re-derived once per engine version

The ledger keeps intervals, not the standard errors and degrees of freedom behind them, and its
older rows were computed by another method (ADR 0030). Core rebuilds the evaluate request a
finished one-item experiment was judged on (`experiments.ReplayRequest`: the same passes,
exclusions, off-track counts and look) and the engine (`POST /sensitivity/measure`,
`sensitivity/measure.py`) judges it again for its finding and measures the primary channel and
each guardrail over all corners and per frozen speed class (3 corners and 3 block degrees of
freedom at least), with the block-level estimate, standard error and degrees of freedom.
Nothing is written to the ledger. Core keeps the answer in `sensitivity_derivation` (status,
finding, engine version) and `sensitivity_measurement`, once per experiment and engine version
(the engine's `GET /health`). An experiment that cannot be replayed (not fully driven, features
pending) or that the engine refuses is kept as failed with the reason; its ledger rows are pooled
only as **stale** (below). An engine that is down stops the read and stores nothing.

### 2. Strata

A stratum is everything that may change what one click does: item, clicks, channel, corner class,
phase (`corner` for the whole corner; phase rows from ADR 0031 fit the key), **track** (game id and
layout, a `_paddock` copy counted as its layout, ADR 0027), **driver** (AI or human), **AI hint
bucket** (the planning share to the nearest 0.01: the experiment's pin, else its blocks'
`run_context` of ADR 0028; `mixed` when the blocks differ by more than 0.005, `unknown` before
run context existed, `none` for a person), the item's **baseline value** and **click size**.
Measurements of different strata are never pooled into one row. When an otherwise identical
stratum was measured on several tracks, a row with track `all` is made beside the per-track rows
and labelled "All tracks"; consumers that act on one track never read it.

Never pooled, and listed in the profile's `excluded`:

- an experiment whose finding is **invalid** (ADR 0029);
- an experiment with **fewer than 3 block degrees of freedom** (a 4-block run: 1 or 2) in a stratum
  with longer ones: its standard error rests on one or two block contrasts and weighting by it is
  weighting by noise (experiment 8: 0.06 ms on 1 degree of freedom). A stratum of only such runs
  is pooled from them;
- a **stale** measurement (the ledger's numbers, not re-derived) in a stratum with re-derived
  ones. A stratum of only stale rows is pooled from them, marked stale and capped at
  insufficient. Its interval is read back with z (its degrees of freedom were never stored),
  which overstates the error of a t interval: conservative.

### 3. Pooling (`sensitivity/pooling.py`, `half_normal.py`, `knowledge.py`)

| Experiments | Method |
| --- | --- |
| 1 | the experiment's own block-level t interval (`single`) |
| 2 | random effects with a half-normal prior on tau, scale = the margin (the largest error when the channel has none), flat prior on the mean; the posterior mixture's central interval (`half_normal`) |
| 3 or more | Paule-Mandel tau^2, the Hartung-Knapp-Sidik-Jonkman interval with t(k-1) and the max(1, q) floor, and a prediction interval with t(k-2) (`hksj`) |

Each experiment is weighted by its error widened by t(df) / z at the profile's level, so a normal
gives back its own interval: a 4-block experiment counts about 1/15 of what its raw error says.
`from_interval` now inverts with t(df) when the degrees of freedom are known (finding I1).

Coverage of the 90 % interval, 800 simulated pools per cell, per-experiment errors 2 to 6 ms and
themselves estimated (5 degrees of freedom), margin 5 ms (`tests/test_pooling.py`):

| k | tau = 0: HKSJ / fixed | tau = margin/2 | tau = margin |
| --- | --- | --- | --- |
| 2 | 100.0 / 86.4 % (half-normal 98.3) | 100.0 / 79.0 % (96.5) | 99.5 / 65.5 % (90.1) |
| 3 | 99.0 / 84.9 % | 97.0 / 76.7 % | 94.9 / 60.6 % |
| 4 | 96.9 / 84.0 % | 93.0 / 71.8 % | 90.2 / 61.1 % |
| 5 | 95.5 / 83.0 % | 93.0 / 74.4 % | 88.6 / 55.2 % |
| 6 | 93.5 / 81.6 % | 89.5 / 71.6 % | 90.1 / 56.1 % |

Fixed-effect pooling collapses as tau grows; it under-covers even at tau = 0 because the
experiments' errors are estimated. The prediction interval covers a new experiment's true effect
83 % at k = 5, tau = margin (Higgins' interval under-covers with few experiments; reported, it
decides nothing). Paule-Mandel agrees with statsmodels' to 1e-3.

### 4. Knowledge levels, explicit

| Level | Rule |
| --- | --- |
| confirmed | at least 3 experiments and the HKSJ interval wholly beyond the margin |
| neutral | at least 3 experiments and the interval wholly inside the margin (TOST at the interval's level) |
| preliminary | the interval excludes zero (fewer than 3 experiments, or not beyond the margin) |
| insufficient | otherwise; always for a stale row |

Two or fewer experiments never make a pooled confirmation or neutrality. The margin is the
engine's (`config/margins.toml`, per item and channel, ADR 0029), else the frozen band. Two more
kinds of knowledge are kept out of the measured rows: **findings from other cars** are
`SensitivityPrior`s in their own list and table (`sensitivity_prior`) with no effect class and no
level; **general vehicle-dynamics hypotheses** (the parameter-to-observable map,
`response/parameters.toml`) are never stored as findings. Ranking reads priors only from the
request's prior list; base setup lists them as left out, labelled; the Garage shows them in a
separate labelled section. `sensitivity_effect` has no `is_prior` column and a CHECK that every
row names its experiments.

Each row carries its provenance: experiments, engine versions, method, k, tau, the prediction
interval, `p_increase` (one-sided p-value, or posterior probability for two experiments, of an
effect at most zero), stale.

### 5. Base setup (`optimize/base_setup.py`, `fdr.py`)

A row qualifies when it is on this track (not the all-track scope) for the base experiment's
driver, not stale, **confirmed or preliminary with its interval wholly on the better side of
zero**, and its one-sided p-value passes **Benjamini-Hochberg at q = 0.05** across every measured
change of the car on the judged channel (the other exclusions of ADR 0018 stay). q = 0.05 is the
one-sided 5 % each row's 90 % interval already spends, shared among the items: a car with one
measured change is judged as before. The prediction is the sum of the estimates **shrunk towards
zero** by empirical Bayes (true effects ~ N(0, A), A by the method of moments over the car's
measured changes), with the summed measured variances (shrinking moves the centre, it buys no
certainty). The base experiment still judges the combination.

Simulated with known truth (`tests/test_base_setup_fdr.py`), eight items, 3 ms errors: with every
true effect 0, anything qualifies in 5.1 % of 2000 cars (34 % without the adjustment; 10.2 % at
q = 0.10), 0 of 300 with three experiments per item. With true effects from N(0, 6 ms), the shrunk
estimate of what qualified is off by -0.2 ms on average, the raw one by -2.1 ms (the winner's
curse).

Ranking skips only a row pooled as neutral or confirmed worse: one experiment no longer rules a
change out, its narrow belief ranks it last instead.

### 6. Storage (migration 00014)

The pooled rows stay derived (ADR 0011), but a read no longer deletes and rebuilds them: core
hashes each car's pooled reply and replaces the car's rows only when the hash in
`sensitivity_profile` changed. The expensive, provenance-heavy part (the re-derivation) is
persisted append-only per engine version. Up deletes the old rows (they are derived and lack the
strata); Down drops the new tables and columns, restores `is_prior` and deletes the rows again.
00013 and 00015 are taken by changes written in parallel.

## Replay on the real rows (read-only)

Corner time over all corners, FH26 at the Hungaroring, rear ARB +1 (experiments 7, 11, 16, 19;
all AI, hint unknown, baseline 2, one click):

| | Method | Estimate [90 %] | Level |
| --- | --- | --- | --- |
| Before | fixed effect, z, the ledger's intervals of all four | -2.64 [-6.46, +1.17] ms | "inconclusive" |
| After | 7 and 11 left out (4 blocks, 1 block degree of freedom); 16 and 19 re-derived, half-normal tau | -1.84 [-8.03, +4.67] ms, tau 3.4 ms | insufficient |
| For comparison | all four re-derived, HKSJ | -2.23 [-7.66, +3.20] ms, tau 0, prediction [-8.97, +4.51] | insufficient |

Re-derived (engine 0.2.0): 7 -1.03 ms (se 1.96, df 1), 11 -4.67 (2.75, 1), 16 -3.65 (2.50, 6),
19 +1.29 (3.79, 5). Fast corners, 16 and 19: -4.40 [-9.84, +0.90] ms.

Base setup now: on the FH26 nothing qualifies (rear ARB +1, diff preload -1 (17), brake bias -1
(18), front wing +1 (12), rear wing -1 (15) are all insufficient; 8, 9, 10 left out as 4-block
runs), as before. On the 488 GT3 at COTA, ARB front -1 (experiment 3, -6.05 [-7.64, -4.46] ms,
preliminary, 2 block degrees of freedom, alone in its stratum) still qualifies, predicted -5.95 ms
after shrinkage; experiments 4 and 6 re-derive as invalid and are excluded. The live profile still
holds the old rows; they are rebuilt on the first read after the migration.

## Alternatives considered

- **DerSimonian-Laird tau.** Biased low with few experiments; Paule-Mandel is the recommended
  moment estimator for few studies; REML was not implemented.
- **HKSJ at k = 2.** t(1) = 6.3 makes every interval useless; the half-normal prior is explicit
  about the one assumption two experiments need.
- **Weighting by the raw block-level error.** Lets a 4-block experiment dominate (experiment 8).
- **Excluding stale rows outright.** Would silently drop experiments whose passes are gone; a stale
  row is kept, marked and capped instead.
- **Keeping priors as flagged rows.** Every consumer would have to remember the flag (I11).
- **Rebuilding on every read.** Kept the table consistent but wrote on every GET; the hash keeps it
  consistent and quiet.

## Known limits

- Per-experiment variances are treated as known in HKSJ (the t(df)/z widening only approximates
  their uncertainty); 4-block runs are pooled when nothing longer exists.
- The hint bucket rounds to 0.01; older experiments are all `unknown` and pool with each other.
- The all-track scope pools tracks by random effects; nothing models a track effect.
- Shrinkage ignores the uncertainty in A; with few measured changes A is noisy.
- The first read after an engine upgrade re-derives every experiment (about 0.1 s each offline).
- Core and engine were tested apart (fakes, contract round trips); the end-to-end test on the
  mocked game was updated but not run here.
- **Unverified in game**; the Garage was checked with Vitest, not in a browser.
