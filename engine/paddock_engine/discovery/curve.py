"""The shape of an observable's response to a swept setup item (ADR 0032).

A sweep measures each step as its own experiment against the same baseline, so the effect of a
step `c` clicks from the baseline is f(c) - f(0) and the baseline itself is the point (0, 0),
known exactly. Each step's interval gives its standard error, se = half-width / z at the
interval's own level (conservative when the interval was a t interval: t > z).

The classification, at one level (0.90 unless asked otherwise), z = z_{(1 + level) / 2}:

1. **insufficient** with fewer than two distinct steps: one step is a point, not a curve; it
   never yields linear or nonlinear, however clear it is.
2. **negligible** when every step's interval at the level lies inside +-margin: two one-sided
   tests at each step, and since the claim is "inside at every step" (an intersection-union
   test) no correction is needed. Without a margin an observable is never negligible.
3. **direction change** when one stretch of the curve rises and another falls, each clear of
   zero: the slopes between neighbouring steps, the baseline included, each tested at
   level 1 - (1 - level) / segments (Bonferroni over the segments). Reported with the step at
   which it turns (the highest or lowest point).
4. **nonlinear** when the quadratic term of the weighted least-squares fit through the baseline,
   y = b c + q c^2 with weights 1 / se^2, is clear of zero at the level.
5. **linear** when the slope b of the weighted straight line through the baseline is clear of
   zero and the quadratic term is not.
6. **insufficient** otherwise: nothing is clear and nothing is shown to be negligible.

Each observable is classified on its own: across many observables some will look linear by
chance (no multiplicity control; the classification describes, it never decides).
"""

from collections.abc import Sequence
from dataclasses import dataclass
from enum import StrEnum
from itertools import pairwise

import numpy as np
from scipy import stats


class Shape(StrEnum):
    linear = "linear"
    nonlinear = "nonlinear"
    negligible = "negligible"
    direction_change = "direction_change"
    insufficient = "insufficient"


@dataclass(frozen=True)
class Point:
    """The effect of one step: `clicks` from the baseline, its estimate and standard error."""

    clicks: int
    estimate: float
    se: float


@dataclass(frozen=True)
class Coefficient:
    estimate: float
    low: float
    high: float

    def clear_of_zero(self) -> bool:
        return self.low > 0.0 or self.high < 0.0


@dataclass(frozen=True)
class Curve:
    shape: Shape
    clicks: tuple[int, ...]
    reason: str
    slope: Coefficient | None = None
    curvature: Coefficient | None = None
    turn_clicks: int | None = None


def standard_error(low: float, high: float, level: float) -> float:
    """The standard error an interval at `level` implies, read as estimate +- z x se."""
    return (high - low) / (2.0 * float(stats.norm.ppf((1.0 + level) / 2.0)))


def pooled(points: Sequence[Point]) -> list[Point]:
    """Usable steps by clicks, ascending: the baseline (0 clicks) and steps without a finite
    positive standard error are left out, and steps at the same clicks are pooled by inverse
    variance."""
    groups: dict[int, list[Point]] = {}
    for p in points:
        if p.clicks != 0 and np.isfinite(p.estimate) and np.isfinite(p.se) and p.se > 0.0:
            groups.setdefault(p.clicks, []).append(p)
    out = []
    for clicks in sorted(groups):
        weights = np.array([1.0 / p.se**2 for p in groups[clicks]])
        estimates = np.array([p.estimate for p in groups[clicks]])
        out.append(
            Point(clicks, float(weights @ estimates / weights.sum()), float(weights.sum() ** -0.5))
        )
    return out


def _fit(points: Sequence[Point], powers: tuple[int, ...]) -> tuple[np.ndarray, np.ndarray]:
    """Weighted least squares through the origin on clicks^power; coefficients and covariance."""
    c = np.array([p.clicks for p in points], dtype=float)
    y = np.array([p.estimate for p in points])
    w = np.array([1.0 / p.se**2 for p in points])
    design = np.column_stack([c**k for k in powers])
    information = design.T @ (w[:, None] * design)
    covariance = np.linalg.inv(information)
    return covariance @ design.T @ (w * y), covariance


def _coefficient(estimate: float, variance: float, z: float) -> Coefficient:
    half = z * float(np.sqrt(variance))
    return Coefficient(float(estimate), float(estimate) - half, float(estimate) + half)


def fits(points: Sequence[Point], z: float) -> tuple[Coefficient, Coefficient]:
    """The slope of the straight line and the quadratic term of the parabola, both through the
    baseline, with their intervals. Needs two distinct non-zero clicks."""
    line, line_cov = _fit(points, (1,))
    parabola, parabola_cov = _fit(points, (1, 2))
    return (
        _coefficient(line[0], line_cov[0, 0], z),
        _coefficient(parabola[1], parabola_cov[1, 1], z),
    )


def negligible(points: Sequence[Point], margin: float | None, z: float) -> bool:
    """Every step's interval inside +-margin (two one-sided tests at each step)."""
    if margin is None or margin <= 0.0:
        return False
    return all(abs(p.estimate) + z * p.se < margin for p in points)


def turn(points: Sequence[Point], level: float) -> int | None:
    """Where the response turns, when one stretch between neighbouring steps (the baseline is
    one, known exactly) rises and another falls, each clear of zero after Bonferroni over the
    stretches; None when it does not."""
    nodes = sorted([Point(0, 0.0, 0.0), *points], key=lambda p: p.clicks)
    pairs = list(pairwise(nodes))
    z = float(stats.norm.ppf(1.0 - (1.0 - level) / (2.0 * len(pairs))))
    signs = []
    for a, b in pairs:
        run = b.clicks - a.clicks
        slope, se = (b.estimate - a.estimate) / run, float(np.hypot(a.se, b.se)) / run
        if abs(slope) > z * se:
            signs.append(1 if slope > 0 else -1)
    if 1 not in signs or -1 not in signs:
        return None
    extreme = max if signs[0] > 0 else min
    return extreme(nodes, key=lambda p: p.estimate).clicks


def _shape(
    usable: Sequence[Point],
    margin: float | None,
    level: float,
    fitted: tuple[Coefficient, Coefficient],
) -> tuple[Shape, str, int | None]:
    """The shape, why, and where a direction change turns, in the order of the module docstring."""
    slope, curvature = fitted
    if negligible(usable, margin, float(stats.norm.ppf((1.0 + level) / 2.0))):
        return Shape.negligible, f"Every step is inside +-{margin:g}.", None
    turned = turn(usable, level)
    if turned is not None:
        return Shape.direction_change, f"It turns at {turned:+d} clicks.", turned
    if curvature.clear_of_zero():
        return Shape.nonlinear, "The quadratic term is clear of zero.", None
    if slope.clear_of_zero():
        return Shape.linear, "The slope is clear of zero; the quadratic term is not.", None
    return Shape.insufficient, "Neither a slope nor a curvature is clear of zero.", None


def classify(points: Sequence[Point], margin: float | None, level: float = 0.9) -> Curve:
    """The shape of one observable's response from its steps (module docstring)."""
    usable = pooled(points)
    clicks = tuple(p.clicks for p in usable)
    if len(usable) < 2:
        reason = "One step is a point, not a curve: at least two are needed."
        return Curve(Shape.insufficient, clicks, reason)
    fitted = fits(usable, float(stats.norm.ppf((1.0 + level) / 2.0)))
    shape, reason, turned = _shape(usable, margin, level, fitted)
    return Curve(shape, clicks, reason, slope=fitted[0], curvature=fitted[1], turn_clicks=turned)
