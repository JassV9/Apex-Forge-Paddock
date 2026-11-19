"""The driver-input check against its margins (ADR 0034, amendment): a driver input has shifted
only when Holm rejects "no change" and the whole interval lies beyond the channel's margin.

Experiment 27 (identical arms, the AI at hint 0.97) put the braking point at -0.27 m
[-0.43, -0.11] and flagged it; a 27 cm wobble against a 1 m margin is not adaptation."""

from typing import Any

import numpy as np
import pytest
from simulation import BASELINE, simulate_abab

from paddock_engine.config.margins import margins
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import EvaluateResponse, Kind, Role
from paddock_engine.experiments.evaluate import evaluate

CORNERS = 10
TIME = "corner_time_s"
BRAKE = "brake_point_m"
TEST = {"channel": TIME, "rope": 0.005, "level": 0.9, "better": "lower"}
BRAKE_SIGMA = 0.6  # m per pass: an interval about +-0.15 m at 8 blocks, as in experiment 27
TRIALS = 200


def passes_with(channel: str, shift: float, sigma: float, rng: np.random.Generator) -> list[Any]:
    """8 ABAB blocks: corner time with no effect, `channel` moved by `shift` in the variant."""
    times = simulate_abab(rng, 0.0, CORNERS, sigma=0.03, blocks=8)
    inputs = simulate_abab(rng, shift, CORNERS, sigma=sigma, blocks=8)
    return [
        {
            "corner": t.corner,
            "arm": "baseline" if t.arm == BASELINE else "variant",
            "block": t.block + 1,
            "lap": t.lap,
            "metrics": {TIME: t.value, channel: i.value},
            "covariates": {},
        }
        for t, i in zip(times, inputs, strict=True)
    ]


def judge(passes: list[Any], channel: str) -> EvaluateResponse:
    request = {
        "success_test": TEST,
        "guardrails": [],
        "predictions": [],
        "driver_channels": [channel],
        "passes": passes,
    }
    return evaluate(EvaluateRequest.model_validate(request))


def flag_rate(shift: float, seed: int) -> float:
    rng = np.random.default_rng(seed)
    runs = [judge(passes_with(BRAKE, shift, BRAKE_SIGMA, rng), BRAKE) for _ in range(TRIALS)]
    return float(np.mean([v.driver_adapted for v in runs]))


def test_every_driver_channel_core_sends_has_a_margin() -> None:
    """core/internal/experiments: driverChannels and their absolute replacements."""
    channels = ("brake_dist_m", "throttle_pickup_m", "steer_corrections_per_s")
    absolute = ("brake_point_m", "throttle_pickup_point_m")
    assert all((margins().metrics.get(c) or 0) > 0 for c in channels + absolute)


def test_a_braking_point_wobble_inside_its_margin_is_not_driver_adaptation() -> None:
    verdict = judge(passes_with(BRAKE, -0.3, 0.2, np.random.default_rng(5)), BRAKE)

    (result,) = [r for r in verdict.results if r.role is Role.driver_input]
    assert result.ci_high < 0  # it did move: against zero alone it would be flagged
    assert not verdict.driver_adapted
    assert all(c.kind is not Kind.driver_input_shifted for c in verdict.confounders or [])
    assert BRAKE not in verdict.reason


def test_a_braking_point_shift_beyond_its_margin_is_still_flagged() -> None:
    verdict = judge(passes_with(BRAKE, -2.5, 0.2, np.random.default_rng(5)), BRAKE)

    (result,) = [r for r in verdict.results if r.role is Role.driver_input]
    assert verdict.driver_adapted
    assert BRAKE in verdict.reason
    assert result.effect_class.value == "negative"  # judged against the margin, not zero


def test_a_channel_without_a_margin_is_judged_against_zero_as_before() -> None:
    verdict = judge(passes_with("throttle_pickup_s", 0.2, 0.05, np.random.default_rng(5)),
                    "throttle_pickup_s")  # fmt: skip

    assert verdict.driver_adapted


@pytest.mark.parametrize(
    ("shift", "low", "high"),
    [(0.0, 0.0, 0.02), (-0.3, 0.0, 0.02), (-1.0, 0.0, 0.15), (-1.5, 0.95, 1.0), (-2.0, 0.95, 1.0)],
)
def test_how_often_a_braking_point_shift_is_flagged(shift: float, low: float, high: float) -> None:
    """Measured: 0, 0, 5.5, 100 and 100 % (zero alone: 11, 96.5, 100, 100, 100 %). Identical
    setups and a 0.3 margin wobble never; one margin is the boundary; 1.5 margins always."""
    assert low <= flag_rate(shift, seed=int(10 * abs(shift)) + 1) <= high
