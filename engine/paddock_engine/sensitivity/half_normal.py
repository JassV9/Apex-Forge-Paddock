"""Two experiments pooled with a half-normal prior on tau (ADR 0033).

Two estimates say almost nothing about how much experiments differ: Paule-Mandel gives tau = 0
whenever they happen to agree, and HKSJ's t(1) quantile (6.3 at 90 %) makes the interval useless
when they do not. So tau gets a prior instead: half-normal with its scale at the channel's margin,
"experiments on the same change usually differ by less than an effect that matters". The pooled
mean has a flat prior. The posterior of the mean is a mixture over tau of the normal
random-effects posteriors, each weighted by how well that tau explains the two estimates; the
interval is the mixture's central `level` interval, the standard error its standard deviation and
`p_increase` its probability of an effect at most zero.

The result is a credible interval, reported with the same fields as the others. It never makes a
pooled confirmation (sensitivity/knowledge.py): two experiments are preliminary at best.
"""

from collections.abc import Sequence

import numpy as np
from scipy import optimize, stats

from paddock_engine.contracts.sensitivity_response_schema import Method
from paddock_engine.sensitivity.pooling import Estimate, Pooled, pooling_error

_NODES = 400
_PRIOR_REACH = 8.0  # prior scales the tau grid covers
_SEARCH = 12.0  # posterior standard deviations the quantile search spans


def _posterior(y: np.ndarray, v: np.ndarray, scale: float) -> tuple[np.ndarray, ...]:
    """Weights, means and variances of the mixture components, and the tau grid."""
    tau = np.linspace(0.0, _PRIOR_REACH * scale, _NODES)
    w = 1.0 / (v[None, :] + tau[:, None] ** 2)
    total = w.sum(axis=1)
    means = (w @ y) / total
    q = (w * (y[None, :] - means[:, None]) ** 2).sum(axis=1)
    log_like = 0.5 * np.log(w).sum(axis=1) - 0.5 * np.log(total) - 0.5 * q
    log_post = log_like - 0.5 * (tau / scale) ** 2
    weights = np.exp(log_post - log_post.max())
    return weights / weights.sum(), means, 1.0 / total, tau


def _cdf(x: float, weights: np.ndarray, means: np.ndarray, variances: np.ndarray) -> float:
    return float(weights @ stats.norm.cdf((x - means) / np.sqrt(variances)))


def half_normal(estimates: Sequence[Estimate], scale: float, level: float) -> Pooled:
    """Random-effects pooling of a few estimates with tau ~ half-normal(`scale`)."""
    y = np.array([e.mean for e in estimates])
    v = np.array([pooling_error(e, level) ** 2 for e in estimates])
    weights, means, variances, tau = _posterior(y, v, scale)
    mean = float(weights @ means)
    sd = float(np.sqrt(weights @ (variances + means**2) - mean**2))
    reach = _SEARCH * max(sd, float(np.sqrt(variances.max())))

    def at(probability: float) -> float:
        return float(
            optimize.brentq(
                lambda x: _cdf(x, weights, means, variances) - probability,
                mean - reach,
                mean + reach,
            )
        )

    tail = (1.0 - level) / 2.0
    return Pooled(
        estimate=Estimate(mean=mean, se=sd),
        low=at(tail),
        high=at(1.0 - tail),
        tau=float(weights @ tau),
        method=Method.half_normal,
        experiments=len(estimates),
        p_increase=_cdf(0.0, weights, means, variances),
    )
