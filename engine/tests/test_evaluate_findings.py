"""Verdicts with known truth (ADR 0029): context-dependent effects, neutrality, the classes that
are not stored, off-track passes, excluded passes and the driver-input family."""

from functools import cache
from typing import Any

import numpy as np
import pytest
from block_simulation import CLASSES, Design, as_passes, simulate

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import (
    CornerClass,
    EvaluateResponse,
    FindingClass,
    Kind,
    Role,
    Status,
)
from paddock_engine.experiments.evaluate import evaluate

TEST = {"channel": "corner_time_s", "rope": 0.005, "level": 0.9, "better": "lower"}
TRIALS = 200


def judge(passes: list[dict[str, Any]], **overrides: Any) -> EvaluateResponse:
    request = {
        "success_test": TEST,
        "guardrails": [],
        "predictions": [],
        "driver_channels": [],
        "passes": passes,
        "look": 1,
        "extensions_left": 2,
    } | overrides
    return evaluate(EvaluateRequest.model_validate(request))


def interacting(verdict: EvaluateResponse) -> bool:
    return any(c.kind is Kind.speed_class_interaction for c in verdict.confounders or [])


@cache
def context_runs(slow: float, fast: float) -> list[EvaluateResponse]:
    """8 blocks on a car with half the Formula Hybrid's pass scatter (3 to 73 ms). With all of
    it the same interaction is found in 37 % at 8 blocks and 53 % at 12 (ADR 0029)."""
    rng = np.random.default_rng(int(1000 * (slow + 1) + fast * 100))
    design = Design(class_effects={"slow": slow, "fast": fast}, noise_scale=0.5)
    return [judge(as_passes(simulate(rng, design))) for _ in range(TRIALS)]


def test_opposite_effects_in_slow_and_fast_corners_are_found_context_dependent() -> None:
    runs = context_runs(+0.010, -0.010)

    flagged = np.mean([interacting(v) for v in runs])
    context = np.mean([v.finding_class is FindingClass.context_dependent for v in runs])

    assert flagged >= 0.80
    assert context >= 0.80
    names = {
        c.name for v in runs for c in v.confounders or [] if c.kind is Kind.speed_class_interaction
    }
    assert "fast vs slow" in names


def test_without_an_interaction_few_experiments_are_called_context_dependent() -> None:
    runs = context_runs(0.0, 0.0)

    assert np.mean([interacting(v) for v in runs]) <= 0.05


def test_a_speed_class_with_too_few_corners_is_not_a_result() -> None:
    passes = as_passes(simulate(np.random.default_rng(8), Design(effect=-0.02)))
    for item in passes:  # only corners 2 and 5 are fast; slow and medium keep their corners
        corner = item["corner"]
        item["speed_class"] = "fast" if corner in (2, 5) else ("slow" if corner % 2 else "medium")

    verdict = judge(passes)

    classes = [r.corner_class for r in verdict.results if r.role is Role.primary]
    assert classes == [CornerClass.all, CornerClass.slow, CornerClass.medium]


@pytest.mark.parametrize(
    ("effect", "neutral_at_most", "neutral_at_least"),
    [(0.0, 1.0, 0.6), (0.0025, 1.0, 0.2), (0.005, 0.05, 0.0), (0.0075, 0.05, 0.0)],
)
def test_neutral_is_two_one_sided_tests_against_the_margin(
    effect: float, neutral_at_most: float, neutral_at_least: float
) -> None:
    """A quiet car (Ferrari-like: 3 ms passes, 1 ms laps) at 12 blocks, the last look. A change at
    the margin or beyond it is called neutral at most 5 % of the time; no change mostly is."""
    rng = np.random.default_rng(int(effect * 1e5))
    quiet = Design(effect=effect, blocks=12, lap_sd=0.001, noise_scale=0.1)
    runs = [
        judge(as_passes(simulate(rng, quiet)), look=3, extensions_left=0) for _ in range(TRIALS)
    ]
    neutral = np.mean([v.finding_class is FindingClass.neutral for v in runs])

    assert neutral_at_least <= neutral <= neutral_at_most


def test_too_many_passes_on_another_setup_make_the_experiment_invalid() -> None:
    passes = as_passes(simulate(np.random.default_rng(4), Design(effect=-0.02)))

    fine = judge(passes, excluded={"setup_mismatch": 10, "source_mismatch": 0})
    broken = judge(passes, excluded={"setup_mismatch": 30, "source_mismatch": 12})

    assert fine.finding_class is not FindingClass.invalid
    assert broken.finding_class is FindingClass.invalid
    assert broken.recommendation is not None and broken.recommendation.value == "stop_invalid"
    assert broken.passes_excluded == 42
    assert {c.kind for c in broken.confounders or []} >= {Kind.setup_mismatch, Kind.source_mismatch}


def offtrack_blocks(per_block: list[int]) -> list[dict[str, Any]]:
    return [
        {"block": b, "arm": "baseline" if b % 2 else "variant", "passes": 42, "offtrack": n}
        for b, n in enumerate(per_block, start=1)
    ]


@pytest.mark.parametrize(
    ("per_block", "status"),
    [
        ([0] * 8, Status.holds),
        ([0, 1, 0, 0, 1, 0, 0, 1], Status.holds),
        ([0, 6, 1, 7, 0, 8, 1, 6], Status.broke),  # the variant runs wide in every block
    ],
)
def test_off_track_passes_are_a_guardrail_not_dropped(per_block: list[int], status: Status) -> None:
    passes = as_passes(simulate(np.random.default_rng(6), Design(effect=-0.02)))

    verdict = judge(passes, offtrack=offtrack_blocks(per_block))

    (guardrail,) = [g for g in verdict.guardrails if g.channel == "offtrack_rate"]
    assert guardrail.status is status
    (result,) = [r for r in verdict.results if r.channel == "offtrack_rate"]
    assert result.role is Role.guardrail and result.n_passes == 8 * 42


def test_steady_driver_inputs_rarely_flag_adaptation_across_three_channels() -> None:
    """Three channels each tested at 10 % flagged about 27 % of identical experiments; Holm keeps
    the family at 10 %."""
    rng = np.random.default_rng(31)
    flagged = []
    for _ in range(TRIALS):
        passes = as_passes(simulate(rng, Design()))
        noise = simulate(rng, Design())
        for item, extra in zip(passes, noise, strict=True):
            for index, channel in enumerate(("brake_dist_m", "throttle_pickup_m", "steer_rate")):
                item["metrics"][channel] = extra.value * (index + 1) + rng.normal(0.0, 0.01)
        verdict = judge(passes, driver_channels=["brake_dist_m", "throttle_pickup_m", "steer_rate"])
        flagged.append(verdict.driver_adapted)

    assert np.mean(flagged) <= 0.12
    assert CLASSES.count("fast") >= 3  # the simulated track has classes worth reporting
