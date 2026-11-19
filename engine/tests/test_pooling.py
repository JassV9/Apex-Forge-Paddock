"""Random-effects pooling with known truth (ADR 0033, finding I1).

Simulated experiments on one change: each has its own true effect, drawn around the pooled effect
with a between-experiment spread tau, and is measured with a block-level standard error that is
itself estimated (5 degrees of freedom, as an 8-block experiment's). The standard errors span
2 to 6 ms, as the FH26's experiments do; the margin is the corner-time margin, 5 ms.
"""

from collections.abc import Callable

import numpy as np
import pytest
from scipy import stats

from paddock_engine.contracts.sensitivity_response_schema import Method
from paddock_engine.sensitivity.half_normal import half_normal
from paddock_engine.sensitivity.knowledge import combine
from paddock_engine.sensitivity.pooling import (
    Estimate,
    Pooled,
    from_interval,
    hksj,
    paule_mandel,
    pooling_error,
    quantile,
)

MARGIN, LEVEL, DF = 0.005, 0.90, 5
RUNS = 800


def simulate(k: int, tau: float, seed: int) -> list[list[Estimate]]:
    rng = np.random.default_rng(seed)
    out = []
    for _ in range(RUNS):
        true_se = rng.uniform(0.002, 0.006, k)
        effects = rng.normal(0.0, true_se) + rng.normal(0.0, tau, k)
        reported = true_se * np.sqrt(rng.chisquare(DF, k) / DF)
        out.append(
            [Estimate(float(y), float(s), DF) for y, s in zip(effects, reported, strict=True)]
        )
    return out


def coverage(
    method: Callable[[list[Estimate]], tuple[float, float]], runs: list[list[Estimate]]
) -> float:
    return float(np.mean([low <= 0.0 <= high for low, high in map(method, runs)]))


def random_effects(estimates: list[Estimate]) -> tuple[float, float]:
    pooled = hksj(estimates, LEVEL)
    return pooled.low, pooled.high


def fixed_effect(estimates: list[Estimate]) -> tuple[float, float]:
    """ADR 0011's pooling: inverse-variance weights, tau = 0, a z quantile."""
    w = np.array([1.0 / pooling_error(e, LEVEL) ** 2 for e in estimates])
    mean = float(w @ np.array([e.mean for e in estimates]) / w.sum())
    half = quantile(LEVEL) / float(np.sqrt(w.sum()))
    return mean - half, mean + half


@pytest.mark.parametrize("k", [2, 3, 4, 5, 6])
@pytest.mark.parametrize("tau_share", [0.0, 0.5, 1.0])
def test_hksj_covers_at_least_88_percent_where_fixed_effect_collapses(
    k: int, tau_share: float
) -> None:
    runs = simulate(k, tau_share * MARGIN, seed=100 * k + int(10 * tau_share))

    assert coverage(random_effects, runs) >= 0.88
    if tau_share == 1.0 and k >= 4:
        assert coverage(fixed_effect, runs) < 0.80  # measured 56 to 74 %


def test_two_experiments_with_a_half_normal_tau_cover_at_least_88_percent() -> None:
    for tau_share in (0.0, 0.5, 1.0):
        runs = simulate(2, tau_share * MARGIN, seed=7 + int(10 * tau_share))
        covered = coverage(lambda e: _ends(half_normal(e, MARGIN, LEVEL)), runs)
        assert covered >= 0.88, tau_share


def _ends(pooled: Pooled) -> tuple[float, float]:
    return pooled.low, pooled.high


def test_the_prediction_interval_covers_a_new_experiments_true_effect() -> None:
    rng = np.random.default_rng(3)
    runs = simulate(5, MARGIN, seed=11)
    hits = []
    for estimates in runs:
        pooled = hksj(estimates, LEVEL)
        assert pooled.prediction is not None
        low, high = pooled.prediction
        hits.append(low <= rng.normal(0.0, MARGIN) <= high)
    # Measured 83 %: the t(k - 2) prediction interval under-covers with few experiments because
    # tau is estimated from them (a known property of Higgins' interval). Reported, never decides.
    assert np.mean(hits) >= 0.80


def test_paule_mandel_solves_the_generalised_q_and_agrees_with_statsmodels() -> None:
    from statsmodels.stats.meta_analysis import combine_effects

    y = np.array([-0.00103, -0.00467, -0.00365, 0.00129, -0.009])
    v = np.array([0.0045, 0.0066, 0.0030, 0.0045, 0.0021]) ** 2

    tau2 = paule_mandel(y, v)

    w = 1 / (v + tau2)
    mean = w @ y / w.sum()
    assert w @ (y - mean) ** 2 == pytest.approx(len(y) - 1, rel=1e-6)
    reference = combine_effects(y, v, method_re="pm")
    assert tau2 == pytest.approx(reference.tau2, rel=1e-3)
    assert paule_mandel(np.array([0.0, 0.0001]), np.array([1e-4, 1e-4])) == 0.0


def test_hksj_never_narrower_than_the_plain_random_effects_interval() -> None:
    agreeing = [Estimate(-0.004, 0.003), Estimate(-0.0041, 0.003), Estimate(-0.0039, 0.003)]

    pooled = hksj(agreeing, LEVEL)

    plain_half = quantile(LEVEL, 2) * 0.003 / np.sqrt(3)
    assert pooled.tau == 0.0 and pooled.method is Method.hksj
    assert pooled.high - pooled.low == pytest.approx(2 * plain_half)  # the max(1, q) floor


def test_an_interval_is_read_back_with_the_quantile_it_was_made_with() -> None:
    t = stats.t.ppf(0.95, 5)

    assert from_interval(0.0, -t, t, LEVEL, df=5).se == pytest.approx(1.0)
    assert from_interval(0.0, -1.645, 1.645, LEVEL).se == pytest.approx(1.0, abs=1e-3)
    assert from_interval(0.0, -t, t, LEVEL).se > 1.2  # the inversion I1 found: z on a t interval


def test_combine_dispatches_by_the_number_of_experiments() -> None:
    one, two = Estimate(-0.004, 0.002, 5), Estimate(-0.006, 0.003, 5)

    assert combine([one], MARGIN, LEVEL).method is Method.single
    assert combine([one], MARGIN, LEVEL).low == pytest.approx(one.interval(LEVEL)[0])
    assert combine([one, two], MARGIN, LEVEL).method is Method.half_normal
    assert combine([one, two, one], MARGIN, LEVEL).method is Method.hksj
    assert combine([one, two], None, LEVEL).method is Method.half_normal  # scale: largest error
    with pytest.raises(ValueError, match="nothing to pool"):
        combine([], MARGIN, LEVEL)


def test_a_four_block_experiment_counts_far_less_than_its_raw_error_says() -> None:
    four_blocks, ten_blocks = Estimate(-0.001, 0.002, 1), Estimate(-0.004, 0.0025, 6)

    weight = (pooling_error(ten_blocks, LEVEL) / pooling_error(four_blocks, LEVEL)) ** 2

    assert weight < 0.15 * (0.0025 / 0.002) ** 2
