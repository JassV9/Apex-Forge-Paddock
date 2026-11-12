"""Run plans: A-B-A-B blocks and what they can answer, known before a lap is driven."""

import math

from scipy import stats

from paddock_engine.contracts.plan_request_schema import PlanRequest
from paddock_engine.contracts.plan_response_schema import (
    Arm,
    Corner,
    PlanBlock,
    PlanResponse,
    SigmaSource,
)
from paddock_engine.stats.mde import minimum_detectable_effect
from paddock_engine.stats.noise import noise_model
from paddock_engine.stats.observations import Observation

BLOCK_ORDER = (Arm.baseline, Arm.variant, Arm.baseline, Arm.variant)
# Without earlier passes there is no measured scatter. 28 ms of corner-time scatter against a
# 5 ms equivalence band is what the real FA26 COTA session showed; the same ratio is assumed
# for any channel, and the plan says "assumed".
ASSUMED_SIGMA_PER_ROPE = 5.6
POWER_Z = 0.84  # z for 80 % power
# In A-B-A-B the variant is driven later on average, so adjusting for time in session costs
# precision: the variance inflation of the arm coefficient is about 1.25, 1.12 in standard error.
ABAB_TIME_INFLATION = 1.12


def _sigma(request: PlanRequest) -> tuple[float, SigmaSource]:
    observations = [Observation(p.corner, "baseline", p.value) for p in request.baseline_passes]
    try:
        return noise_model(observations).typical, SigmaSource.measured
    except ValueError:
        return ASSUMED_SIGMA_PER_ROPE * request.success_test.rope, SigmaSource.assumed


def decision_effect(
    sigma: float, passes_per_arm: int, corners: int, rope: float, level: float, laps: int
) -> float:
    """Smallest effect that clears the equivalence band with 80 % power.

    A decision needs the whole interval beyond the band, so the effect must exceed the band by
    the interval's half-width plus the power margin. `laps` is the number of laps in the plan:
    with few laps the interval uses a wide Student's t.
    """
    standard_error = ABAB_TIME_INFLATION * sigma * math.sqrt(2.0 / (passes_per_arm * corners))
    degrees = max(1, laps - len(BLOCK_ORDER) - 1)
    quantile = float(stats.t.ppf(1.0 - (1.0 - level) / 2.0, degrees))
    return rope + (quantile + POWER_Z) * standard_error


def plan(request: PlanRequest) -> PlanResponse:
    """An A-B-A-B plan over every corner of the track."""
    sigma, source = _sigma(request)
    corners = sorted({corner.root for corner in request.corners})
    laps = request.laps_per_block
    passes_per_arm = laps * BLOCK_ORDER.count(Arm.variant)
    return PlanResponse(
        sigma=sigma,
        sigma_source=source,
        corners=[Corner(number) for number in corners],
        blocks=[
            PlanBlock(order=order, arm=arm, laps_target=laps)
            for order, arm in enumerate(BLOCK_ORDER, start=1)
        ],
        passes_per_arm=passes_per_arm,
        mde_detect=minimum_detectable_effect(sigma, passes_per_arm, len(corners)),
        mde_decide=decision_effect(
            sigma,
            passes_per_arm,
            len(corners),
            request.success_test.rope,
            request.success_test.level,
            laps * len(BLOCK_ORDER),
        ),
    )
