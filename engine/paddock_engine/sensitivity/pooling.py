"""Combining the results of several experiments on the same change.

Each experiment gives an effect with an interval. Experiments that tested the same item, the
same clicks and the same channel are pooled by inverse-variance weighting: the precise ones
count for more, and the pooled interval is narrower than any single one. This assumes the
effect of a click does not depend on which baseline it was measured against; where that
fails, the non-linearity check (sensitivity/shape.py) is what notices.
"""

import math
from collections.abc import Sequence
from dataclasses import dataclass

from scipy.stats import norm


@dataclass(frozen=True)
class Estimate:
    """An effect as a mean and a standard error."""

    mean: float
    se: float

    def interval(self, level: float) -> tuple[float, float]:
        half = quantile(level) * self.se
        return self.mean - half, self.mean + half


def quantile(level: float) -> float:
    """Half-width of a two-sided interval at `level`, in standard errors."""
    return float(norm.ppf(0.5 + level / 2.0))


def from_interval(estimate: float, ci_low: float, ci_high: float, level: float) -> Estimate:
    """The standard error an interval implies. A zero-width interval (a channel that never
    moved) keeps a tiny error so it can still be weighted."""
    se = (ci_high - ci_low) / (2.0 * quantile(level))
    return Estimate(mean=estimate, se=max(se, 1e-12))


def pool(estimates: Sequence[Estimate]) -> Estimate:
    """Inverse-variance weighted mean of independent estimates of the same effect."""
    if not estimates:
        raise ValueError("nothing to pool")
    weights = [1.0 / (item.se * item.se) for item in estimates]
    total = sum(weights)
    mean = sum(w * item.mean for w, item in zip(weights, estimates, strict=True)) / total
    return Estimate(mean=mean, se=math.sqrt(1.0 / total))
