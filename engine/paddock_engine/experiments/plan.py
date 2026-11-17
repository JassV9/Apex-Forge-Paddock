"""Run plans: A-B-A-B blocks and what they can answer, known before a lap is driven.

The plan is sized for the noise it will meet (ADR 0025): the scatter of single passes and the
error a whole lap shares across its corners. A lap's error averages out over laps only, so a
car whose laps scatter as a whole gets more blocks, up to MAX_BLOCKS. A plan that cannot reach
its target within that says so instead of promising what it cannot decide.
"""

import math
from dataclasses import dataclass

from scipy import stats

from paddock_engine.contracts.plan_request_schema import PlanRequest
from paddock_engine.contracts.plan_response_schema import (
    Arm,
    Corner,
    PlanBlock,
    PlanResponse,
    SigmaLapSource,
    SigmaSource,
)
from paddock_engine.stats.lap_noise import LapPass, lap_scatter
from paddock_engine.stats.mde import lap_inclusive_sigma, minimum_detectable_effect
from paddock_engine.stats.noise import NoiseModel, noise_model
from paddock_engine.stats.observations import Observation

# Without earlier passes there is no measured scatter. 28 ms of corner-time scatter against a
# 5 ms equivalence band is what the real FA26 COTA session showed; the same ratio is assumed
# for any channel, and the plan says "assumed".
ASSUMED_SIGMA_PER_ROPE = 5.6
POWER_Z = 0.84  # z for 80 % power
STANDARD_BLOCKS = 4  # A-B-A-B, the plan the statistics were validated on (ADR 0008)
# The longest run the ledger already lets an experiment drive: four blocks and two extensions
# of two blocks each (core's MaxExtensions).
MAX_BLOCKS = 8
# A plan should decide an effect of four equivalence bands: the brief's 20 ms of corner time
# against its 5 ms band (ADR 0008). See `target`.
TARGET_IN_ROPES = 4


@dataclass(frozen=True)
class Sizing:
    blocks: int
    mde_detect: float
    mde_decide: float
    target: float
    met: bool


def time_inflation(blocks: int) -> float:
    """Standard-error inflation of the arm from adjusting for a steady drift in time.

    With arms alternating over `blocks` equal blocks the arm correlates with the block's position
    by r^2 = 3 / (blocks^2 - 1), so the inflation is sqrt(1 / (1 - r^2)): 1.12 for A-B-A-B,
    1.05 for six blocks, 1.02 for eight.
    """
    return math.sqrt((blocks**2 - 1) / (blocks**2 - 4))


def decision_effect(
    sigma: float,
    sigma_lap: float,
    corners: int,
    laps_per_block: int,
    blocks: int,
    rope: float,
    level: float,
) -> float:
    """Smallest effect that clears the equivalence band with 80 % power.

    A decision needs the whole interval beyond the band, so the effect must exceed the band by
    the interval's half-width plus the power margin. With few laps the interval uses a wide
    Student's t (laps - blocks - 1 degrees of freedom, as the verdict does).
    """
    passes_per_arm = laps_per_block * blocks // 2
    effective = lap_inclusive_sigma(sigma, sigma_lap, corners)
    standard_error = (
        time_inflation(blocks) * effective * math.sqrt(2.0 / (passes_per_arm * corners))
    )
    degrees = max(1, laps_per_block * blocks - blocks - 1)
    quantile = float(stats.t.ppf(1.0 - (1.0 - level) / 2.0, degrees))
    return rope + (quantile + POWER_Z) * standard_error


def target(standard: float, rope: float) -> float:
    """The decision effect a plan is sized for, a whole number of equivalence bands.

    Four bands, or, for a car whose passes alone scatter more, the fewest whole bands that cover
    what the standard plan decides on pass scatter alone (`standard`). Without lap noise every
    plan is then the standard one, and lap noise worth less than the rest of a band adds no
    blocks. Without a band the target is `standard` itself.
    """
    if rope <= 0:
        return standard
    bands = max(TARGET_IN_ROPES, math.ceil(standard / rope - 1e-9))
    return bands * rope


def size(
    sigma: float, sigma_lap: float, corners: int, laps_per_block: int, rope: float, level: float
) -> Sizing:
    """The fewest blocks (in pairs, from four up to MAX_BLOCKS) that decide the target."""
    standard = decision_effect(sigma, 0.0, corners, laps_per_block, STANDARD_BLOCKS, rope, level)
    goal = target(standard, rope)
    blocks = STANDARD_BLOCKS
    decide = decision_effect(sigma, sigma_lap, corners, laps_per_block, blocks, rope, level)
    while decide > goal and blocks < MAX_BLOCKS:
        blocks += 2
        decide = decision_effect(sigma, sigma_lap, corners, laps_per_block, blocks, rope, level)
    effective = lap_inclusive_sigma(sigma, sigma_lap, corners)
    detect = minimum_detectable_effect(effective, laps_per_block * blocks // 2, corners)
    return Sizing(blocks, detect, decide, goal, decide <= goal)


def _noise(request: PlanRequest) -> NoiseModel | None:
    observations = [Observation(p.corner, "baseline", p.value) for p in request.baseline_passes]
    try:
        return noise_model(observations)
    except ValueError:
        return None


def _lap_noise(passes: list[LapPass]) -> float:
    """Raises ValueError when the laps cannot tell (too few, or too few corners)."""
    model = noise_model([Observation(p.corner, "baseline", p.value) for p in passes])
    return lap_scatter(passes, model.per_corner)[0]


def sigma_lap(request: PlanRequest) -> tuple[float, SigmaLapSource]:
    """Lap noise from laps driven to a run plan; with fewer than six of them, from every lap.

    Free and calibration laps are a fallback: during calibration the agent changes the AI's grip
    lap by lap, which is not the noise a frozen run plan meets.
    """
    planned: list[LapPass] = []
    every: list[LapPass] = []
    for p in request.baseline_passes:
        if p.lap is None:
            continue  # an out-lap, or a pass without a game lap
        every.append(LapPass(p.corner, p.lap, p.value))
        if p.planned:
            planned.append(every[-1])
    for passes, source in ((planned, SigmaLapSource.planned), (every, SigmaLapSource.free)):
        try:
            return _lap_noise(passes), source
        except ValueError:
            continue
    return 0.0, SigmaLapSource.none


def _arms(blocks: int) -> list[Arm]:
    return [Arm.baseline if order % 2 == 0 else Arm.variant for order in range(blocks)]


def plan(request: PlanRequest) -> PlanResponse:
    """A-B-A-B... over every corner of the track, as long as the noise needs."""
    model = _noise(request)
    rope, level = request.success_test.rope, request.success_test.level
    sigma = model.typical if model else ASSUMED_SIGMA_PER_ROPE * rope
    lap, lap_source = sigma_lap(request) if model else (0.0, SigmaLapSource.none)
    corners = sorted({corner.root for corner in request.corners})
    laps = request.laps_per_block
    sizing = size(sigma, lap, len(corners), laps, rope, level)
    return PlanResponse(
        sigma=sigma,
        sigma_source=SigmaSource.measured if model else SigmaSource.assumed,
        corners=[Corner(number) for number in corners],
        blocks=[
            PlanBlock(order=order, arm=arm, laps_target=laps)
            for order, arm in enumerate(_arms(sizing.blocks), start=1)
        ],
        passes_per_arm=laps * sizing.blocks // 2,
        mde_detect=sizing.mde_detect,
        mde_decide=sizing.mde_decide,
        sigma_lap=lap,
        sigma_lap_source=lap_source,
        mde_target=sizing.target,
        target_met=sizing.met,
    )
