"""Which covariates the model may adjust for (ADR 0022): never one the arm cannot be told from."""

import numpy as np
import pytest
from hypothesis import given, settings
from hypothesis import strategies as st
from simulation import BASELINE, simulate_restarted_abab

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.effects import estimate_effects
from paddock_engine.stats.observations import (
    MAX_VARIANCE_INFLATION,
    Design,
    Observation,
    _variance_inflation,
    build_design,
)


@pytest.mark.parametrize("seed", range(5))
def test_a_restarted_experiment_keeps_a_narrow_interval(seed: int) -> None:
    """Experiment 6: time and fuel were correlated (0.94) but not copies, and with the clock
    and the tank reset by each new session both were entangled with which setup was on the car.
    The interval was +-450 ms around an effect of 0; it must stay within a few milliseconds."""
    observations = simulate_restarted_abab(np.random.default_rng(seed), effect=0.0)

    design = build_design(observations, BASELINE)
    (effect,) = estimate_effects(observations, BASELINE, rope=0.005)

    assert (design.covariates, design.dropped) == (["time_in_session_s"], ["fuel_l"])
    assert effect.dropped_covariates == ["fuel_l"]
    assert effect.ci_high - effect.ci_low < 0.03
    assert effect.ci_low < 0.0 < effect.ci_high


def test_a_covariate_that_only_repeats_the_arm_is_dropped_and_the_arm_stays() -> None:
    rng = np.random.default_rng(3)
    observations = [
        Observation(
            corner,
            arm,
            5.0 + corner + rng.normal(0.0, 0.01),
            block=index // 4,
            lap=index,
            covariates={"fuel": 40.0 + (arm == "B") + rng.normal(0.0, 0.01)},
        )
        for corner in (1, 2, 3)
        for index, arm in enumerate(["A", "B", "A", "B", "A", "B", "A", "B"])
    ]

    design = build_design(observations, "A")

    assert design.dropped == ["fuel"]
    assert design.covariates == []
    assert design.arms == ["B"]
    assert design.x.shape == (len(observations), 1)


def _observations(seed: int, weights: list[float], scales: list[float]) -> list[Observation]:
    """Covariates that are mixtures of the arm, time and noise, in arbitrary units."""
    rng = np.random.default_rng(seed)
    rows = []
    for corner in range(1, 5):
        for index in range(12):
            arm = "B" if (index // 3) % 2 else "A"
            signals = (float(arm == "B"), float(index), rng.normal())
            covariates = {
                f"c{k}": scales[k] * (w * signals[0] + (1.0 - w) * signals[1] + 0.05 * signals[2])
                for k, w in enumerate(weights)
            }
            rows.append(Observation(corner, arm, 5.0 + corner, index // 6, index, covariates))
    return rows


@settings(max_examples=60, deadline=None)
@given(
    seed=st.integers(0, 1000),
    weights=st.lists(st.floats(0.0, 1.0), min_size=1, max_size=3),
    scale=st.floats(1e-3, 1e3),
)
def test_what_is_kept_is_never_tangled_and_units_do_not_matter(
    seed: int, weights: list[float], scale: float
) -> None:
    unit = _observations(seed, weights, [1.0] * len(weights))
    rescaled = _observations(seed, weights, [scale] * len(weights))

    design = build_design(unit, "A")
    other = build_design(rescaled, "A")

    assert (design.covariates, design.dropped) == (other.covariates, other.dropped)
    assert sorted(design.covariates + design.dropped) == [f"c{k}" for k in range(len(weights))]
    assert design.x.shape[1] == len(design.arms) + len(design.covariates)
    assert _variance_inflation(design.x - _corner_means(design)).max() <= MAX_VARIANCE_INFLATION


def _corner_means(design: Design) -> FloatArray:
    """The design centred within corners, as the VIF is judged."""
    counts = np.bincount(design.groups)[:, None]
    sums = np.zeros((len(counts), design.x.shape[1]))
    np.add.at(sums, design.groups, design.x)
    return (sums / counts)[design.groups]
