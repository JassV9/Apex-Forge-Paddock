"""Lap noise: the error a whole lap shares across all its corners (ADR 0025).

Some laps are quicker or slower everywhere at once: the AI's pace on that lap, the tyres, a
gust. Every corner of the lap carries that error, so it does not average out over corners, only
over laps. On the Formula Hybrid 2026 it was most of what was left of an experiment's interval
once corners were weighted by their scatter (ADR 0023).

It is the between-lap variance component of the random-intercept model, with the lap as the
group (the moment estimate of `stats/model.py`, Swamy-Arora): the variance of the laps' mean
residuals minus what their passes' own scatter puts into those means. Two choices fit it to the
planner:

- each lap's mean weights its corners as the verdict does (`stats/weights.py`), from the
  per-corner scatter of `stats/noise.py`, so a wild corner's pass noise is not taken for lap
  noise;
- the spread of the lap means is robust (1.4826 x MAD), as the pass scatter is, so a ruined lap
  or a lap at another tyre hint during calibration does not set the plan.

A corner's scatter is measured around its level over all laps, so it holds one share of the lap
error as well; the estimate takes that into account (`_lap_variance`). Only laps that reached at
least half of the corners count.
"""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

import numpy as np

from paddock_engine.stats.noise import MAD_TO_SIGMA
from paddock_engine.stats.weights import corner_weights

MIN_LAPS = 6  # one experiment's laps on one setup: two blocks of three
MIN_CORNER_SHARE = 0.5  # of the corners with a scatter, for a lap to count


@dataclass(frozen=True)
class LapPass:
    corner: int
    lap: int  # passes with the same code were driven on the same lap
    value: float


@dataclass(frozen=True)
class LapMeans:
    means: list[float]  # weighted mean residual per lap
    pass_noise: list[float]  # variance the passes' own scatter puts into each mean
    shares: list[float]  # sum(w^2) / sum(w)^2: how much of one corner's variance a mean keeps


def _centres(passes: Sequence[LapPass], corners: set[int]) -> dict[int, float]:
    values: dict[int, list[float]] = {}
    for p in passes:
        if p.corner in corners:
            values.setdefault(p.corner, []).append(p.value)
    return {corner: float(np.median(v)) for corner, v in values.items()}


def lap_means(passes: Sequence[LapPass], per_corner: Mapping[int, float]) -> LapMeans:
    """Each lap's mean residual, its corners weighted as the verdict weighs them."""
    corners = sorted(per_corner)
    scales = np.array([per_corner[c] for c in corners], dtype=np.float64)
    weight = dict(zip(corners, corner_weights(scales).tolist(), strict=True))
    centre = _centres(passes, set(corners))
    laps: dict[int, list[LapPass]] = {}
    for p in passes:
        if p.corner in weight:
            laps.setdefault(p.lap, []).append(p)
    result = LapMeans([], [], [])
    for lap in laps.values():
        if len(lap) < MIN_CORNER_SHARE * len(corners):
            continue
        w = np.array([weight[p.corner] for p in lap])
        residuals = np.array([p.value - centre[p.corner] for p in lap])
        variances = np.array([per_corner[p.corner] ** 2 for p in lap])
        total = float(w.sum())
        result.means.append(float(w @ residuals) / total)
        result.pass_noise.append(float((w**2) @ variances) / total**2)
        result.shares.append(float(w @ w) / total**2)
    return result


def _lap_variance(spread: float, means: LapMeans) -> float:
    """Solves Var(mean) = lap + pass_noise - share x lap for the lap variance.

    A corner's variance is its pass noise plus the lap's, so pass_noise counts the lap's share
    once too often; with one corner per lap the two cannot be told apart at all.
    """
    share = float(np.mean(means.shares))
    if share >= 1.0 - 1e-12:
        raise ValueError("with one corner per lap, lap noise cannot be told from pass noise")
    excess = spread**2 - float(np.mean(means.pass_noise))
    return max(0.0, excess / (1.0 - share))


def lap_scatter(passes: Sequence[LapPass], per_corner: Mapping[int, float]) -> tuple[float, int]:
    """The lap noise as a standard deviation, and how many laps it was measured on.

    `per_corner` is the pass scatter of each corner (`NoiseModel.per_corner`). Raises ValueError
    with fewer than MIN_LAPS laps that reach enough corners.
    """
    means = lap_means(passes, per_corner)
    if len(means.means) < MIN_LAPS:
        raise ValueError(f"{len(means.means)} laps reach enough corners, {MIN_LAPS} are needed")
    values = np.array(means.means)
    spread = MAD_TO_SIGMA * float(np.median(np.abs(values - np.median(values))))
    return float(np.sqrt(_lap_variance(spread, means))), len(values)
