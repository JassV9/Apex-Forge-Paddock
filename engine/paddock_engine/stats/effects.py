"""Effect of each setup arm against the baseline, with an interval and a class.

The interval comes from a block bootstrap: whole laps are resampled within each run block, so
errors shared by the passes of one lap (a scruffy lap, a gust, traffic) stay together and the
A-B-A-B structure is kept. Two details make it honest with as few as two or three laps per
block (measured in tests/test_stats_acceptance.py; see docs/adr/0008-effect-model.md):

- each resample draws one lap fewer than the block has (Rao-Wu rescaling), which removes the
  downward bias of a bootstrap variance with few clusters;
- the interval is estimate +- t x bootstrap standard error, with Student's t for the number of
  laps, instead of the percentiles of the resamples.

The random seed is fixed, so the same passes always give the same verdict.
"""

from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np
from scipy import stats

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray, NotEstimableError, fit_random_intercept
from paddock_engine.stats.observations import Design, Observation, build_design
from paddock_engine.stats.rope import EffectClass, classify

DEFAULT_LEVEL = 0.90
DEFAULT_REPLICATES = 400
DEFAULT_SEED = 20261111
MIN_USABLE_SHARE = 0.8  # of bootstrap replicates that must be estimable


@dataclass(frozen=True)
class Effect:
    arm: str
    estimate: float  # arm minus baseline, per corner pass, in the channel's unit
    ci_low: float
    ci_high: float
    standard_error: float
    effect_class: EffectClass
    n_passes: int  # passes used, both arms
    corners: int
    sigma_residual: float  # pass-to-pass scatter within a corner, from the model
    covariates: list[str]  # covariates the model adjusted for


def _clusters(observations: Sequence[Observation]) -> list[list[IntArray]]:
    """Row indices grouped as blocks -> laps. A pass without a lap is its own cluster."""
    blocks: dict[int, dict[tuple[str, int], list[int]]] = {}
    for index, observation in enumerate(observations):
        key = ("lap", observation.lap) if observation.lap is not None else ("pass", index)
        blocks.setdefault(observation.block, {}).setdefault(key, []).append(index)
    return [
        [np.array(rows, dtype=np.intp) for rows in laps.values()]
        for _, laps in sorted(blocks.items())
    ]


def _resample(clusters: list[list[IntArray]], rng: np.random.Generator) -> IntArray:
    """Per block, draw one lap fewer than it has, with replacement (at least one)."""
    picked = [
        laps[i] for laps in clusters for i in rng.integers(0, len(laps), size=max(1, len(laps) - 1))
    ]
    return np.concatenate(picked)


def _bootstrap_errors(
    design: Design, clusters: list[list[IntArray]], replicates: int, seed: int
) -> FloatArray:
    """Standard error of each arm's coefficient."""
    rng = np.random.default_rng(seed)
    arms = len(design.arms)
    draws = []
    for _ in range(replicates):
        rows = _resample(clusters, rng)
        groups = np.unique(design.groups[rows], return_inverse=True)[1]
        try:
            fit = fit_random_intercept(design.y[rows], design.x[rows], groups)
        except NotEstimableError:
            continue
        draws.append(fit.coefficients[:arms])
    if len(draws) < MIN_USABLE_SHARE * replicates:
        raise NotEstimableError("too few passes: most bootstrap resamples cannot be fitted")
    errors: FloatArray = np.std(np.array(draws), axis=0, ddof=1)
    return errors


def _t_quantile(level: float, clusters: list[list[IntArray]]) -> float:
    laps = sum(len(block) for block in clusters)
    degrees = max(1, laps - len(clusters) - 1)
    return float(stats.t.ppf(1.0 - (1.0 - level) / 2.0, degrees))


def estimate_effects(
    observations: Sequence[Observation],
    baseline_arm: str,
    rope: float,
    level: float = DEFAULT_LEVEL,
    replicates: int = DEFAULT_REPLICATES,
    seed: int = DEFAULT_SEED,
) -> list[Effect]:
    """One Effect per non-baseline arm. `rope` is the half-width of the equivalence band.

    Raises NotEstimableError when the passes cannot support the model.
    """
    design = build_design(observations, baseline_arm)
    fit = fit_random_intercept(design.y, design.x, design.groups)
    clusters = _clusters(observations)
    errors = _bootstrap_errors(design, clusters, replicates, seed)
    quantile = _t_quantile(level, clusters)
    effects = []
    for column, arm in enumerate(design.arms):
        estimate = float(fit.coefficients[column])
        half_width = quantile * float(errors[column])
        effects.append(
            Effect(
                arm=arm,
                estimate=estimate,
                ci_low=estimate - half_width,
                ci_high=estimate + half_width,
                standard_error=float(errors[column]),
                effect_class=classify(estimate - half_width, estimate + half_width, rope),
                n_passes=sum(o.arm in (arm, baseline_arm) for o in observations),
                corners=int(design.groups.max()) + 1,
                sigma_residual=fit.sigma_residual,
                covariates=design.covariates,
            )
        )
    return effects
