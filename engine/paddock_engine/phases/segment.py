"""Cuts one corner pass into phases: brake, entry, mid, exit.

Ported from Apex Forge v0 `core/analyze.lua` and tested against its outputs
(tests/test_phases.py). The cuts and thresholds are unchanged; v0's balance number, which was
computed in the same function, is not carried over (docs/audit-v0.md).

| Phase | From | To |
| --- | --- | --- |
| brake | first braking | turn-in |
| entry | turn-in (steering past 30 % of its peak) | the mid window |
| mid   | around the slowest point while speed is within 3 % of the minimum, at least +-0.15 s | |
| exit  | end of the mid window | steering back under 30 % of its peak |

Braking inside the mid window counts as entry.
"""

from dataclasses import dataclass
from enum import IntEnum

import numpy as np
import numpy.typing as npt

from paddock_engine.io.samples import FloatArray, PassSamples


class Phase(IntEnum):
    NONE = 0
    BRAKE = 1
    ENTRY = 2
    MID = 3
    EXIT = 4


PHASE_NAMES = {Phase.BRAKE: "brake", Phase.ENTRY: "entry", Phase.MID: "mid", Phase.EXIT: "exit"}


@dataclass(frozen=True)
class Corner:
    """Where a corner is on the track, in metres from the start line."""

    number: int
    zone_start_m: float
    start_m: float
    end_m: float
    apex_m: float
    zone_end_m: float


@dataclass(frozen=True)
class Thresholds:
    brake_on: float = 0.08  # brake pedal counted as braking
    brake_off: float = 0.04
    steer_share: float = 0.3  # turn-in and unwind: share of peak steering
    flat_speed_loss: float = 0.04  # below this share of entry speed the corner is "flat"
    apex_margin_m: float = 30.0
    mid_speed_band: float = 1.03  # mid lasts while speed is within 3 % of the minimum
    mid_min_half_s: float = 0.15
    mid_half_s_by_lat: float = 0.3  # flat corners: +-0.3 s around peak lateral g


@dataclass(frozen=True)
class Segmentation:
    """Sample indices are 0-based and inclusive."""

    x: FloatArray  # metres from the zone start
    labels: npt.NDArray[np.int8]  # Phase per sample
    apex: int
    apex_by_lat: bool  # flat-out corner: the apex is the peak lateral g, not the slowest point
    v_min: float
    v_entry: float
    steer_peak: float
    lat_peak: float  # 95th percentile, so a kerb spike does not count as grip
    brake_start: int | None
    brake_end: int | None
    brake_peak: float
    turn_in: int
    unwind: int
    mid_start: int
    mid_end: int


def _first(mask: npt.NDArray[np.bool_], start: int, stop: int) -> int | None:
    """Index of the first True in mask[start:stop + 1], or None."""
    hits = np.flatnonzero(mask[start : stop + 1])
    return None if len(hits) == 0 else int(start + hits[0])


def _find_apex(s: PassSamples, x: FloatArray, o_start: float, o_end: float, th: Thresholds) -> int:
    near = (x >= o_start - th.apex_margin_m) & (x <= o_end + th.apex_margin_m)
    candidates = np.flatnonzero(near)
    if len(candidates) == 0:
        return int(np.argmin(s.v))
    return int(candidates[np.argmin(s.v[candidates])])


def _flat_corner_apex(s: PassSamples, x: FloatArray, o_start: float, o_end: float) -> int | None:
    inside = np.flatnonzero((x >= o_start) & (x <= o_end))
    if len(inside) == 0:
        return None
    return int(inside[np.argmax(np.abs(s.lat_g[inside]))])


def _braking(s: PassSamples, apex: int, th: Thresholds) -> tuple[int | None, int | None, float]:
    start = _first(s.brake > th.brake_on, 0, apex)
    if start is None:
        return None, None, 0.0
    peak, quiet = 0.0, 0
    for i in range(start, s.n):
        peak = max(peak, float(s.brake[i]))
        if s.brake[i] < th.brake_off:
            quiet += 1
            if quiet >= 3:
                return start, i - 2, peak
        else:
            quiet = 0
    return start, s.n - 1, peak


def _mid_window(
    s: PassSamples, apex: int, v_min: float, by_lat: bool, th: Thresholds
) -> tuple[int, int]:
    t, v = s.t, s.v
    first, last = apex, apex
    if by_lat:
        while first > 0 and t[apex] - t[first - 1] <= th.mid_half_s_by_lat:
            first -= 1
        while last < s.n - 1 and t[last + 1] - t[apex] <= th.mid_half_s_by_lat:
            last += 1
        return first, last
    limit = v_min * th.mid_speed_band
    while first > 0 and (v[first - 1] <= limit or t[apex] - t[first - 1] <= th.mid_min_half_s):
        first -= 1
    while last < s.n - 1 and (v[last + 1] <= limit or t[last + 1] - t[apex] <= th.mid_min_half_s):
        last += 1
    return first, last


def _labels(s: PassSamples, cut: dict[str, int | None], th: Thresholds) -> npt.NDArray[np.int8]:
    index = np.arange(s.n)
    labels = np.zeros(s.n, dtype=np.int8)
    turn_in, mid_a, mid_b, unwind = cut["turn_in"], cut["mid_start"], cut["mid_end"], cut["unwind"]
    assert turn_in is not None and mid_a is not None and mid_b is not None and unwind is not None
    labels[(index > mid_b) & (index <= unwind)] = Phase.EXIT
    in_mid = (index >= mid_a) & (index <= mid_b)
    labels[in_mid] = Phase.MID
    labels[in_mid & (s.brake > th.brake_off)] = Phase.ENTRY
    labels[(index >= turn_in) & (index < mid_a)] = Phase.ENTRY
    brake_a, brake_end = cut["brake_start"], cut["brake_end"]
    if brake_a is not None and brake_end is not None:
        labels[(index >= brake_a) & (index < min(turn_in, brake_end))] = Phase.BRAKE
    return labels


def segment(
    s: PassSamples, corner: Corner, track_length: float, th: Thresholds | None = None
) -> Segmentation:
    th = th or Thresholds()
    x = np.mod(s.d - corner.zone_start_m, track_length)
    o_start = (corner.start_m - corner.zone_start_m) % track_length
    o_end = (corner.end_m - corner.zone_start_m) % track_length
    abs_steer = np.abs(s.steer)
    steer_peak = float(abs_steer.max())
    lat_peak = float(np.sort(np.abs(s.lat_g))[max(1, int(0.95 * s.n)) - 1])

    apex = _find_apex(s, x, o_start, o_end, th)
    v_min = float(s.v[apex])
    v_entry = float(s.v[: apex + 1].max())
    braked = bool((s.brake[: apex + 1] > th.brake_on).any())
    by_lat = not braked and (v_entry - v_min) / max(v_entry, 1.0) < th.flat_speed_loss
    if by_lat:
        apex = _flat_corner_apex(s, x, o_start, o_end) or apex

    brake_start, brake_end, brake_peak = _braking(s, apex, th)
    threshold = th.steer_share * steer_peak
    turn_in = _first(abs_steer >= threshold, 0, apex)
    unwind = _first(abs_steer < threshold, apex, s.n - 1)
    mid_start, mid_end = _mid_window(s, apex, v_min, by_lat, th)
    cut = {
        "brake_start": brake_start, "brake_end": brake_end,
        "turn_in": apex if turn_in is None else turn_in,
        "unwind": s.n - 1 if unwind is None else unwind,
        "mid_start": mid_start, "mid_end": mid_end,
    }  # fmt: skip
    return Segmentation(
        x=x, labels=_labels(s, cut, th), apex=apex, apex_by_lat=by_lat, v_min=v_min,
        v_entry=v_entry, steer_peak=steer_peak, lat_peak=lat_peak, brake_start=brake_start,
        brake_end=brake_end, brake_peak=brake_peak, turn_in=cut["turn_in"] or 0,
        unwind=cut["unwind"] or 0, mid_start=mid_start, mid_end=mid_end,
    )  # fmt: skip
