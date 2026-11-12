"""The measurement model and its helpers: model fit, design, ROPE, MDE, noise."""

import warnings

import numpy as np
import pytest
import statsmodels.formula.api as smf
from hypothesis import given, settings
from hypothesis import strategies as st
from simulation import BASELINE, SIGMA_S, VARIANT, simulate_abab

from paddock_engine.stats.effects import estimate_effects
from paddock_engine.stats.mde import minimum_detectable_effect, passes_needed
from paddock_engine.stats.model import NotEstimableError, fit_random_intercept
from paddock_engine.stats.noise import noise_model
from paddock_engine.stats.observations import Observation, build_design
from paddock_engine.stats.rope import EffectClass, classify


def test_fit_agrees_with_statsmodels_mixedlm() -> None:
    """Same model, independent implementation: REML mixed model with a random corner intercept."""
    rng = np.random.default_rng(11)
    observations = simulate_abab(rng, effect=0.030, corners=8, drift_per_lap=-0.004)
    design = build_design(observations, BASELINE)

    ours = fit_random_intercept(design.y, design.x, design.groups)

    frame = {
        "y": design.y, "arm": design.x[:, 0], "time": design.x[:, 1], "corner": design.groups,
    }  # fmt: skip
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")
        theirs = smf.mixedlm("y ~ arm + time", frame, groups=frame["corner"]).fit(reml=True)
    assert ours.coefficients[0] == pytest.approx(theirs.params["arm"], abs=1e-5)
    assert ours.coefficients[1] == pytest.approx(theirs.params["time"], abs=1e-5)
    assert ours.sigma_residual == pytest.approx(np.sqrt(theirs.scale), rel=0.02)
    assert ours.sigma_residual == pytest.approx(SIGMA_S, rel=0.25)


@settings(max_examples=40, deadline=None)
@given(
    effect=st.floats(-0.2, 0.2),
    shift=st.floats(-50, 50),
    seed=st.integers(0, 10_000),
)
def test_estimate_recovers_the_effect_and_ignores_a_common_shift(
    effect: float, shift: float, seed: int
) -> None:
    observations = simulate_abab(np.random.default_rng(seed), effect, corners=6, sigma=1e-6)
    shifted = [
        Observation(o.corner, o.arm, o.value + shift, o.block, o.lap, o.covariates)
        for o in observations
    ]

    plain = build_design(observations, BASELINE)
    moved = build_design(shifted, BASELINE)

    first = fit_random_intercept(plain.y, plain.x, plain.groups).coefficients[0]
    second = fit_random_intercept(moved.y, moved.x, moved.groups).coefficients[0]
    assert first == pytest.approx(effect, abs=1e-4)
    assert second == pytest.approx(first, abs=1e-6)


def test_same_passes_always_give_the_same_answer() -> None:
    observations = simulate_abab(np.random.default_rng(5), 0.02, corners=10)

    assert estimate_effects(observations, BASELINE, rope=0.005) == estimate_effects(
        observations, BASELINE, rope=0.005
    )


def test_effect_reports_what_it_was_computed_from() -> None:
    observations = simulate_abab(np.random.default_rng(6), 0.05, corners=10)

    (effect,) = estimate_effects(observations, BASELINE, rope=0.005)

    assert effect.arm == VARIANT
    assert (effect.n_passes, effect.corners) == (120, 10)
    assert effect.covariates == ["time_in_session"]
    assert effect.ci_low < effect.estimate < effect.ci_high
    assert effect.effect_class is EffectClass.POSITIVE


def test_design_drops_covariates_that_cannot_help() -> None:
    def row(corner: int, arm: str, lap: int, **covariates: float) -> Observation:
        return Observation(corner, arm, 5.0 + lap, lap=lap, covariates=covariates)

    observations = [
        row(c, arm, lap, fuel=40.0 - lap, time=float(lap), grip=0.97, tyre=80.0 + (lap % 3))
        for c in (1, 2)
        for lap, arm in enumerate(["A", "B", "A", "B", "A", "B"], start=1)
    ]
    first = {"fuel": 39.0, "time": 1.0, "grip": 0.97, "tyre": 81.0}
    observations[0] = Observation(1, "A", 6.0, lap=1, covariates=first)

    design = build_design(observations, "A")

    # grip never varies; time duplicates fuel (it burns linearly); "missing" is not on every pass
    assert design.covariates == ["fuel", "tyre"]
    assert design.x.shape == (12, 3)


def test_design_needs_both_arms() -> None:
    only_baseline = [Observation(1, "A", 5.0), Observation(1, "A", 5.1)]
    with pytest.raises(ValueError, match="baseline"):
        build_design(only_baseline, "A")
    with pytest.raises(ValueError, match="baseline"):
        build_design(only_baseline, "B")


def test_too_few_passes_are_reported_not_guessed() -> None:
    observations = [Observation(1, "A", 5.0), Observation(1, "B", 5.1), Observation(2, "A", 7.0)]
    design = build_design(observations, "A")

    with pytest.raises(NotEstimableError):
        fit_random_intercept(design.y, design.x, design.groups)


@pytest.mark.parametrize(
    ("low", "high", "expected"),
    [
        (0.006, 0.020, EffectClass.POSITIVE),
        (-0.020, -0.006, EffectClass.NEGATIVE),
        (-0.004, 0.003, EffectClass.NEGLIGIBLE),
        (-0.005, 0.005, EffectClass.NEGLIGIBLE),
        (0.001, 0.012, EffectClass.INCONCLUSIVE),  # real, but maybe too small to matter
        (-0.012, 0.012, EffectClass.INCONCLUSIVE),
        (0.005, 0.012, EffectClass.INCONCLUSIVE),  # touching the band is not beyond it
    ],
)
def test_classify_against_the_equivalence_band(
    low: float, high: float, expected: EffectClass
) -> None:
    assert classify(low, high, rope=0.005) is expected


def test_classify_rejects_nonsense() -> None:
    with pytest.raises(ValueError, match="invalid"):
        classify(0.2, 0.1, rope=0.005)
    with pytest.raises(ValueError, match="invalid"):
        classify(0.1, 0.2, rope=-1)


@pytest.mark.parametrize(
    ("passes", "corners", "expected_ms"),
    [(6, 10, 14.3), (3, 6, 26.1)],  # the two examples in docs/design.md
)
def test_mde_matches_the_design_document(passes: int, corners: int, expected_ms: float) -> None:
    assert minimum_detectable_effect(28.0, passes, corners) == pytest.approx(expected_ms, abs=0.05)


@given(sigma=st.floats(1, 200), effect=st.floats(1, 100), corners=st.integers(1, 30))
def test_passes_needed_is_the_smallest_budget_that_reaches_the_effect(
    sigma: float, effect: float, corners: int
) -> None:
    needed = passes_needed(sigma, effect, corners)

    assert minimum_detectable_effect(sigma, needed, corners) <= effect * (1 + 1e-9)
    if needed > 1:
        assert minimum_detectable_effect(sigma, needed - 1, corners) > effect


def test_noise_model_is_robust_to_a_few_slow_passes() -> None:
    rng = np.random.default_rng(8)
    observations = [
        Observation(corner, "A", 5.0 + corner + rng.normal(0, 0.010 * corner))
        for corner in (1, 2, 3)
        for _ in range(300)
    ]
    observations += [Observation(1, "A", 9.0), Observation(1, "A", 12.0)]  # two ruined passes
    observations += [Observation(4, "A", 5.0), Observation(4, "A", 5.5)]  # too few to judge

    model = noise_model(observations)

    assert set(model.per_corner) == {1, 2, 3}
    assert model.per_corner[1] == pytest.approx(0.010, rel=0.2)
    assert model.per_corner[3] == pytest.approx(0.030, rel=0.2)
    assert model.typical == model.per_corner[2]
    assert model.quietest(2) == [1, 2]


def test_noise_model_needs_repeated_passes() -> None:
    with pytest.raises(ValueError, match="3 passes"):
        noise_model([Observation(1, "A", 5.0), Observation(2, "A", 6.0)])


def test_a_channel_that_never_moves_has_an_effect_of_exactly_zero() -> None:
    """Counts such as lockups are 0 on every pass of most experiments: that is "no effect",
    not "cannot be estimated"."""
    rows = simulate_abab(np.random.default_rng(1), 0.0, corners=7)
    still = [Observation(r.corner, r.arm, 0.0, r.block, r.lap, r.covariates) for r in rows]

    effect = estimate_effects(still, BASELINE, rope=0.05)[0]

    assert (effect.estimate, effect.ci_low, effect.ci_high) == (0.0, 0.0, 0.0)
    assert effect.effect_class is EffectClass.NEGLIGIBLE
