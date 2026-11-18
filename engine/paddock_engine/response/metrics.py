"""Vehicle-response metrics of one pass over one frozen phase (ADR 0031).

Every metric is computed on the pass's 1 m grid (phases/align.py), over the metres of the phase
as the experiment's cuts define them, so both arms are measured over the same track. Means are
distance-weighted. A metric that has too few qualifying metres is None, never zero. What each
metric is, its unit and its margin are in response/metrics.toml; the names here and there
must match (tests/test_response.py).
"""

from collections.abc import Callable
from dataclasses import dataclass

import numpy as np
import numpy.typing as npt

from paddock_engine.io.samples import FloatArray
from paddock_engine.phases.align import Aligned

MIN_METRES = 3
LAT_MIN_G = 0.3
LAT_SHARE_OF_PEAK = 0.45
LOCK_SLIP_RATIO = 0.18
BRAKING = 0.2
POWERED = 0.3
BRAKE_OFF = 0.04
COASTING_GAS = 0.1


@dataclass(frozen=True)
class Slice:
    """The metres [start, end] of one pass, with the pass's own peaks."""

    p: Aligned
    rows: npt.NDArray[np.intp]
    steer_p95: float
    lat_p95: float
    start_m: float
    end_m: float


def phase_slice(p: Aligned, start_m: float, end_m: float) -> Slice | None:
    """None when the pass does not cover the phase."""
    if not p.covers(start_m, end_m):
        return None
    own = p.x <= p.zone_length_m
    rows = np.flatnonzero((p.x >= start_m) & (p.x <= end_m))
    return Slice(
        p=p,
        rows=rows,
        steer_p95=float(np.percentile(np.abs(p.steer[own]), 95)),
        lat_p95=float(np.percentile(np.abs(p.lat_g[own]), 95)),
        start_m=start_m,
        end_m=end_m,
    )


def _mean(values: FloatArray, mask: npt.NDArray[np.bool_] | None = None) -> float | None:
    chosen = values if mask is None else values[mask]
    return float(chosen.mean()) if len(chosen) >= MIN_METRES else None


def _cornering(s: Slice) -> npt.NDArray[np.bool_]:
    gate = max(LAT_MIN_G, LAT_SHARE_OF_PEAK * s.lat_p95)
    cornering: npt.NDArray[np.bool_] = np.abs(s.p.lat_g[s.rows]) >= gate
    return cornering


def phase_time(s: Slice) -> float:
    return s.p.time_at(s.end_m) - s.p.time_at(s.start_m)


def steer_per_g(s: Slice) -> float | None:
    mask = _cornering(s)
    steer, lat = _mean(np.abs(s.p.steer[s.rows]), mask), _mean(np.abs(s.p.lat_g[s.rows]), mask)
    return None if steer is None or not lat else steer / lat


def yaw_gain(s: Slice) -> float | None:
    v, steer, yaw = s.p.v[s.rows], np.abs(s.p.steer[s.rows]), np.abs(s.p.yaw[s.rows])
    mask = (v > 5.0) & (steer > 0.1 * s.steer_p95) & (steer > 0)
    gains = 1000 * yaw / (np.maximum(v, 5.0) * np.maximum(steer, 1e-6))
    return _mean(gains, mask)


def _slip(s: Slice, axle: FloatArray) -> float | None:
    return _mean(axle[s.rows], _cornering(s))


def _ratio(s: Slice, axle: FloatArray, powered: bool) -> float | None:
    if powered:
        mask = (s.p.gas[s.rows] > POWERED) & (s.p.brake[s.rows] < BRAKE_OFF)
    else:
        mask = s.p.brake[s.rows] > BRAKING
    return _mean(axle[s.rows], mask)


def _lock_share(s: Slice, axle: FloatArray) -> float | None:
    braking = s.p.brake[s.rows] > BRAKING
    locked = (axle[s.rows] > LOCK_SLIP_RATIO).astype(np.float64)
    return _mean(locked, braking)


def steer_reversals_per_s(s: Slice) -> float | None:
    """Reversals with hysteresis, per second of the phase."""
    duration = phase_time(s)
    if len(s.rows) < MIN_METRES or duration <= 0:
        return None
    hysteresis = max(1.5, 0.04 * s.steer_p95)
    direction, reference, reversals = 0, float(s.p.steer[s.rows[0]]), 0
    for angle in s.p.steer[s.rows]:
        if direction >= 0 and angle < reference - hysteresis:
            reversals += direction > 0
            direction, reference = -1, float(angle)
        elif direction <= 0 and angle > reference + hysteresis:
            reversals += direction < 0
            direction, reference = 1, float(angle)
        elif (direction > 0 and angle > reference) or (direction < 0 and angle < reference):
            reference = float(angle)
    return reversals / duration


def steer_rate(s: Slice) -> float | None:
    if len(s.rows) < MIN_METRES:
        return None
    dt = np.diff(s.p.t[s.rows])
    moving = dt > 0
    rates = np.abs(np.diff(s.p.steer[s.rows]))[moving] / dt[moving]
    return float(rates.mean()) if len(rates) else None


def coast(s: Slice) -> float | None:
    if len(s.rows) < 2:
        return None
    dt = np.diff(s.p.t[s.rows])
    gas, brake = s.p.gas[s.rows][1:], s.p.brake[s.rows][1:]
    return float(dt[(gas < COASTING_GAS) & (brake < BRAKE_OFF)].sum())


def _sd(values: FloatArray) -> float | None:
    return float(values.std(ddof=1)) if len(values) >= MIN_METRES else None


def _kmh(values: FloatArray, pick: Callable[[FloatArray], float]) -> float | None:
    return pick(values) * 3.6 if len(values) else None


PhaseMetric = Callable[[Slice], float | None]

PHASE_METRICS: dict[str, PhaseMetric] = {
    "phase_time_s": phase_time,
    "v_min_kmh": lambda s: _kmh(s.p.v[s.rows], lambda v: float(v.min())),
    "v_start_kmh": lambda s: float(np.interp(s.start_m, s.p.x, s.p.v)) * 3.6,
    "v_end_kmh": lambda s: float(np.interp(s.end_m, s.p.x, s.p.v)) * 3.6,
    "v_max_kmh": lambda s: _kmh(s.p.v[s.rows], lambda v: float(v.max())),
    "lat_g_mean": lambda s: _mean(np.abs(s.p.lat_g[s.rows])),
    "long_g_mean": lambda s: _mean(s.p.lon_g[s.rows]),
    "peak_decel_g": lambda s: float(-s.p.lon_g[s.rows].min()) if len(s.rows) else None,
    "steer_per_g": steer_per_g,
    "yaw_gain": yaw_gain,
    "front_slip_deg": lambda s: _slip(s, s.p.front_slip),
    "rear_slip_deg": lambda s: _slip(s, s.p.rear_slip),
    "slip_balance_deg": lambda s: _mean((s.p.front_slip - s.p.rear_slip)[s.rows], _cornering(s)),
    "front_slip_ratio_braking": lambda s: _ratio(s, s.p.front_ratio, powered=False),
    "rear_slip_ratio_braking": lambda s: _ratio(s, s.p.rear_ratio, powered=False),
    "front_slip_ratio_traction": lambda s: _ratio(s, s.p.front_ratio, powered=True),
    "rear_slip_ratio_traction": lambda s: _ratio(s, s.p.rear_ratio, powered=True),
    "front_lock_share": lambda s: _lock_share(s, s.p.front_ratio),
    "rear_lock_share": lambda s: _lock_share(s, s.p.rear_ratio),
    "steer_rate_deg_s": steer_rate,
    "steer_reversals_per_s": steer_reversals_per_s,
    "coast_s": coast,
    "brake_mean": lambda s: _mean(s.p.brake[s.rows]),
    "gas_mean": lambda s: _mean(s.p.gas[s.rows]),
    "ride_height_front_mm": lambda s: _mean(1000 * s.p.rh_f[s.rows]),
    "ride_height_rear_mm": lambda s: _mean(1000 * s.p.rh_r[s.rows]),
    "ride_height_front_sd_mm": lambda s: _sd(1000 * s.p.rh_f[s.rows]),
    "ride_height_rear_sd_mm": lambda s: _sd(1000 * s.p.rh_r[s.rows]),
}

# One value per pass, from the corner as a whole: computed in response/passes.py.
CORNER_METRICS = (
    "corner_time_s",
    "brake_point_m",
    "turn_in_point_m",
    "apex_point_m",
    "pickup_point_m",
    "end_point_m",
)
