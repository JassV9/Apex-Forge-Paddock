"""Effect of each setup arm against the baseline, with an interval and a class.

The point estimate and the covariate slopes come from the weighted random-intercept fit
(stats/model.py, ADR 0008 and 0023). The interval comes from the run blocks (ADR 0029): the setup
changes only between blocks, so it is told apart from a few block contrasts, and an error shared
by a whole block (a refuel, a restart, the AI planning differently) is part of what the estimate
carries. The standard error is that of the fit's corner-weighted block means
(stats/block_level.py), and the interval is estimate +- t x standard error with the block
degrees of freedom: blocks - 2, less one per covariate that changes mostly between blocks.

This replaces ADR 0008's bootstrap of laps within blocks, whose laps - blocks - 1 degrees of
freedom (15 at 8 blocks of 3 laps) described noise the setup comparison does not average over:
with 6 ms block shocks its 90 % intervals covered 76 % at 8 blocks; the block-level interval
covers 88 to 92 % at 4, 8 and 12 blocks with block shocks of 0, 3 or 6 ms
(tests/test_stats_blocks.py).

Corners are weighted by their own scatter (stats/weights.py, ADR 0023), measured once on all
passes. Nothing is random: the same passes always give the same verdict.
"""

from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np
from scipy import stats

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.block_level import block_error
from paddock_engine.stats.model import Fit, IntArray, NotEstimableError, fit_random_intercept
from paddock_engine.stats.observations import Design, Observation, build_design
from paddock_engine.stats.rope import EffectClass, classify
from paddock_engine.stats.weights import corner_scales, corner_weights

DEFAULT_LEVEL = 0.90


@dataclass(frozen=True)
class Effect:
    arm: str
    estimate: float  # arm minus baseline, per corner pass, in the channel's unit
    ci_low: float
    ci_high: float
    standard_error: float  # from the spread of the block means (ADR 0029)
    effect_class: EffectClass
    n_passes: int  # passes used, both arms
    corners: int
    sigma_residual: float  # pass-to-pass scatter within a full-weight corner, from the model
    covariates: list[str]  # covariates the model adjusted for
    dropped_covariates: list[str]  # left out: too tangled with the arm or each other (ADR 0022)
    corner_weights: dict[int, float]  # corner number -> how much it counted, 1 = fully (ADR 0023)
    block_degrees: int  # of the interval: blocks - 2 - block-varying covariates
    blocks: int  # run blocks of the baseline and the arm
    block_varying: list[str]  # covariates that cost a block degree of freedom


def weighted_fit(design: Design) -> tuple[FloatArray, Fit]:
    """Corner weights (per corner code, from the unweighted model's residuals, both arms
    together) and the weighted fit."""
    unweighted = fit_random_intercept(design.y, design.x, design.groups)
    residuals = design.y - design.x @ unweighted.coefficients
    weights = corner_weights(corner_scales(residuals, design.groups))
    return weights, fit_random_intercept(design.y, design.x, design.groups, weights)


def block_codes(observations: Sequence[Observation]) -> IntArray:
    """Run blocks numbered 0, 1, ... in order."""
    _, codes = np.unique([o.block for o in observations], return_inverse=True)
    blocks: IntArray = codes.astype(np.intp)
    return blocks


def t_quantile(level: float, degrees: int) -> float:
    """Two-sided Student's t quantile of a `level` interval."""
    return float(stats.t.ppf(1.0 - (1.0 - level) / 2.0, degrees))


def estimate_effects(
    observations: Sequence[Observation],
    baseline_arm: str,
    rope: float,
    level: float = DEFAULT_LEVEL,
) -> list[Effect]:
    """One Effect per non-baseline arm. `rope` is the half-width of the equivalence band.

    Raises NotEstimableError when the passes cannot support the model, the blocks cannot carry
    the arm and the covariates that change between them, or a session term is tangled with the
    arm (session terms are never left out, stats/observations.py)."""
    design = build_design(observations, baseline_arm)
    if design.session_confounded:
        raise NotEstimableError("the game sessions cannot be told apart from the setups")
    weights, fit = weighted_fit(design)
    effects = []
    for column, arm in enumerate(design.arms):
        block = block_error(design, fit.coefficients, weights, block_codes(observations), column)
        estimate = float(fit.coefficients[column])
        half_width = t_quantile(level, block.degrees) * block.standard_error
        effects.append(
            Effect(
                arm=arm,
                estimate=estimate,
                ci_low=estimate - half_width,
                ci_high=estimate + half_width,
                standard_error=block.standard_error,
                effect_class=classify(estimate - half_width, estimate + half_width, rope),
                n_passes=sum(o.arm in (arm, baseline_arm) for o in observations),
                corners=int(design.groups.max()) + 1,
                sigma_residual=fit.sigma_residual,
                covariates=design.covariates,
                dropped_covariates=design.dropped,
                corner_weights=dict(zip(design.corners, weights.tolist(), strict=True)),
                block_degrees=block.degrees,
                blocks=block.blocks,
                block_varying=block.block_varying,
            )
        )
    return effects
