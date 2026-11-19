"""A simulated car with known effects, for the sensitivity and ranking tests (ADR 0011, 0033)."""

from functools import cache
from typing import Any

from test_experiments import TEST, TIME, experiment_passes

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.sensitivity_request_schema import SensitivityRequest
from paddock_engine.contracts.sensitivity_response_schema import (
    SensitivityEffect,
    SensitivityResponse,
)
from paddock_engine.sensitivity.measure import measure
from paddock_engine.sensitivity.pooling import Estimate
from paddock_engine.sensitivity.profile import update

ROPE = 0.005
LEVEL = 0.90
# A repeatable driver (the Ghost Driver's target): 8 ms of scatter per corner pass. With the
# 28 ms of a human, calling an effect neutral against a 5 ms band takes about 90 laps per
# setup; the profile then says "insufficient", which is the honest answer.
REPEATABLE_SIGMA = 0.008

# The simulated car: seconds per corner pass against the baseline, by item and clicks.
TRUE_EFFECTS = {
    ("ARB_FRONT", 1): -0.020,  # linear: 20 ms per click
    ("ARB_FRONT", -1): 0.020,
    ("ARB_FRONT", 2): -0.040,
    ("ARB_REAR", 1): 0.0,  # negligible: the car ignores it
    ("ARB_REAR", -1): 0.0,
    ("WING_REAR", 1): 0.015,  # non-linear: the baseline is the optimum, both ways are slower
    ("WING_REAR", -1): 0.015,
}

STRATUM: dict[str, Any] = {
    "phase": "corner",
    "track": "fn_hungaroring/layout_f1_2026",
    "track_name": "Hungaroring F1 2026",
    "driver": "ai",
    "hint_bucket": "0.94",
    "baseline_value": 3.0,
    "click_step": 1.0,
}


def synthetic(
    experiment_id: int, parameter: str, clicks: int, estimate: float, se: float, **changes: Any
) -> dict[str, Any]:
    """A re-derived measurement with a given estimate and error (6 block degrees of freedom)."""
    df = changes.pop("df", 6)
    low, high = Estimate(estimate, se, df).interval(LEVEL)
    return (
        {
            "experiment_id": experiment_id,
            "parameter": parameter,
            "clicks": clicks,
            "channel": TIME,
            "corner_class": "all",
            "estimate": estimate,
            "ci_low": low,
            "ci_high": high,
            "standard_error": se,
            "df": df,
            "n_passes": 240,
            "stale": False,
            "note": "",
            "finding_class": "insufficient_evidence",
            "engine_version": "0.2.0",
        }
        | STRATUM
        | changes
    )


@cache
def _measured(experiment_id: int, effect: float) -> tuple[float, float, int, int]:
    passes = experiment_passes({TIME: (effect, REPEATABLE_SIGMA)}, seed=100 + experiment_id)
    request = {"success_test": TEST, "guardrails": [], "predictions": [], "driver_channels": []}
    got = measure(EvaluateRequest.model_validate(request | {"passes": passes}))
    row = next(e for e in got.effects if e.channel == TIME and e.corner_class.value == "all")
    return row.estimate, row.standard_error, row.df, row.n_passes


def measurement(experiment_id: int, parameter: str, clicks: int, effect: float) -> dict[str, Any]:
    """One planned 8-block experiment on the simulated car, re-derived by the real engine."""
    estimate, se, df, passes = _measured(experiment_id, effect)
    return synthetic(experiment_id, parameter, clicks, estimate, se, df=df, n_passes=passes)


def request_of(
    measurements: list[dict[str, Any]], transfers: list[dict[str, Any]] | None = None
) -> SensitivityRequest:
    return SensitivityRequest.model_validate(
        {
            "measurements": measurements,
            "transfers": transfers or [],
            "ropes": {TIME: ROPE},
            "level": LEVEL,
        }
    )


@cache
def sweep() -> SensitivityResponse:
    """A planned one-factor-at-a-time sweep: every item one click each way, the first also two."""
    measurements = [
        measurement(index + 1, parameter, clicks, effect)
        for index, ((parameter, clicks), effect) in enumerate(TRUE_EFFECTS.items())
    ]
    return update(request_of(measurements))


def row(profile: SensitivityResponse, parameter: str, clicks: int) -> SensitivityEffect:
    return next(e for e in profile.effects if e.parameter == parameter and e.clicks == clicks)


def transfer(parameter: str, clicks: int, estimate: float, **changes: Any) -> dict[str, Any]:
    fields = {
        "parameter": parameter,
        "source_car": "Formula Alpha 2025",
        "same_drivetrain": True,
        "clicks": clicks,
        "channel": TIME,
        "corner_class": "all",
        "phase": "corner",
        "estimate": estimate,
        "standard_error": 0.0024,
        "n_passes": 200,
        "source_track": "fn_cota/layout_f1_2026",
        "source_experiments": 1,
    }
    return fields | changes


def candidate(parameter: str, clicks: int) -> dict[str, Any]:
    return {
        "parameter": parameter,
        "label": parameter.replace("_", " ").title(),
        "clicks": clicks,
        "why": "a rule of thumb",
        "hypotheses": ["mid_understeer.slow"],
    }
