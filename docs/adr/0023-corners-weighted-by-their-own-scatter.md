# ADR 0023: Each corner counts by its own scatter

Status: accepted (2026-11-17). Amends the model of [ADR 0008](0008-effect-model.md).

## Context

On the Formula Hybrid 2026 at the Hungaroring, AI experiments 7 to 10 came back with corner-time
intervals of +-28 to +-45 ms over 155 passes. The Ferrari 488 GT3's experiments at COTA got
+-4 ms. The FH26's corners are not alike. Over the push passes of experiments 7 and 8, the
scatter of corner 7 was 151 ms and corner 14 115 ms. Corner 1 was 88 ms, from a few bad passes
(robust 30 ms). Six corners sat at 30 to 47 ms, corners 10 and 6 at 21 to 25 ms, and corners 11,
3 and 4 at 6 to 14 ms. The model of ADR 0008 counts every pass alike, so two or three wild
corners set the interval and the steady ones barely count.

## Decision

**Weighted least squares by corner.** The model is the same, `y ~ arm + covariates +
(1 | corner)`, with the pass-to-pass variance allowed to differ by corner:
`Var(e) = sigma^2 / w[corner]`. The fit (`stats/model.py`) is the GLS solution of that model.
Each corner gets its own quasi-demeaning factor from its own variance, and every row is scaled by
`sqrt(w)`. With equal weights it is the previous fit exactly (hypothesis test). With corner
levels seconds apart it matches statsmodels' WLS with corner dummies to 1e-6.

**The scale comes from the experiment's own passes** (`stats/weights.py`). First the unweighted
model is fitted to all passes. A corner's scale is the standard deviation of that fit's residuals
in the corner, with the corner levels, arms and covariates already removed. Both arms are pooled,
so which setup a pass was driven on does not enter. A corner with fewer than 3 passes counts as
typical.

**Bounds.** The typical corner's scale is the median over corners.

- *Floor: a corner up to 1.5 times the typical scale counts fully* (`STEADY_UP_TO`). A
  corner that happens to look steady (even a scale of 0) gets no more weight than a typical one,
  so it cannot take all the weight. When corners scatter alike, the answer is the unweighted one.
  A standard deviation from 11 passes is good to about +-22 %, so at equal scatter a corner
  rarely crosses 1.5. Simulated, the mean interval width at equal scatter was 18.7 ms both ways
  (100 experiments, 14 corners). With the floor at 1.0 the weights followed the noise of the
  scales, and coverage fell to 83 to 87 % (MAD scales, 200 experiments).
- *Cap: no corner counts less than a tenth of a full-weight corner* (`MAX_WEIGHT_RATIO`), which
  is reached at 4.7 times the typical scale. The effect stays an average over the whole lap, not
  just the steady corners. A cap of 25 or 100 changed the simulated widths and the replayed
  intervals by under 1 ms, so the cap costs nothing measurable.

Only ratios of scales enter, so the weights do not depend on the channel's unit (hypothesis
test).

**Standard deviation, not MAD.** The first version used 1.4826 x MAD, as `stats/noise.py` does.
That scale is robust to a few ruined passes, but the ruined passes stay in the estimate (ADR 0022
rejected an outlier rule). A corner with two 300 ms mistakes then counts fully, and its passes
set the interval. Corner 14 of experiment 7 is the real case: MAD scale 31 ms, standard deviation
123 ms, largest residual 323 ms. Inverse-variance weighting needs the variance the estimate
actually carries. On Gaussian corners the two scales gave the same widths. With mistakes they
did not:

| 200 simulated experiments, 14 corners, effect 20 ms | unweighted | MAD | SD (chosen) |
| --- | --- | --- | --- |
| equal scatter 28 ms: width, coverage | 17.6 ms, 86 % | 17.4 ms, 86 % | 17.5 ms, 86 % |
| 2 corners at 10x | 70.5 ms, 90 % | 20.4 ms, 86 % | 20.2 ms, 86 % |
| FH26-like scatter (10 to 155 ms) | 40.6 ms, 89 % | 21.0 ms, 88 % | 20.5 ms, 88 % |
| 3 % of passes ruined (+0.1 to 0.4 s), all corners | 34.1 ms, 88 % | 33.4 ms, 88 % | 23.5 ms, 88 % |
| 15 % ruined in 2 corners | 28.6 ms, 91 % | 26.0 ms, 90 % | 18.5 ms, 88 % |
| equal, 2 laps per block | 31.3 ms, 92 % | 30.9 ms, 91 % | 30.9 ms, 92 % |
| equal, 15 ms noise shared within a lap | 39.6 ms, 88 % | 39.1 ms, 88 % | 39.5 ms, 88 % |

All estimates were unbiased to within 0.7 ms. Coverage is nominal 90 %. With these seeds,
unweighted equal scatter covers 86 %, and that is the baseline.

The robustness this needs comes from the median, which a few wild corners cannot move, and from
the floor and the cap.

**Interval.** The weights are measured once, on all passes, and held fixed in every bootstrap
resample. Measuring them again in each resample was tried first. Rao-Wu resamples repeat laps,
which adds noise to the scales that the real data does not have. The intervals came out about a third
wider than the scatter of the estimates warrants (MAD scales, 2 corners at 10x: 26.9 ms against
about 20 ms, coverage 95 %).
Held fixed, they cover 86 to 92 % in every scenario above. The t quantile and Rao-Wu rescaling
of ADR 0008 are unchanged.

**Everywhere the effect is estimated.** The primary channel, guardrails, driver inputs,
predictions and every speed class all go through `estimate_effects`, so all of them are
weighted. Each effect carries `corner_weights` (corner number to weight, 1 = full). No contract
changed, and the weights are not stored in the ledger.

**Never silent.** When any corner of the primary effect counted less than fully, the verdict's
reason says so and names up to three corners under half weight, the least weighted first:
"Corners weighted by their own scatter (ADR 0023); least weight: 7, 14, 1." If none fell under
half, the reason ends with "none counted below half" instead. When every corner counted fully,
nothing is added.

## Replay on the real rows (offline; the ledger was not touched)

The passes were rebuilt as `core/internal/experiments/verdict.go` sends them and cut to the
passes each verdict was judged on. The engine at the previous commit reproduces the ledger's
primary intervals exactly for experiments 5 and 7 to 10. Experiment 6's ledger row predates ADR
0022. Corner time, ms:

| Exp | Car | Before (previous engine) | After | Least weight |
| --- | --- | --- | --- | --- |
| 5 | Ferrari 488 GT3 | -0.4 [-4.7, +3.8], negligible | -0.0 [-3.4, +3.3], negligible | 13, 11, 1 |
| 6 | Ferrari 488 GT3 | +4.5 [+0.1, +8.8], inconclusive (ledger: +-450, before ADR 0022) | +2.3 [+0.1, +4.5], negligible | 10, 13, 1 |
| 7 | Formula Hybrid 2026 | -15.8 [-53.2, +21.7] | +0.4 [-19.2, +20.1] | 7, 14, 1 |
| 8 | Formula Hybrid 2026 | +11.3 [-20.8, +43.3] | +5.7 [-13.4, +24.8] | 7, 14 |
| 9 | Formula Hybrid 2026 | +15.8 [-11.9, +43.6] | +2.6 [-17.6, +22.8] | 1, 14, 7 |
| 10 | Formula Hybrid 2026 | -10.2 [-55.0, +34.6] | -4.2 [-36.2, +27.9] | 7, 14, 1 |

The FH26 intervals narrow by 27 to 48 %. On the evidence they had, every FH26 experiment stays
inconclusive. The guardrails narrow too: rear slip peak goes from +-0.52 to +-0.21 degrees on
experiment 7, and from +-0.70 to +-0.21 degrees on experiment 10.

## Consequences

- Past verdicts are not re-judged. The ledger keeps what was decided, and this ADR explains
  why a replay differs.
- An evaluation fits the model one more time (the unweighted fit that gives the scales). The
  bootstrap cost is unchanged.
- **The estimand moves.** With weights, the effect is a precision-weighted average of the
  per-corner effects, not a plain one. If a setup change acts only in a wild corner, it counts at
  as little as a tenth. For a guardrail on an event channel this matters most. Countersteer in
  experiment 7 happens in corners 7 and 12, and those two now count 0.1. A change that brings
  countersteer to one corner is still detected about as often, but its estimate against the
  tolerance shrinks. The cap bounds this, and the per-corner weights are on the effect.

## Not fixed here

- **Noise shared within a lap.** On the FH26, the mean residual of a lap ranges from about -35 to
  +31 ms over the 14 corners. That error is common to every corner, so no corner weighting can
  remove it, and it is now most of what is left of the FH26 interval. The planner counts it since
  [ADR 0025](0025-finished-laps-and-lap-noise.md).
- Per-speed-class effects with one or two corners (for example "slow" on the FH26, 11 passes)
  are as wide as before (+-3.4 s): with one corner there is nothing to weigh.
- A corner whose scatter is changed by the setup itself counts less for it. That is honest about
  precision, but it is part of the estimand point above.
- Everything above is unverified in the game. The replay uses rows the game produced.

## Alternatives considered

- **1.4826 x MAD as the scale.** Measured and rejected, see above.
- **Weights measured again in every resample.** Measured and rejected, see above.
- **Floor at the typical scale (1.0).** It undercovered, see above.
- **Per-arm scales.** These would let which setup a pass was on enter the weights. Pooled
  residuals of the full model do not.
- **Rejecting ruined passes.** Still not built (ADR 0022). Corner weighting reaches most of the
  same gain without dropping data, and it is explainable per corner.
