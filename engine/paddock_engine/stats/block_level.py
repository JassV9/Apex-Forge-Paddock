"""The error a setup effect carries from block to block (ADR 0029).

The setup changes only between run blocks, so it is identified from the contrast of a few block
means: about B - 2 of them in an A-B-A-B... plan of B blocks. Anything shared by every pass of a
block (a refuel, a restart, tyres that came out a little different, the AI planning a little
differently) moves a whole block, and resampling laps inside blocks cannot see it. Measured on
simulated experiments with 6 ms block shocks, the lap bootstrap alone covered 77 % at 8 blocks.

The block-level error is measured from the experiment's own blocks:

1. the passes' residuals from the weighted fit (arm, covariates and corner levels removed), so
   the covariate slopes and the corner weights are those of the point estimate;
2. one corner-weighted mean per run block;
3. a two-sample t on those block means: the pooled spread of the block means inside each arm.

The degrees of freedom are the blocks less two arm means less one for each covariate that
changes mostly between blocks (time since the experiment started, a session term). Such a
covariate's slope is learnt partly from the block contrasts themselves, and its residual spread
would otherwise look smaller than it is.
"""

from collections.abc import Hashable, Sequence
from dataclasses import dataclass

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray, NotEstimableError
from paddock_engine.stats.observations import Design

# A covariate counts against the block degrees of freedom when more than this share of its
# variation (within corners, the arm removed) lies between blocks.
BLOCK_VARYING_SHARE = 0.5
MIN_BLOCK_DEGREES = 1


@dataclass(frozen=True)
class BlockError:
    standard_error: float
    degrees: int  # blocks - 2 - block-varying covariates
    blocks: int  # run blocks of the baseline and the arm
    block_varying: list[str]  # covariates charged a degree of freedom


def corner_residuals(design: Design, coefficients: FloatArray) -> FloatArray:
    """Each pass less the fitted arm and covariates, less its corner's mean of what is left."""
    residuals = design.y - design.x @ coefficients
    counts = np.bincount(design.groups)
    levels = np.bincount(design.groups, weights=residuals) / counts
    centred: FloatArray = residuals - levels[design.groups]
    return centred


def row_arms(design: Design) -> IntArray:
    """0 for a baseline pass, i + 1 for a pass on the design's i-th arm."""
    indicators = design.x[:, : len(design.arms)]
    on_arm = indicators.max(axis=1) > 0.5
    arms: IntArray = np.where(on_arm, indicators.argmax(axis=1) + 1, 0).astype(np.intp)
    return arms


def cell_means[Key: Hashable](
    values: FloatArray, weights: FloatArray, keys: Sequence[Key]
) -> dict[Key, float]:
    """Weighted mean of `values` per key."""
    sums: dict[Key, list[float]] = {}
    for value, weight, key in zip(values.tolist(), weights.tolist(), keys, strict=True):
        cell = sums.setdefault(key, [0.0, 0.0])
        cell[0] += weight * value
        cell[1] += weight
    return {key: total / weight for key, (total, weight) in sums.items() if weight > 0}


def block_varying(design: Design, blocks: IntArray) -> list[int]:
    """Covariate indices whose variation (within corners, the arm removed) is mostly between
    blocks."""
    arms = len(design.arms)
    indicators = np.column_stack([np.ones(len(design.y)), design.x[:, :arms]])
    counts = np.bincount(blocks)
    varying = []
    for index in range(len(design.covariates)):
        column = design.x[:, arms + index]
        slope, *_ = np.linalg.lstsq(indicators, column, rcond=None)
        rest = column - indicators @ slope
        total = float(rest @ rest)
        if total <= 0.0:
            continue
        means = np.bincount(blocks, weights=rest) / np.maximum(counts, 1)
        if float(np.sum(counts * means**2)) / total > BLOCK_VARYING_SHARE:
            varying.append(index)
    return varying


def arm_error(outcome: FloatArray, regressors: FloatArray) -> tuple[float, float, int]:
    """Coefficient and standard error of the arm (column 1, after the intercept) in a
    least-squares fit of the block means, and its degrees of freedom.

    Raises NotEstimableError when the blocks cannot carry the regressors with at least
    MIN_BLOCK_DEGREES degrees of freedom to spare, or the arm cannot be told from them.
    """
    degrees = len(outcome) - regressors.shape[1]
    if degrees < MIN_BLOCK_DEGREES or np.linalg.matrix_rank(regressors) < regressors.shape[1]:
        raise NotEstimableError(
            f"{len(outcome)} blocks cannot carry the arm and "
            f"{regressors.shape[1] - 2} covariate(s) that change between blocks"
        )
    coefficients, *_ = np.linalg.lstsq(regressors, outcome, rcond=None)
    residual = outcome - regressors @ coefficients
    inverse = np.linalg.inv(regressors.T @ regressors)
    error = float(np.sqrt(residual @ residual / degrees * inverse[1, 1]))
    return float(coefficients[1]), error, degrees


def block_error(
    design: Design,
    coefficients: FloatArray,
    weights: FloatArray,
    blocks: IntArray,
    arm_index: int,
) -> BlockError:
    """Block-level standard error of arm `arm_index` (0-based) against the baseline.

    The corner-weighted block means of the fit's residuals are regressed on the arm and on the
    block means of the covariates that change between blocks; with none, that is the two-sample
    t. A block is a run block and arm: a block that (in a test) holds both arms is two cells.
    `weights` holds one corner weight per corner code.
    """
    arms = row_arms(design)
    rows = (arms == 0) | (arms == arm_index + 1)
    keys = [(int(block), int(arm)) for block, arm in zip(blocks[rows], arms[rows], strict=True)]
    pass_weights = weights[design.groups][rows]
    means = cell_means(corner_residuals(design, coefficients)[rows], pass_weights, keys)
    cells = list(means)
    varying = block_varying(design, blocks)
    columns = [np.ones(len(cells)), np.array([arm != 0 for _, arm in cells], dtype=np.float64)]
    for index in varying:
        covariate = cell_means(design.x[rows, len(design.arms) + index], pass_weights, keys)
        columns.append(np.array([covariate[cell] for cell in cells]))
    outcome = np.array([means[cell] for cell in cells])
    _, error, degrees = arm_error(outcome, np.column_stack(columns))
    return BlockError(error, degrees, len(cells), [design.covariates[i] for i in varying])
