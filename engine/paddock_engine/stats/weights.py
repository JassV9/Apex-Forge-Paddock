"""How much each corner counts in an effect: the inverse of its own variance (ADR 0023).

Some corners are driven the same way every lap and some are not: on the Formula Hybrid 2026 at
the Hungaroring the pass-to-pass scatter ran from under 10 ms to about 150 ms. Counting every
pass alike lets two wild corners drown the steady ones, so the model is fitted by weighted
least squares, each corner weighted by 1 / scale^2.

The scale is measured, not assumed, from the experiment's own passes: the standard deviation of
the residuals of the unweighted model in that corner (corner levels, arms and covariates
removed, so which setup a pass was driven on does not enter). It is the standard deviation and
not a robust scale because the passes stay in the estimate: a corner with a few ruined passes
carries their variance, and a median absolute deviation would call it steady (ADR 0023).

Robustness comes from the bounds, which keep the weights honest with about ten passes a corner:

- the floor: the typical corner is the median over corners, and no corner within STEADY_UP_TO
  times it counts less than fully, so a corner that happens to look steady does not take all
  the weight, and with corners that scatter alike the answer stays the unweighted one;
- the cap: no corner counts less than 1 / MAX_WEIGHT_RATIO of a full-weight corner, so the
  effect stays an average over the whole lap.

Only ratios of scales matter, so the weights do not depend on the channel's unit.
"""

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray
from paddock_engine.stats.noise import MIN_PASSES_PER_CELL

STEADY_UP_TO = 1.5  # a corner scattering up to this times the typical corner counts fully
MAX_WEIGHT_RATIO = 10.0  # of a full-weight corner to the least-weighted one


def corner_scales(residuals: FloatArray, groups: IntArray) -> FloatArray:
    """Pass-to-pass standard deviation of each corner code; NaN with too few passes to tell."""
    scales = np.full(int(groups.max()) + 1, np.nan)
    for code in range(len(scales)):
        values = residuals[groups == code]
        if len(values) >= MIN_PASSES_PER_CELL:
            scales[code] = float(np.std(values, ddof=1))
    return scales


def corner_weights(scales: FloatArray) -> FloatArray:
    """Relative weight of each corner, 1 for the steady ones down to 1 / MAX_WEIGHT_RATIO.

    A corner without a scale counts as typical. When the typical corner does not scatter at all
    (a count that is 0 on most passes) there is nothing to weigh by and every corner counts 1.
    """
    known = scales[np.isfinite(scales)]
    typical = float(np.median(known)) if len(known) else 0.0
    if typical <= 0.0:
        return np.ones(len(scales))
    floor = STEADY_UP_TO * typical
    measured = np.where(np.isfinite(scales), scales, typical)
    bounded = np.clip(measured, floor, floor * np.sqrt(MAX_WEIGHT_RATIO))
    weights: FloatArray = (floor / bounded) ** 2
    return weights
