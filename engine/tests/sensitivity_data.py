"""A simulated car with known effects, for the sensitivity and ranking tests."""

from functools import cache
from typing import Any

from test_experiments import TIME, experiment_passes, judge

from paddock_engine.contracts.sensitivity_request_schema import SensitivityRequest
from paddock_engine.contracts.sensitivity_response_schema import (
    SensitivityEffect,
    SensitivityResponse,
)
from paddock_engine.sensitivity.profile import update

ROPE = 0.005
LEVEL = 0.90
# A repeatable driver (the Ghost Driver's target): 8 ms of scatter per corner pass. With the
# 28 ms of a human, calling an effect negligible against a 5 ms band takes about 90 laps per
# setup; the profile then says "inconclusive", which is the honest answer.
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


def measurement(experiment_id: int, parameter: str, clicks: int, effect: float) -> dict[str, Any]:
    """One planned experiment on the simulated car, judged by the real evaluation."""
    passes = experiment_passes({TIME: (effect, REPEATABLE_SIGMA)}, seed=100 + experiment_id)
    result = next(r for r in judge(passes).results if r.channel == TIME)
    return {
        "experiment_id": experiment_id,
        "parameter": parameter,
        "clicks": clicks,
        "channel": TIME,
        "corner_class": "all",
        "estimate": result.estimate,
        "ci_low": result.ci_low,
        "ci_high": result.ci_high,
        "n_passes": result.n_passes,
    }


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
        "estimate": estimate,
        "ci_low": estimate - 0.004,
        "ci_high": estimate + 0.004,
        "n_passes": 200,
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
