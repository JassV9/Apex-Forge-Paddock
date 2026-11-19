"""Where things happen in one pass, in metres from the zone start (ADR 0031).

- B braking: the first metre with brake > 0.08 before the apex.
- TI turn-in: the start of the run of steering at or beyond 50 % of the pass's 95th-percentile
  steering, in the apex's direction, that leads to the apex.
- A apex: the slowest metre within 30 m of the corner; on a flat-out corner (no braking, under
  4 % speed loss) the metre of highest curvature, yaw rate / speed, smoothed over 5 m.
- P throttle pickup: the first metre after braking starts (or after the lift) with gas > 0.25
  and brake < 0.04, held 0.2 s.
- E corner end: the first metre after the apex with steering under 20 % of its 95th percentile
  and lateral g under 50 % of its peak (95th percentile), held 0.15 s. Both, not either: with
  "either", a downforce car's slow corner "ended" at the slowest point, where lateral g is
  lowest, and the exit was empty at 4 of the Hungaroring's 14 corners (ADR 0031).

The percentiles and peaks come from the pass's own zone, never from a joined next pass. An event
that does not happen is None. These are per pass; the phases of an experiment are cut at the
medians of the baseline's events (phases/cuts.py), never per pass.
"""

from dataclasses import dataclass

import numpy as np
import numpy.typing as npt

from paddock_engine.io.samples import FloatArray
from paddock_engine.phases.align import Aligned

BRAKE_ON = 0.08
BRAKE_OFF = 0.04
TURN_IN_SHARE = 0.5
GAS_PICKUP = 0.25
PICKUP_HOLD_S = 0.2
END_STEER_SHARE = 0.2
END_LAT_SHARE = 0.5
END_HOLD_S = 0.15
APEX_MARGIN_M = 30.0
FLAT_SPEED_LOSS = 0.04
CURVATURE_SMOOTH_M = 5
STEER_PERCENTILE = 95

Mask = npt.NDArray[np.bool_]


@dataclass(frozen=True)
class CornerWindow:
    """The corner itself, in metres from the zone start."""

    start_m: float
    end_m: float


@dataclass(frozen=True)
class PassEvents:
    braking_m: float | None
    turn_in_m: float
    apex_m: float
    pickup_m: float | None
    end_m: float | None
    by_curvature: bool  # flat-out corner: the apex is the curvature peak
    v_min_kmh: float  # slowest within 30 m of the corner, as features' v_min_kmh


def held_from(mask: Mask, t: FloatArray, start: int, hold_s: float) -> int | None:
    """First index >= start where mask stays True for at least `hold_s` seconds."""
    if start >= len(mask):
        return None
    edges = np.diff(np.concatenate([[False], mask[start:], [False]]).astype(np.int8))
    starts, stops = np.flatnonzero(edges == 1), np.flatnonzero(edges == -1) - 1
    for first, last in zip(starts + start, stops + start, strict=True):
        if t[last] - t[first] >= hold_s:
            return int(first)
    return None


def _own(p: Aligned) -> Mask:
    own: Mask = p.x <= p.zone_length_m
    return own


def _apex(p: Aligned, window: CornerWindow) -> tuple[int, bool, int]:
    """The apex index, whether it is the curvature peak, and the slowest index."""
    near = np.flatnonzero(
        _own(p) & (p.x >= window.start_m - APEX_MARGIN_M) & (p.x <= window.end_m + APEX_MARGIN_M)
    )
    if len(near) == 0:
        near = np.flatnonzero(_own(p))
    slowest = int(near[np.argmin(p.v[near])])
    entry = float(p.v[: slowest + 1].max())
    braked = bool((p.brake[: slowest + 1] > BRAKE_ON).any())
    if braked or (entry - p.v[slowest]) / max(entry, 1.0) >= FLAT_SPEED_LOSS:
        return slowest, False, slowest
    curvature = np.abs(p.yaw) / np.maximum(p.v, 5.0)
    kernel = np.ones(CURVATURE_SMOOTH_M) / CURVATURE_SMOOTH_M
    smooth = np.convolve(curvature, kernel, mode="same")
    inside = near[(p.x[near] >= window.start_m) & (p.x[near] <= window.end_m)]
    candidates = inside if len(inside) else near
    return int(candidates[np.argmax(smooth[candidates])]), True, slowest


def _turn_in(p: Aligned, apex: int, steer_p95: float) -> int:
    """Walk back from the apex through the run of steering beyond the share, same direction."""
    direction = np.sign(p.steer[apex]) or 1.0
    turned = (p.steer * direction) >= TURN_IN_SHARE * steer_p95
    before = np.flatnonzero(turned[: apex + 1])
    if len(before) == 0:
        return apex
    index = int(before[-1])
    while index > 0 and turned[index - 1]:
        index -= 1
    return index


def _pickup(p: Aligned, braking: int | None, apex: int) -> int | None:
    start = braking
    if start is None:
        lifted = np.flatnonzero(p.gas[: apex + 1] < GAS_PICKUP)
        if len(lifted) == 0:
            return None
        start = int(lifted[0])
    on = (p.gas > GAS_PICKUP) & (p.brake < BRAKE_OFF)
    return held_from(on, p.t, start, PICKUP_HOLD_S)


def _end(p: Aligned, apex: int, steer_p95: float, lat_peak: float) -> int | None:
    unwound = (np.abs(p.steer) < END_STEER_SHARE * steer_p95) & (
        np.abs(p.lat_g) < END_LAT_SHARE * lat_peak
    )
    return held_from(unwound, p.t, apex, END_HOLD_S)


def pass_events(p: Aligned, window: CornerWindow) -> PassEvents:
    own = _own(p)
    steer_p95 = float(np.percentile(np.abs(p.steer[own]), STEER_PERCENTILE))
    lat_peak = float(np.percentile(np.abs(p.lat_g[own]), STEER_PERCENTILE))
    apex, by_curvature, slowest = _apex(p, window)
    braking = np.flatnonzero(p.brake[: apex + 1] > BRAKE_ON)
    brake_index = int(braking[0]) if len(braking) else None
    pickup = _pickup(p, brake_index, apex)
    end = _end(p, apex, steer_p95, lat_peak)

    def at(index: int | None) -> float | None:
        return None if index is None else float(p.x[index])

    return PassEvents(
        braking_m=at(brake_index),
        turn_in_m=float(p.x[_turn_in(p, apex, steer_p95)]),
        apex_m=float(p.x[apex]),
        pickup_m=at(pickup),
        end_m=at(end),
        by_curvature=by_curvature,
        v_min_kmh=float(p.v[slowest]) * 3.6,
    )
