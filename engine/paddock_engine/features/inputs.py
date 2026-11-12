"""What the driver did in one corner pass: braking, throttle, steering corrections, slides.

Ported from Apex Forge v0 `core/analyze.lua` (the "input metrics" the audit keeps) and tested
against its outputs. These channels describe the driver, not the car: an experiment only
counts as a setup effect when they did not shift between the two setups.
"""

from dataclasses import dataclass

import numpy as np
import numpy.typing as npt

from paddock_engine.features.axles import AxleChannels
from paddock_engine.io.samples import FloatArray, PassSamples
from paddock_engine.phases.segment import Segmentation

BRAKE_OFF = 0.04
GAS_PICKUP = 0.25
GAS_FULL = 0.95
LAT_MIN_G = 0.3
LOCK_SLIP_RATIO = 0.18  # slip ratio counted as a lockup or wheelspin
LOCK_RUN_SAMPLES = 3  # in a row, about 50 ms


@dataclass(frozen=True)
class DriverInputs:
    brake_dist_m: float | None  # where braking starts, metres before the apex
    brake_peak: float | None
    brake_attack_s: float | None  # time to 80 % of peak pressure
    brake_release_m: float | None  # where braking ends relative to the apex (negative: before)
    trail_share: float | None  # share of braking (pressure x time) done with the wheel turned
    throttle_pickup_m: float | None  # relative to the apex (negative: before)
    throttle_to_full_s: float | None
    lifts: int
    coast_s: float
    steer_corrections_per_s: float
    countersteer_s: float
    countersteer_exit_s: float
    front_lockups: int
    rear_lockups: int
    wheelspin: int


def _dt(t: FloatArray) -> FloatArray:
    """Time since the previous sample; 0 for the first."""
    return np.diff(t, prepend=t[0])


def _attack(s: PassSamples, seg: Segmentation) -> float | None:
    if seg.brake_start is None or seg.brake_end is None:
        return None
    window = s.brake[seg.brake_start : seg.brake_end + 1]
    hits = np.flatnonzero(window >= 0.8 * seg.brake_peak)
    if len(hits) == 0:
        return None
    return float(s.t[seg.brake_start + hits[0]] - s.t[seg.brake_start])


def _trail_share(s: PassSamples, seg: Segmentation) -> float | None:
    if seg.brake_start is None or seg.brake_end is None:
        return None
    span = slice(max(1, seg.brake_start), seg.brake_end + 1)
    weight = _dt(s.t)[span] * s.brake[span]
    total = float(weight.sum())
    if total <= 0.05:
        return None
    turned = np.abs(s.steer[span]) >= 0.25 * seg.steer_peak
    return float(weight[turned].sum()) / total


def _throttle(s: PassSamples, seg: Segmentation) -> tuple[int | None, int | None, int]:
    """Pickup index, full-throttle index and the number of lifts before the corner unwinds."""
    start = seg.brake_end if seg.brake_end is not None else seg.turn_in
    picked = np.flatnonzero((s.gas[start:] > GAS_PICKUP) & (s.brake[start:] < BRAKE_OFF))
    if len(picked) == 0:
        return None, None, 0
    pick = int(start + picked[0])
    full, lifts, lifted, peak = None, 0, False, float(s.gas[pick])
    for i in range(pick, s.n):
        gas = float(s.gas[i])
        if full is None and gas >= GAS_FULL:
            full = i
        if i > seg.unwind:
            continue
        peak = max(peak, gas)
        if not lifted and gas < peak - 0.25 and s.brake[i] < BRAKE_OFF:
            lifted = True
        elif lifted and gas > peak - 0.1:
            lifts, lifted = lifts + 1, False
    return pick, full, lifts


def _coast(s: PassSamples, seg: Segmentation, pick: int | None) -> float:
    first = seg.brake_start if seg.brake_start is not None else seg.turn_in
    last = pick if pick is not None else seg.apex
    span = slice(max(1, first), last + 1)
    coasting = (s.gas[span] < 0.1) & (s.brake[span] < BRAKE_OFF)
    return float(_dt(s.t)[span][coasting].sum())


def _countersteer(s: PassSamples, seg: Segmentation) -> tuple[float, float]:
    """Seconds with the wheel pointing against the car's rotation: whole pass, and after mid."""
    turning = s.steer * s.yaw
    reference = (np.abs(s.lat_g) > 0.5) & (np.abs(s.steer) > 3) & (np.abs(s.yaw) > 0.05)
    sign = 1.0 if (turning[reference] > 0).sum() >= (turning[reference] <= 0).sum() else -1.0
    against = (
        (np.abs(s.steer) > 2) & (np.abs(s.yaw) > 0.1) & (np.abs(s.lat_g) > LAT_MIN_G)
        & (turning * sign < 0)
    )  # fmt: skip
    against[0] = False
    dt = _dt(s.t)
    after_mid = np.arange(s.n) > seg.mid_end
    return float(dt[against].sum()), float(dt[against & after_mid].sum())


def _corrections(s: PassSamples, seg: Segmentation) -> float:
    """Steering reversals per second beyond the one a clean corner needs."""
    hysteresis = max(1.5, 0.04 * seg.steer_peak)
    direction, reference, reversals = 0, float(s.steer[seg.turn_in]), 0
    for i in range(seg.turn_in, seg.unwind + 1):
        angle = float(s.steer[i])
        if direction >= 0 and angle < reference - hysteresis:
            reversals += direction > 0
            direction, reference = -1, angle
        elif direction <= 0 and angle > reference + hysteresis:
            reversals += direction < 0
            direction, reference = 1, angle
        elif (direction > 0 and angle > reference) or (direction < 0 and angle < reference):
            reference = angle
    steer_time = max(0.2, float(s.t[seg.unwind] - s.t[seg.turn_in]))
    return max(0, reversals - 1) / steer_time


def _runs(flags: npt.NDArray[np.bool_], need: int = LOCK_RUN_SAMPLES) -> int:
    """How many runs of at least `need` consecutive True values."""
    events, run = 0, 0
    for flag in flags:
        run = run + 1 if flag else 0
        events += run == need
    return events


def _slip_events(s: PassSamples, axles: AxleChannels, driven: str) -> tuple[int, int, int]:
    braking = (s.brake > 0.2) & (s.gas < 0.1)
    powered = (s.gas > 0.3) & (s.brake < BRAKE_OFF)
    driven_ratio = {
        "front": axles.front_ratio,
        "all": np.maximum(axles.front_ratio, axles.rear_ratio),
    }.get(driven, axles.rear_ratio)
    return (
        _runs(braking & (axles.front_ratio > LOCK_SLIP_RATIO)),
        _runs(braking & (axles.rear_ratio > LOCK_SLIP_RATIO)),
        _runs(powered & (driven_ratio > LOCK_SLIP_RATIO)),
    )


def driver_inputs(
    s: PassSamples, seg: Segmentation, axles: AxleChannels, driven: str = "rear"
) -> DriverInputs:
    """`driven` is the driven axle: "rear", "front" or "all"."""
    x = seg.x
    apex_x = float(x[seg.apex])
    pick, full, lifts = _throttle(s, seg)
    countersteer, countersteer_exit = _countersteer(s, seg)
    front_locks, rear_locks, spin = _slip_events(s, axles, driven)
    braked = seg.brake_start is not None
    return DriverInputs(
        brake_dist_m=apex_x - float(x[seg.brake_start]) if seg.brake_start is not None else None,
        brake_peak=seg.brake_peak if braked else None,
        brake_attack_s=_attack(s, seg),
        brake_release_m=float(x[seg.brake_end]) - apex_x if seg.brake_end is not None else None,
        trail_share=_trail_share(s, seg),
        throttle_pickup_m=float(x[pick]) - apex_x if pick is not None else None,
        throttle_to_full_s=(
            float(s.t[full] - s.t[pick]) if pick is not None and full is not None else None
        ),
        lifts=lifts,
        coast_s=_coast(s, seg, pick),
        steer_corrections_per_s=_corrections(s, seg),
        countersteer_s=countersteer,
        countersteer_exit_s=countersteer_exit,
        front_lockups=front_locks,
        rear_lockups=rear_locks,
        wheelspin=spin,
    )
