"""Combining the results of several experiments on the same change (ADR 0033).

Experiments of one stratum (the same item, clicks, channel, corner class, phase, track, driver,
AI hint, baseline value and click size) are pooled with a random-effects model: each experiment's
true effect may differ from the others' by a between-experiment standard deviation tau (another
day, another AI plan, a different block-1 tank), and the pooled mean is the mean of those
effects. Fixed-effect pooling (tau = 0, z quantiles, ADR 0011) is what this replaces: with tau
at the margin its 90 % interval covers about 60 % at six experiments (tests).

- k >= 3: tau^2 by Paule-Mandel, the Hartung-Knapp-Sidik-Jonkman interval with t(k - 1) and the
  max(1, q) floor (never narrower than the plain random-effects interval), and a prediction
  interval for the effect in a new experiment with t(k - 2).
- k = 2: tau has a half-normal prior scaled to the margin (sensitivity/half_normal.py); two
  experiments say almost nothing about tau, and HKSJ's t(1) would make every interval useless.
- k = 1: the experiment's own block-level t interval.

Each experiment's block-level standard error has few degrees of freedom (1 at 4 blocks, 5 or 6 at
8 to 10). Weighting by the raw error would let a 4-block experiment, whose own 90 % t interval is
6.3 standard errors wide on each side, count like a 10-block one. So an experiment is weighted by
its error widened by t(df) / z at the profile's level: the normal with that error gives back the
experiment's own interval. A 4-block experiment then counts about 1/15 as much as its raw error
would say; one of 8 blocks about 2/3.

Fewer than 3 experiments never make a pooled confirmation (sensitivity/knowledge.py).
"""

import math
from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np
from scipy import optimize, stats

from paddock_engine.contracts.sensitivity_response_schema import Method

HKSJ_FROM = 3
_TAU2_GROWTH = 4.0


@dataclass(frozen=True)
class Estimate:
    """An effect as a mean and a standard error, with the error's degrees of freedom
    (None: treated as known, a normal quantile)."""

    mean: float
    se: float
    df: float | None = None

    def interval(self, level: float) -> tuple[float, float]:
        half = quantile(level, self.df) * self.se
        return self.mean - half, self.mean + half

    def p_increase(self) -> float:
        """One-sided p-value of an effect at most zero."""
        z = self.mean / self.se
        return float(stats.norm.sf(z) if self.df is None else stats.t.sf(z, self.df))


@dataclass(frozen=True)
class Pooled:
    """A pooled effect: its estimate (with the interval's own degrees of freedom), the
    interval, the prediction interval when there is one, tau, and how it was made."""

    estimate: Estimate
    low: float
    high: float
    tau: float | None
    method: Method
    experiments: int
    prediction: tuple[float, float] | None = None
    p_increase: float | None = None

    def p(self) -> float:
        return self.estimate.p_increase() if self.p_increase is None else self.p_increase


def quantile(level: float, df: float | None = None) -> float:
    """Half-width of a two-sided interval at `level`, in standard errors: Student's t with `df`
    degrees of freedom, or the normal when `df` is None."""
    upper = 0.5 + level / 2.0
    return float(stats.norm.ppf(upper) if df is None else stats.t.ppf(upper, df))


def from_interval(
    estimate: float, ci_low: float, ci_high: float, level: float, df: float | None = None
) -> Estimate:
    """The standard error an interval implies. An interval made with t(df) is inverted with
    t(df), never z (finding I1): with z the error of a t interval on 5 degrees of freedom came out
    31 % too large. A zero-width interval keeps a tiny error so it can still be weighted."""
    se = (ci_high - ci_low) / (2.0 * quantile(level, df))
    return Estimate(mean=estimate, se=max(se, 1e-12), df=df)


def pooling_error(estimate: Estimate, level: float) -> float:
    """The error an experiment is weighted by: its own, widened by t(df) / z at `level`."""
    return estimate.se * quantile(level, estimate.df) / quantile(level)


def _weighted(y: np.ndarray, v: np.ndarray, tau2: float) -> tuple[float, np.ndarray]:
    w = 1.0 / (v + tau2)
    return float(w @ y / w.sum()), w


def _q_excess(y: np.ndarray, v: np.ndarray, tau2: float) -> float:
    """Generalised Q minus its expectation k - 1; decreasing in tau^2."""
    mean, w = _weighted(y, v, tau2)
    return float(w @ (y - mean) ** 2) - (len(y) - 1)


def paule_mandel(y: np.ndarray, v: np.ndarray) -> float:
    """The tau^2 at which the generalised Q equals k - 1; 0 when Q is already below it."""
    if _q_excess(y, v, 0.0) <= 0.0:
        return 0.0
    upper = max(float(np.var(y)), float(v.max()), 1e-30)
    while _q_excess(y, v, upper) > 0.0:
        upper *= _TAU2_GROWTH
    return float(optimize.brentq(lambda t2: _q_excess(y, v, t2), 0.0, upper, xtol=1e-30))


def hksj(estimates: Sequence[Estimate], level: float) -> Pooled:
    """Random effects with Paule-Mandel tau^2 and the HKSJ interval (needs k >= 2)."""
    y = np.array([e.mean for e in estimates])
    v = np.array([pooling_error(e, level) ** 2 for e in estimates])
    k = len(estimates)
    tau2 = paule_mandel(y, v)
    mean, w = _weighted(y, v, tau2)
    q = float(w @ (y - mean) ** 2) / (k - 1)
    se = math.sqrt(max(1.0, q) / float(w.sum()))
    pooled = Estimate(mean=mean, se=se, df=k - 1)
    low, high = pooled.interval(level)
    prediction = None
    if k >= HKSJ_FROM:
        half = quantile(level, k - 2) * math.sqrt(tau2 + se * se)
        prediction = (mean - half, mean + half)
    return Pooled(pooled, low, high, math.sqrt(tau2), Method.hksj, k, prediction)


def single(estimate: Estimate, level: float) -> Pooled:
    low, high = estimate.interval(level)
    return Pooled(estimate, low, high, None, Method.single, 1)
