"""The car's response inside the verdict (ADR 0034): a known effect on the car with none on the
clock, Holm across the expected cells, phase and regime interactions, and verdicts without it."""

from functools import cache

import numpy as np
import pytest
from block_simulation import CLASSES, Design, as_passes, simulate
from fastapi.testclient import TestClient
from verdict_data import Truth, measured_experiment, regimes, sampled_experiment

from paddock_engine import app as app_module
from paddock_engine.app import create_app
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import (
    EvaluateResponse,
    FindingClass,
    Kind,
    Recommendation,
)
from paddock_engine.contracts.evaluate_response_schema import ResponseFindingClass as Found
from paddock_engine.experiments.context import context
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.experiments.response_findings import MOVED, response_findings
from paddock_engine.experiments.vehicle import PHASES, Vehicle, expected_cells
from paddock_engine.response.config import observables

TRIALS = 300
SLIP = 0.3  # deg added to the rear slip angle from turn-in to the corner end


@cache
def rear_slip_verdict() -> EvaluateResponse:
    request, samples = sampled_experiment({"rear_slip_extra_deg": SLIP}, seed=7)
    request["extensions_left"] = 0
    return evaluate(EvaluateRequest.model_validate(request), samples)


def test_a_rear_slip_effect_with_no_time_effect_is_found_on_the_car_not_the_clock() -> None:
    verdict = rear_slip_verdict()
    cells = {(f.metric, f.phase.value, f.regime.value): f for f in verdict.responses or []}

    assert verdict.finding_class in (FindingClass.neutral, FindingClass.insufficient_evidence)
    slip = cells[("rear_slip_deg", "mid", "all")]
    assert slip.finding in (Found.increase, Found.likely_increase)
    assert slip.ci_low is not None and slip.ci_high is not None
    assert slip.ci_low < SLIP < slip.ci_high and slip.expected.value == "direct"
    assert cells[("yaw_gain", "mid", "all")].finding not in MOVED  # nothing else was changed
    assert verdict.recommendation is Recommendation.switch_metric
    assert verdict.response_parameter == "rear_arb"


def _aa_vehicle(rng: np.random.Generator, truth: Truth) -> Vehicle:
    config = observables()
    row = config.row_for("arb", "R")
    expected = expected_cells(row, config)
    cells = sorted({(c.metric, c.phase) for c in expected})
    scale = {metric: config.metrics[metric].margin for metric, _ in cells}
    measured = measured_experiment(rng, cells, truth, scale)
    return Vehicle(measured=measured, regimes=regimes(truth.corners), row=row, expected=expected)


def test_holm_keeps_false_response_findings_on_identical_setups_at_five_percent() -> None:
    """27 expected cells of the rear ARB row on identical setups, block shocks included. Any
    directional claim is false; each cell at 10 % alone would claim one most of the time."""
    rng = np.random.default_rng(34)
    false_any, unadjusted = [], []
    for _ in range(TRIALS):
        found = response_findings(_aa_vehicle(rng, Truth()), "ARB_R", 0.9)
        false_any.append(any(f.finding in MOVED for f in found))
        unadjusted.append(any(f.ci_low is not None and (f.ci_low > 0 or (f.ci_high or 0) < 0)
                              for f in found))  # fmt: skip

    assert len(found) == 27
    assert np.mean(false_any) <= 0.05
    assert np.mean(unadjusted) >= 0.5


def test_a_real_response_effect_survives_holm() -> None:
    rng = np.random.default_rng(35)
    truth = Truth(effects={("rear_slip_deg", "exit"): 0.15})  # three margins
    hits = []
    for _ in range(100):
        found = response_findings(_aa_vehicle(rng, truth), "ARB_R", 0.9)
        (cell,) = [f for f in found if (f.metric, f.phase.value, f.regime.value) ==
                   ("rear_slip_deg", "exit", "all")]  # fmt: skip
        hits.append(cell.finding in (Found.increase, Found.likely_increase))

    assert np.mean(hits) >= 0.8


def _phase_vehicle(rng: np.random.Generator, braking: float, exit_: float) -> Vehicle:
    cells = [("phase_time_s", phase) for phase in PHASES]
    truth = Truth(effects={("phase_time_s", "braking"): braking, ("phase_time_s", "exit"): exit_})
    measured = measured_experiment(rng, cells, truth, {"phase_time_s": 0.002})
    return Vehicle(measured=measured, regimes=regimes(9), row=None, expected=[])


def test_losing_in_braking_what_is_gained_on_exit_is_context_dependent_by_phase() -> None:
    rng = np.random.default_rng(36)
    runs = [context([], "corner_time_s", {}, _phase_vehicle(rng, 0.004, -0.004)) for _ in range(50)]

    assert np.mean([any(p.first == "braking" and p.second == "exit" for p in r.phases)
                    for r in runs]) >= 0.8  # fmt: skip


def test_phases_without_an_interaction_are_rarely_called_context_dependent() -> None:
    rng = np.random.default_rng(37)
    runs = [context([], "corner_time_s", {}, _phase_vehicle(rng, 0.0, 0.0)) for _ in range(200)]

    assert np.mean([r.found for r in runs]) <= 0.06


def _by_minimum_speed(passes: list[dict[str, object]]) -> list[dict[str, object]]:
    """No stored class at all: the regime comes from the baseline's minimum speeds."""
    speed = {"slow": 80.0, "medium": 140.0, "fast": 220.0}
    out = []
    for item in passes:
        corner = int(item["corner"])  # type: ignore[call-overload]
        v_min = speed[CLASSES[corner - 1]] + corner
        metrics = dict(item["metrics"]) | {"v_min_kmh": v_min}  # type: ignore[call-overload]
        out.append({k: v for k, v in item.items() if k != "speed_class"} | {"metrics": metrics})
    return out


def _judge(passes: list[dict[str, object]]) -> EvaluateResponse:
    request = {"success_test": {"channel": "corner_time_s", "rope": 0.005, "level": 0.9,
               "better": "lower"}, "guardrails": [], "predictions": [], "driver_channels": [],
               "passes": passes, "look": 1, "extensions_left": 2}  # fmt: skip
    return evaluate(EvaluateRequest.model_validate(request))


def test_regimes_from_minimum_speeds_find_a_speed_interaction() -> None:
    rng = np.random.default_rng(38)
    design = Design(class_effects={"slow": 0.010, "fast": -0.010}, noise_scale=0.5)
    runs = [_judge(_by_minimum_speed(as_passes(simulate(rng, design)))) for _ in range(60)]

    found = [any(c.kind is Kind.speed_class_interaction for c in v.confounders or []) for v in runs]
    assert np.mean(found) >= 0.8


def test_a_pass_s_own_class_never_moves_its_corner() -> None:
    passes = as_passes(simulate(np.random.default_rng(39), Design(effect=-0.01)))
    flipped = [p | {"speed_class": "fast"} if p["arm"] == "variant" else p for p in passes]

    assert _judge(passes).results == _judge(flipped).results


def test_samples_that_cannot_be_read_leave_the_time_verdict_and_say_so() -> None:
    request, _ = sampled_experiment({"rear_slip_extra_deg": SLIP}, blocks=4, laps=2)

    verdict = evaluate(EvaluateRequest.model_validate(request), None)

    assert verdict.finding_class is not None and verdict.responses == []
    notes = verdict.response_notes or []
    assert notes and "could not be read" in notes[0].root


def test_a_request_without_a_response_context_answers_as_before() -> None:
    request, samples = sampled_experiment({}, blocks=4, laps=2)
    del request["response"]

    verdict = evaluate(EvaluateRequest.model_validate(request), samples)

    assert verdict.responses is None and verdict.response_notes is None
    assert EvaluateResponse.model_validate(
        verdict.model_dump(exclude={"responses", "response_parameter", "response_notes"})
    )


def test_the_endpoint_reads_the_samples_the_request_points_at(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    request, samples = sampled_experiment({"rear_slip_extra_deg": SLIP}, blocks=4, laps=2)
    monkeypatch.setattr(app_module, "verdict_samples", lambda _request: samples)

    reply = TestClient(create_app()).post("/evaluate", json=request)

    assert reply.status_code == 200, reply.text
    body = reply.json()
    assert body["response_parameter"] == "rear_arb" and len(body["responses"]) == 27
