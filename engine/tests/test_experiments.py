"""Run plans and verdicts: POST /plan and POST /evaluate, on simulated A-B-A-B experiments."""

import math
from typing import Any

import numpy as np
import pytest
from fastapi.testclient import TestClient
from simulation import BASELINE, simulate_abab

from paddock_engine.app import create_app
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import (
    CornerClass,
    Decision,
    EvaluateResponse,
    Role,
    Status,
)
from paddock_engine.contracts.plan_request_schema import PlanRequest
from paddock_engine.contracts.plan_response_schema import Arm, SigmaSource
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.experiments.plan import ASSUMED_SIGMA_PER_ROPE, plan
from paddock_engine.stats.mde import minimum_detectable_effect

CORNERS = 10
TIME = "corner_time_s"
MIN_SPEED = "min_speed_kmh"
THROTTLE = "throttle_pickup_s"
SPEED_CLASSES = ("slow", "medium", "fast")
TEST = {"channel": TIME, "rope": 0.005, "level": 0.9, "better": "lower"}


def experiment_passes(
    effects: dict[str, tuple[float, float]], seed: int = 7
) -> list[dict[str, Any]]:
    """Passes of an A-B-A-B experiment; `effects` maps a channel to (variant effect, sigma)."""
    rng = np.random.default_rng(seed)
    passes: list[dict[str, Any]] = []
    for channel, (effect, sigma) in effects.items():
        rows = simulate_abab(rng, effect, CORNERS, sigma=sigma)
        if not passes:
            passes = [
                {
                    "corner": row.corner,
                    "arm": "baseline" if row.arm == BASELINE else "variant",
                    "block": row.block + 1,
                    "lap": row.lap,
                    "speed_class": SPEED_CLASSES[row.corner % 3],
                    "metrics": {},
                    "covariates": {"time_in_session_s": float(row.lap or 0) * 100.0},
                }
                for row in rows
            ]
        for item, row in zip(passes, rows, strict=True):
            item["metrics"][channel] = row.value
    return passes


def judge(passes: list[dict[str, Any]], **overrides: Any) -> EvaluateResponse:
    request = {
        "success_test": TEST,
        "guardrails": [],
        "predictions": [],
        "driver_channels": [],
        "passes": passes,
    } | overrides
    return evaluate(EvaluateRequest.model_validate(request))


SPEED_GUARDRAIL = {"channel": MIN_SPEED, "tolerance": 1.0, "worse": "lower"}


@pytest.mark.parametrize(
    ("time_effect", "time_sigma", "speed_effect", "rope", "decision", "guardrail"),
    [
        (-0.060, 0.028, 0.0, 0.005, Decision.accepted, Status.holds),
        (-0.060, 0.028, -4.0, 0.005, Decision.tradeoff, Status.broke),
        (+0.060, 0.028, 0.0, 0.005, Decision.rejected, Status.holds),
        (0.0, 0.028, 0.0, 0.100, Decision.rejected, Status.holds),  # negligible: baseline stays
        (0.0, 0.300, 0.0, 0.005, Decision.inconclusive, Status.holds),
        (0.0, 0.300, -4.0, 0.005, Decision.rejected, Status.broke),  # unclear and a guardrail broke
    ],
)
def test_decision_follows_the_primary_effect_and_the_guardrails(
    time_effect: float,
    time_sigma: float,
    speed_effect: float,
    rope: float,
    decision: Decision,
    guardrail: Status,
) -> None:
    passes = experiment_passes({TIME: (time_effect, time_sigma), MIN_SPEED: (speed_effect, 0.5)})

    verdict = judge(passes, success_test=TEST | {"rope": rope}, guardrails=[SPEED_GUARDRAIL])

    assert verdict.decision is decision, verdict.reason
    assert [g.status for g in verdict.guardrails] == [guardrail]
    assert verdict.passes_used == len(passes)
    assert verdict.passes_excluded == 0


def test_results_carry_the_primary_effect_overall_and_per_speed_class() -> None:
    verdict = judge(experiment_passes({TIME: (-0.060, 0.028)}))

    primary = [r for r in verdict.results if r.role is Role.primary]
    assert [r.corner_class for r in primary] == [
        CornerClass.all,
        CornerClass.slow,
        CornerClass.medium,
        CornerClass.fast,
    ]
    overall = primary[0]
    assert overall.ci_low < -0.060 < overall.ci_high
    assert overall.n_passes == CORNERS * 12
    assert sum(r.n_passes for r in primary[1:]) == overall.n_passes


def test_a_shift_in_driver_inputs_is_reported_with_the_verdict() -> None:
    passes = experiment_passes({TIME: (-0.060, 0.028), THROTTLE: (0.20, 0.05)})

    verdict = judge(passes, driver_channels=[THROTTLE])

    assert verdict.driver_adapted
    assert THROTTLE in verdict.reason
    assert verdict.decision is Decision.accepted  # reported, not overruled: the driver decides
    assert [r.channel for r in verdict.results if r.role is Role.driver_input] == [THROTTLE]


def test_steady_driver_inputs_are_not_called_adaptation() -> None:
    passes = experiment_passes({TIME: (-0.060, 0.028), THROTTLE: (0.0, 0.05)}, seed=11)

    assert not judge(passes, driver_channels=[THROTTLE]).driver_adapted


def test_predictions_are_scored_by_direction() -> None:
    passes = experiment_passes({TIME: (-0.060, 0.028), MIN_SPEED: (0.0, 0.5)})
    predictions = [
        {"channel": TIME, "direction": "decrease", "low": 0.02, "high": 0.08},
        {"channel": MIN_SPEED, "direction": "increase", "low": 1.0, "high": 3.0},
        {"channel": "never_measured", "direction": "increase", "low": 1.0, "high": 3.0},
    ]

    verdict = judge(passes, predictions=predictions)

    assert (verdict.prediction_hits, verdict.prediction_total) == (1, 2)
    assert [r.channel for r in verdict.results if r.role is Role.prediction] == [MIN_SPEED]


def test_passes_without_the_primary_channel_are_excluded_and_counted() -> None:
    passes = experiment_passes({TIME: (-0.060, 0.028)})
    for item in passes[:5]:
        item["metrics"][TIME] = None

    verdict = judge(passes)

    assert verdict.passes_excluded == 5
    assert verdict.passes_used == len(passes) - 5
    assert verdict.decision is Decision.accepted


def test_an_unchecked_guardrail_is_named_not_assumed_to_hold() -> None:
    passes = experiment_passes({TIME: (-0.060, 0.028)})

    verdict = judge(passes, guardrails=[SPEED_GUARDRAIL])

    assert [g.status for g in verdict.guardrails] == [Status.unknown]
    assert "Not checked: " + MIN_SPEED in verdict.reason


def test_one_setup_alone_cannot_be_judged() -> None:
    passes = [p for p in experiment_passes({TIME: (-0.060, 0.028)}) if p["arm"] == "baseline"]

    verdict = judge(passes)

    assert verdict.decision is Decision.inconclusive
    assert verdict.results == []


def plan_request(baseline_passes: list[dict[str, Any]], laps: int = 3) -> PlanRequest:
    return PlanRequest.model_validate(
        {
            "success_test": TEST,
            "corners": list(range(1, CORNERS + 1)),
            "laps_per_block": laps,
            "baseline_passes": baseline_passes,
        }
    )


def test_plan_is_four_blocks_and_says_what_it_can_detect() -> None:
    rng = np.random.default_rng(3)
    earlier = [
        {"corner": corner, "value": 5.0 + corner + float(rng.normal(0.0, 0.028))}
        for corner in range(1, CORNERS + 1)
        for _ in range(8)
    ]

    result = plan(plan_request(earlier))

    assert [(b.order, b.arm, b.laps_target) for b in result.blocks] == [
        (1, Arm.baseline, 3),
        (2, Arm.variant, 3),
        (3, Arm.baseline, 3),
        (4, Arm.variant, 3),
    ]
    assert result.sigma_source is SigmaSource.measured
    assert 0.015 < result.sigma < 0.045
    assert result.passes_per_arm == 6
    assert result.mde_detect == pytest.approx(minimum_detectable_effect(result.sigma, 6, CORNERS))
    assert result.mde_decide > result.mde_detect > 0


def test_plan_without_earlier_passes_says_its_scatter_is_assumed() -> None:
    result = plan(plan_request([]))

    assert result.sigma_source is SigmaSource.assumed
    assert math.isclose(result.sigma, ASSUMED_SIGMA_PER_ROPE * TEST["rope"])  # type: ignore[operator]


def test_more_laps_per_block_detect_smaller_effects() -> None:
    assert plan(plan_request([], laps=5)).mde_decide < plan(plan_request([], laps=2)).mde_decide


def test_plan_and_evaluate_are_served_over_http() -> None:
    client = TestClient(create_app())
    passes = experiment_passes({TIME: (-0.060, 0.028)})

    planned = client.post("/plan", json=plan_request([]).model_dump(mode="json"))
    judged = client.post(
        "/evaluate",
        json={
            "success_test": TEST,
            "guardrails": [],
            "predictions": [],
            "driver_channels": [],
            "passes": passes,
        },
    )

    assert planned.status_code == 200, planned.text
    assert len(planned.json()["blocks"]) == 4
    assert judged.status_code == 200, judged.text
    assert judged.json()["decision"] == "accepted"
    assert client.post("/evaluate", json={"passes": []}).status_code == 422
