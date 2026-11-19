"""Corners weighted by their own scatter (ADR 0023): steady corners count fully, wild ones less."""

from collections.abc import Iterator

import numpy as np
import pytest
import statsmodels.api as sm
from hypothesis import given, settings
from hypothesis import strategies as st
from simulation import BASELINE, SIGMA_S, simulate_abab

import paddock_engine.stats.effects as effects_module
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.model import fit_random_intercept
from paddock_engine.stats.observations import Observation, build_design
from paddock_engine.stats.weights import (
    MAX_WEIGHT_RATIO,
    STEADY_UP_TO,
    corner_scales,
    corner_weights,
)

CORNERS = 14
WILD_SCATTER = [SIGMA_S] * (CORNERS - 2) + [10.0 * SIGMA_S] * 2  # corners 13 and 14 at 10x
TRUTH = 0.020
REPS = 100  # simulated experiments per coverage check: the rates are exact (fixed seeds)
BLOCKS = 8  # the first look (ADR 0029)


def _trials(seed: int, scatter: float | list[float]) -> list[Effect]:
    rng = np.random.default_rng(seed)
    rows = [simulate_abab(rng, TRUTH, CORNERS, sigma=scatter, blocks=BLOCKS) for _ in range(REPS)]
    return [estimate_effects(r, BASELINE, rope=0.005)[0] for r in rows]


def _unweighted_trials(seed: int, scatter: float | list[float]) -> list[Effect]:
    """The same experiments with every corner counted alike, as before ADR 0023."""
    with pytest.MonkeyPatch.context() as patch:
        patch.setattr(effects_module, "corner_weights", lambda scales: np.ones(len(scales)))
        return _trials(seed, scatter)


def _widths(effects: list[Effect]) -> float:
    return float(np.mean([effect.ci_high - effect.ci_low for effect in effects]))


def _coverage(effects: list[Effect]) -> float:
    return float(np.mean([effect.ci_low <= TRUTH <= effect.ci_high for effect in effects]))


@pytest.fixture(scope="module")
def wild() -> Iterator[tuple[list[Effect], list[Effect]]]:
    """Weighted and unweighted effects of the same experiments with two wild corners."""
    yield _trials(1, WILD_SCATTER), _unweighted_trials(1, WILD_SCATTER)


def test_two_wild_corners_no_longer_drown_the_effect(
    wild: tuple[list[Effect], list[Effect]],
) -> None:
    weighted, unweighted = wild

    assert _widths(weighted) < 0.5 * _widths(unweighted)


def test_weighting_stays_unbiased_and_keeps_its_coverage(
    wild: tuple[list[Effect], list[Effect]],
) -> None:
    weighted, _ = wild
    estimates = np.array([effect.estimate for effect in weighted])

    assert abs(estimates.mean() - TRUTH) < 3.0 * estimates.std(ddof=1) / np.sqrt(REPS)
    assert 0.84 <= _coverage(weighted) <= 0.98


def test_the_wild_corners_are_the_ones_that_count_less(
    wild: tuple[list[Effect], list[Effect]],
) -> None:
    weighted, _ = wild

    for effect in weighted[:10]:
        weights = effect.corner_weights
        assert sorted(weights, key=lambda corner: (weights[corner], corner))[:2] == [13, 14]
        assert weights[13] == pytest.approx(1.0 / MAX_WEIGHT_RATIO)
        assert max(weights.values()) == 1.0


def test_corners_that_scatter_alike_give_the_unweighted_answer() -> None:
    weighted, unweighted = _trials(2, SIGMA_S), _unweighted_trials(2, SIGMA_S)
    errors = np.array([effect.standard_error for effect in unweighted])
    shifts = np.array([a.estimate - b.estimate for a, b in zip(weighted, unweighted, strict=True)])

    assert np.median(np.abs(shifts) / errors) < 0.05
    assert _widths(weighted) == pytest.approx(_widths(unweighted), rel=0.05)
    assert _coverage(weighted) >= 0.84


@settings(max_examples=40, deadline=None)
@given(seed=st.integers(0, 10_000), unit=st.floats(1e-3, 1e3))
def test_weights_do_not_depend_on_the_unit_of_the_channel(seed: int, unit: float) -> None:
    rows = simulate_abab(np.random.default_rng(seed), TRUTH, CORNERS, sigma=WILD_SCATTER)
    rows = [Observation(r.corner, r.arm, r.value, r.block, r.lap) for r in rows]
    scaled = [Observation(r.corner, r.arm, r.value * unit, r.block, r.lap) for r in rows]

    first = estimate_effects(rows, BASELINE, rope=0.005)[0]
    second = estimate_effects(scaled, BASELINE, rope=0.005 * unit)[0]

    assert second.corner_weights.keys() == first.corner_weights.keys()
    for corner, weight in first.corner_weights.items():
        assert second.corner_weights[corner] == pytest.approx(weight, rel=1e-6)
    assert second.estimate == pytest.approx(first.estimate * unit, rel=1e-6, abs=1e-12)


scale_lists = st.lists(
    st.one_of(st.just(0.0), st.floats(1e-6, 10.0), st.just(float("nan"))), min_size=1, max_size=20
)


@given(scales=scale_lists, unit=st.floats(1e-3, 1e3))
def test_weights_respect_the_floor_and_the_cap(scales: list[float], unit: float) -> None:
    array = np.array(scales)
    weights = corner_weights(array)

    assert weights.shape == array.shape
    assert ((weights >= 1.0 / MAX_WEIGHT_RATIO - 1e-12) & (weights <= 1.0)).all()
    known = array[np.isfinite(array)]
    typical = float(np.median(known)) if len(known) else 0.0
    steady = ~np.isfinite(array) | (array <= STEADY_UP_TO * typical)
    assert (weights[steady] == 1.0).all()  # the floor: steady-looking corners count fully
    assert corner_weights(array * unit) == pytest.approx(weights)


def test_a_corner_that_never_moves_does_not_take_all_the_weight() -> None:
    weights = corner_weights(np.array([0.0, 0.03, 0.03, 0.03, 0.3]))

    assert weights.tolist() == pytest.approx([1.0, 1.0, 1.0, 1.0, 1.0 / MAX_WEIGHT_RATIO])


def test_nothing_is_weighted_when_the_typical_corner_does_not_scatter() -> None:
    assert corner_weights(np.array([0.0, 0.0, 0.5])).tolist() == [1.0, 1.0, 1.0]
    assert corner_weights(np.array([np.nan, np.nan])).tolist() == [1.0, 1.0]


def test_scales_are_standard_deviations_and_need_three_passes() -> None:
    residuals = np.array([0.0, 0.01, -0.01, 0.02, 1.0, 1.0])
    groups = np.array([0, 0, 0, 0, 1, 1], dtype=np.intp)

    scales = corner_scales(residuals, groups)

    assert scales[0] == pytest.approx(np.std([0.0, 0.01, -0.01, 0.02], ddof=1))
    assert np.isnan(scales[1])


def test_a_corner_with_a_few_ruined_passes_counts_less() -> None:
    """The ruined passes stay in the estimate, so their corner carries their variance: a robust
    scale would call it steady and let two mistakes set the interval (ADR 0023)."""
    rows = simulate_abab(np.random.default_rng(6), TRUTH, CORNERS)
    ruined = {index for index, row in enumerate(rows) if row.corner == CORNERS}
    first_two = sorted(ruined)[:2]
    rows = [
        Observation(r.corner, r.arm, r.value + (0.3 if i in first_two else 0.0), r.block, r.lap)
        for i, r in enumerate(rows)
    ]

    (effect,) = estimate_effects(rows, BASELINE, rope=0.005)
    with pytest.MonkeyPatch.context() as patch:
        patch.setattr(effects_module, "corner_weights", lambda scales: np.ones(len(scales)))
        (plain,) = estimate_effects(rows, BASELINE, rope=0.005)

    assert effect.corner_weights[CORNERS] < 0.5
    assert all(effect.corner_weights[c] == 1.0 for c in range(1, CORNERS))
    assert effect.ci_high - effect.ci_low < 0.8 * (plain.ci_high - plain.ci_low)


def test_the_weighted_fit_is_weighted_least_squares_with_corner_levels() -> None:
    """Corner levels seconds apart, scatter milliseconds: the GLS fit is the fixed-effects WLS."""
    rows = simulate_abab(np.random.default_rng(4), TRUTH, CORNERS, sigma=WILD_SCATTER)
    design = build_design(rows, BASELINE)
    weights = np.linspace(0.1, 1.0, CORNERS)

    ours = fit_random_intercept(design.y, design.x, design.groups, weights)

    dummies = np.eye(CORNERS)[design.groups]
    theirs = sm.WLS(design.y, np.column_stack([design.x, dummies]), weights=weights[design.groups])
    assert ours.coefficients == pytest.approx(theirs.fit().params[: design.x.shape[1]], abs=1e-6)


@given(constant=st.floats(1e-3, 1e3), seed=st.integers(0, 1000))
@settings(max_examples=30, deadline=None)
def test_equal_weights_are_no_weights(constant: float, seed: int) -> None:
    rows = simulate_abab(np.random.default_rng(seed), TRUTH, 6)
    design = build_design(rows, BASELINE)

    plain = fit_random_intercept(design.y, design.x, design.groups)
    same = fit_random_intercept(design.y, design.x, design.groups, np.full(6, constant))

    assert same.coefficients == pytest.approx(plain.coefficients, abs=1e-9)
    assert same.sigma_residual == pytest.approx(plain.sigma_residual * np.sqrt(constant))


@pytest.mark.parametrize(
    ("scatter", "note"),
    [
        (WILD_SCATTER, " Corners weighted by their own scatter (ADR 0023); least weight: 13, 14."),
        (
            [SIGMA_S] * (CORNERS - 2) + [2.0 * SIGMA_S] * 2,
            " Corners weighted by their own scatter (ADR 0023); none counted below half.",
        ),
        (SIGMA_S, ""),  # every corner counted fully: nothing to say
    ],
    ids=["two wild corners", "two somewhat noisier", "corners alike"],
)
def test_the_verdict_says_when_corners_were_weighted(
    scatter: float | list[float], note: str
) -> None:
    rows = simulate_abab(np.random.default_rng(5), -0.060, CORNERS, sigma=scatter, blocks=BLOCKS)
    passes = [
        {
            "corner": row.corner,
            "arm": "baseline" if row.arm == BASELINE else "variant",
            "block": row.block + 1,
            "lap": row.lap,
            "metrics": {"corner_time_s": row.value},
            "covariates": dict(row.covariates),
        }
        for row in rows
    ]
    test = {"channel": "corner_time_s", "rope": 0.005, "level": 0.9, "better": "lower"}
    request = {"success_test": test, "guardrails": [], "predictions": [], "driver_channels": []}

    verdict = evaluate(EvaluateRequest.model_validate(request | {"passes": passes}))

    assert verdict.reason.endswith("Guardrails hold." + note)
