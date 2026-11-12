"""The measurement model: y ~ arm + covariates + (1 | corner).

Every corner has its own level (a hairpin takes longer than a kink), which is the random
intercept. The setup effect is the coefficient of the arm indicator: the average difference per
corner pass with corners, fuel, tyre temperature and time in session accounted for. One model
over all corners replaces v0's 17 separate per-corner comparisons, each with its own chance of
a false alarm.

The model is fitted by feasible GLS (Swamy-Arora variance components, then quasi-demeaning),
which is the GLS solution of the random-intercept mixed model and takes microseconds, so the
bootstrap can refit it hundreds of times. tests/test_stats_model.py checks it against
statsmodels' MixedLM. See docs/adr/0008-effect-model.md.
"""

from dataclasses import dataclass

import numpy as np
import numpy.typing as npt

from paddock_engine.io.samples import FloatArray

IntArray = npt.NDArray[np.intp]


class NotEstimableError(ValueError):
    """The data cannot identify the model (too few passes, or an arm missing)."""


@dataclass(frozen=True)
class Fit:
    coefficients: FloatArray  # one per column of x, in order
    sigma_residual: float  # pass-to-pass scatter within a corner
    sigma_group: float  # spread of the corners' own levels


def _group_means(values: FloatArray, groups: IntArray, counts: FloatArray) -> FloatArray:
    sums = np.zeros((len(counts), *values.shape[1:]))
    np.add.at(sums, groups, values)
    return sums / counts.reshape(-1, *([1] * (values.ndim - 1)))


def _residual_variance(y: FloatArray, x: FloatArray, lost_degrees: int) -> tuple[FloatArray, float]:
    """Least squares of y on x; returns coefficients and SSR / (n - rank - lost_degrees)."""
    coefficients, _, rank, _ = np.linalg.lstsq(x, y, rcond=None)
    degrees = len(y) - int(rank) - lost_degrees
    if degrees <= 0:
        raise NotEstimableError("not enough passes for the number of corners and covariates")
    residuals = y - x @ coefficients
    return coefficients, float(residuals @ residuals) / degrees


def _group_variance(
    y_means: FloatArray, x_means: FloatArray, counts: FloatArray, residual: float
) -> float | None:
    """Variance of the corner levels from the between-corner regression; None if not identified."""
    design = np.column_stack([np.ones(len(y_means)), x_means])
    if len(y_means) <= np.linalg.matrix_rank(design):
        return None
    _, between = _residual_variance(y_means, design, 0)
    harmonic_size = len(counts) / float((1.0 / counts).sum())
    return max(0.0, between - residual / harmonic_size)


def _shrinkage(residual: float, group: float, counts: FloatArray) -> FloatArray:
    """How far each corner's passes are pulled towards the corner's own mean (0: not at all).

    A channel with no scatter at all (a count that is 0 on every pass) has nothing to weigh:
    the plain regression is exact, so the answer is 0 instead of 0 / 0.
    """
    total = residual + counts * group
    scattered = total > 0
    ratio = np.divide(residual, total, out=np.ones_like(total), where=scattered)
    shrink: FloatArray = np.where(scattered, 1.0 - np.sqrt(ratio), 0.0)
    return shrink


def fit_random_intercept(y: FloatArray, x: FloatArray, groups: IntArray) -> Fit:
    """Fit y = intercept + x @ b + u[group] + e. `groups` holds codes 0..G-1."""
    counts = np.bincount(groups).astype(np.float64)
    if (counts == 0).any():
        raise NotEstimableError("group codes must be contiguous")
    y_means, x_means = _group_means(y, groups, counts), _group_means(x, groups, counts)
    within_y, within_x = y - y_means[groups], x - x_means[groups]
    _, residual = _residual_variance(within_y, within_x, len(counts))

    group = _group_variance(y_means, x_means, counts, residual)
    # Corner levels that are not identified are used as fixed effects (no shrinkage).
    fixed = np.ones(len(counts))
    theta = fixed if group is None else _shrinkage(residual, group, counts)
    weight = theta[groups]
    design = np.column_stack([1.0 - weight, x - weight[:, None] * x_means[groups]])
    coefficients, *_ = np.linalg.lstsq(design, y - weight * y_means[groups], rcond=None)
    return Fit(
        coefficients=coefficients[1:],
        sigma_residual=float(np.sqrt(residual)),
        sigma_group=float(np.sqrt(group)) if group is not None else float("nan"),
    )
