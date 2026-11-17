"""Plans sized for the noise of whole laps (ADR 0025): the sizing rule and the lap estimate."""

import math
from typing import Any

import numpy as np
import pytest
from hypothesis import given, settings
from hypothesis import strategies as st
from simulation import BASELINE, simulate_abab

from paddock_engine.contracts.plan_request_schema import PlanRequest
from paddock_engine.contracts.plan_response_schema import Arm, SigmaLapSource
from paddock_engine.experiments.plan import (
    MAX_BLOCKS,
    STANDARD_BLOCKS,
    decision_effect,
    plan,
    size,
    time_inflation,
)
from paddock_engine.stats.effects import estimate_effects
from paddock_engine.stats.lap_noise import MIN_LAPS, LapPass, lap_scatter
from paddock_engine.stats.mde import lap_inclusive_sigma, minimum_detectable_effect

ROPE, LEVEL = 0.005, 0.9
sigmas = st.floats(0.002, 0.15)
lap_sigmas = st.floats(0.0, 0.08)
corner_counts = st.integers(3, 20)


@given(sigma=sigmas, corners=corner_counts, laps=st.integers(2, 5))
def test_without_lap_noise_the_plan_is_the_standard_one(
    sigma: float, corners: int, laps: int
) -> None:
    sizing = size(sigma, 0.0, corners, laps, ROPE, LEVEL)

    assert sizing.blocks == STANDARD_BLOCKS and sizing.met
    assert sizing.mde_detect == pytest.approx(minimum_detectable_effect(sigma, 2 * laps, corners))
    standard = decision_effect(sigma, 0.0, corners, laps, STANDARD_BLOCKS, ROPE, LEVEL)
    assert sizing.mde_decide == standard <= sizing.target


@given(sigma=sigmas, low=lap_sigmas, high=lap_sigmas, corners=corner_counts)
def test_more_lap_noise_never_means_fewer_blocks(
    sigma: float, low: float, high: float, corners: int
) -> None:
    low, high = sorted((low, high))
    quiet, noisy = (
        size(sigma, low, corners, 3, ROPE, LEVEL),
        size(sigma, high, corners, 3, ROPE, LEVEL),
    )

    assert quiet.blocks <= noisy.blocks
    for blocks in range(STANDARD_BLOCKS, MAX_BLOCKS + 1, 2):
        less = decision_effect(sigma, low, corners, 3, blocks, ROPE, LEVEL)
        assert less <= decision_effect(sigma, high, corners, 3, blocks, ROPE, LEVEL) * (1 + 1e-12)


@settings(max_examples=50)
@given(sigma=sigmas, lap=lap_sigmas, corners=corner_counts, scale=st.floats(1e-3, 1e3))
def test_the_plan_does_not_depend_on_the_unit(
    sigma: float, lap: float, corners: int, scale: float
) -> None:
    seconds = size(sigma, lap, corners, 3, ROPE, LEVEL)
    other = size(sigma * scale, lap * scale, corners, 3, ROPE * scale, LEVEL)

    assert (other.blocks, other.met) == (seconds.blocks, seconds.met)
    assert other.mde_decide == pytest.approx(seconds.mde_decide * scale, rel=1e-9)
    assert other.target == pytest.approx(seconds.target * scale, rel=1e-9)


def test_a_car_whose_laps_scatter_gets_more_blocks_up_to_the_longest_plan() -> None:
    ferrari = size(0.025, 0.0026, 17, 3, ROPE, LEVEL)  # experiment 5's baseline
    hybrid = size(0.0336, 0.0092, 14, 3, ROPE, LEVEL)  # experiment 10's
    hopeless = size(0.0385, 0.10, 14, 3, ROPE, LEVEL)

    assert (ferrari.blocks, ferrari.met) == (4, True)
    assert (hybrid.blocks, hybrid.met) == (6, True)
    assert (hopeless.blocks, hopeless.met) == (MAX_BLOCKS, False)
    assert hopeless.mde_decide > hopeless.target


def test_lap_noise_counts_once_per_lap_not_once_per_pass() -> None:
    assert lap_inclusive_sigma(0.03, 0.0, 14) == 0.03
    # 14 corners of a lap share its 20 ms: as passes of sqrt(30^2 - 20^2 + 14 x 20^2) ms each
    assert lap_inclusive_sigma(0.03, 0.02, 14) == pytest.approx(math.sqrt(0.0005 + 14 * 0.0004))
    assert time_inflation(4) == pytest.approx(1.118, abs=1e-3)
    assert time_inflation(8) < time_inflation(6) < time_inflation(4)


def simulated_laps(
    rng: np.random.Generator, laps: int, corners: int, sigma: float, lap_sigma: float
) -> list[LapPass]:
    levels = rng.uniform(3.0, 11.0, corners)
    return [
        LapPass(corner + 1, lap + 1, float(levels[corner] + shared + rng.normal(0.0, sigma)))
        for lap in range(laps)
        for shared in [rng.normal(0.0, lap_sigma)]
        for corner in range(corners)
    ]


def scatter_of(passes: list[LapPass]) -> float:
    sigma = {
        c: float(np.std([p.value for p in passes if p.corner == c]))
        for c in {p.corner for p in passes}
    }
    return lap_scatter(passes, sigma)[0]


def test_the_lap_estimate_finds_lap_noise_and_no_more() -> None:
    estimates = {
        lap: [
            scatter_of(simulated_laps(np.random.default_rng(seed), 30, 14, 0.03, lap))
            for seed in range(40)
        ]
        for lap in (0.0, 0.02)
    }

    assert np.median(estimates[0.0]) < 0.005
    assert 0.015 < np.median(estimates[0.02]) < 0.025


@settings(max_examples=25, deadline=None)
@given(seed=st.integers(0, 2**31), scale=st.floats(1e-2, 1e3))
def test_the_lap_estimate_does_not_depend_on_the_unit(seed: int, scale: float) -> None:
    passes = simulated_laps(np.random.default_rng(seed), 12, 10, 0.03, 0.015)
    scaled = [LapPass(p.corner, p.lap, p.value * scale) for p in passes]

    assert scatter_of(scaled) == pytest.approx(scatter_of(passes) * scale, rel=1e-6, abs=1e-12)


def test_too_few_laps_cannot_tell() -> None:
    passes = simulated_laps(np.random.default_rng(1), MIN_LAPS - 1, 14, 0.03, 0.02)
    with pytest.raises(ValueError):
        scatter_of(passes)


def request(passes: list[dict[str, Any]]) -> PlanRequest:
    return PlanRequest.model_validate(
        {
            "success_test": {
                "channel": "corner_time_s",
                "rope": ROPE,
                "level": LEVEL,
                "better": "lower",
            },
            "corners": list(range(1, 15)),
            "laps_per_block": 3,
            "baseline_passes": passes,
        }
    )


def as_plan_passes(
    passes: list[LapPass], planned: bool, first_lap: int = 0
) -> list[dict[str, Any]]:
    return [
        {"corner": p.corner, "value": p.value, "lap": p.lap + first_lap, "planned": planned}
        for p in passes
    ]


def test_laps_driven_to_a_run_plan_come_before_free_laps() -> None:
    rng = np.random.default_rng(7)
    calibration = as_plan_passes(simulated_laps(rng, 20, 14, 0.03, 0.06), planned=False)
    steady = as_plan_passes(simulated_laps(rng, 12, 14, 0.03, 0.0), planned=True, first_lap=100)

    with_plans = plan(request(calibration + steady))
    without = plan(request(calibration))
    nothing = plan(
        request([{k: v for k, v in p.items() if k in ("corner", "value")} for p in calibration])
    )

    assert with_plans.sigma_lap_source is SigmaLapSource.planned and with_plans.sigma_lap < 0.01
    assert without.sigma_lap_source is SigmaLapSource.free and without.sigma_lap > 0.03
    assert len(without.blocks) > len(with_plans.blocks) == STANDARD_BLOCKS
    assert nothing.sigma_lap_source is SigmaLapSource.none and nothing.sigma_lap == 0.0
    assert [b.arm for b in without.blocks] == [Arm.baseline, Arm.variant] * (
        len(without.blocks) // 2
    )
    assert without.passes_per_arm == 3 * len(without.blocks) // 2


@pytest.mark.parametrize("blocks", [4, 8])
def test_the_planned_standard_error_is_what_simulated_experiments_show(blocks: int) -> None:
    """With pass and lap noise, the estimates scatter as the planner says (time-adjusted)."""
    sigma, lap, corners = 0.028, 0.015, 14
    rng = np.random.default_rng(20261117 + blocks)
    estimates = [
        estimate_effects(
            simulate_abab(rng, 0.0, corners, sigma=sigma, lap_sigma=lap, blocks=blocks),
            BASELINE,
            ROPE,
            replicates=40,
        )[0].estimate
        for _ in range(150)
    ]
    planned = (
        time_inflation(blocks)
        * lap_inclusive_sigma(sigma, 0.0, corners)
        * math.sqrt(2.0 / (3 * blocks // 2 * corners))
    )
    with_laps = (
        time_inflation(blocks)
        * math.sqrt(2.0 / (3 * blocks // 2))
        * math.sqrt(sigma**2 / corners + lap**2)
    )

    assert np.std(estimates) == pytest.approx(with_laps, rel=0.2)
    assert np.std(estimates) > 1.5 * planned
