"""Averages per phase of one corner pass, and how far each can be trusted.

Slip, yaw and lateral g are averaged only over samples where the car is really cornering
(lateral g above 0.3 g and above 45 % of the pass's peak). Ported from Apex Forge v0
`core/analyze.lua` ("diag" and "conf"); the field names are those of the LegacyPhase contract,
so imported v0 data and new data read the same.
"""

from dataclasses import dataclass

import numpy as np

from paddock_engine.features.axles import AxleChannels
from paddock_engine.io.samples import FloatArray, PassSamples
from paddock_engine.phases.segment import PHASE_NAMES, Corner, Phase, Segmentation

LAT_MIN_G = 0.3
LAT_SHARE_OF_PEAK = 0.45
LONG_PLATEAU_S = 2.0


@dataclass(frozen=True)
class PhaseStats:
    confidence: float  # 0..1: coverage x enough samples x steady steering (x mid-shape for mid)
    duration_s: float
    coverage: float  # share of the phase spent really cornering
    front_slip_deg: float
    rear_slip_deg: float
    front_slip_max_deg: float
    rear_slip_max_deg: float
    lat_g: float
    yaw_rate: float
    steer_rate: float  # share of peak steering angle per second
    gas: float
    brake: float


def lat_gate(seg: Segmentation) -> float:
    return max(LAT_MIN_G, LAT_SHARE_OF_PEAK * seg.lat_peak)


def _speed_minima(s: PassSamples, seg: Segmentation, corner: Corner, track_length: float) -> int:
    """Separate speed minima inside the corner: 2 or more is a double apex."""
    o_start = (corner.start_m - corner.zone_start_m) % track_length
    o_end = (corner.end_m - corner.zone_start_m) % track_length
    minima, rising = 0, False
    for i in range(1, s.n):
        if not (o_start - 30 <= seg.x[i] <= o_end + 30):
            continue
        if s.v[i] <= seg.v_min * 1.02 and (minima == 0 or rising):
            minima, rising = minima + 1, False
        elif s.v[i] > seg.v_min * 1.05:
            rising = True
    return minima


def _mid_shape_factor(
    s: PassSamples, seg: Segmentation, corner: Corner, track_length: float
) -> float:
    """Mid readings are trusted less on a long speed plateau and on a double apex."""
    factor = 1.0
    if s.t[seg.mid_end] - s.t[seg.mid_start] > LONG_PLATEAU_S:
        factor *= 0.6
    if _speed_minima(s, seg, corner, track_length) >= 2:
        factor *= 0.4
    return factor


def _one_phase(
    s: PassSamples, seg: Segmentation, axles: AxleChannels, phase: Phase
) -> PhaseStats | None:
    members = np.flatnonzero(seg.labels == phase)
    total = len(members)
    if total == 0:
        return None
    later = members[members > 0]
    dt = s.t[later] - s.t[later - 1]
    moving = dt > 0
    steer_change = np.abs(s.steer[later] - s.steer[later - 1])[moving] / dt[moving]
    rate = float(steer_change.sum()) / (total - 1) / max(seg.steer_peak, 1.0) if total > 1 else 0.0

    cornering = members[np.abs(s.lat_g[members]) >= lat_gate(seg)]
    count = len(cornering)
    coverage = count / total
    steady = min(1.0, max(0.3, 2.0 - rate))

    def mean(values: FloatArray) -> float:
        return float(values[cornering].mean()) if count else 0.0

    return PhaseStats(
        confidence=coverage * min(1.0, count / 5) * steady,
        duration_s=float(dt.sum()),
        coverage=coverage,
        front_slip_deg=mean(axles.front_slip),
        rear_slip_deg=mean(axles.rear_slip),
        front_slip_max_deg=mean(axles.front_slip_max),
        rear_slip_max_deg=mean(axles.rear_slip_max),
        lat_g=mean(np.abs(s.lat_g)),
        yaw_rate=mean(np.abs(s.yaw)),
        steer_rate=rate,
        gas=float(s.gas[members].mean()),
        brake=float(s.brake[members].mean()),
    )


def phase_stats(
    s: PassSamples, seg: Segmentation, axles: AxleChannels, corner: Corner, track_length: float
) -> dict[str, PhaseStats]:
    """Stats for every phase that has at least one sample, keyed by phase name."""
    out: dict[str, PhaseStats] = {}
    for phase, name in PHASE_NAMES.items():
        stats = _one_phase(s, seg, axles, phase)
        if stats is None:
            continue
        if phase is Phase.MID:
            shape = _mid_shape_factor(s, seg, corner, track_length)
            stats = PhaseStats(**{**stats.__dict__, "confidence": stats.confidence * shape})
        out[name] = stats
    return out
