"""Response channels that v0 did not have (docs/design.md, section E.1).

v0 judged balance from one number, front minus rear slip. That number cannot tell a front
axle that is out of grip from a driver asking for more than the tyre has. These channels ask
the questions it could not:

- steer_per_g: how much steering each g of cornering costs. Compared within one corner, where
  the geometry is fixed, a change is a change in understeer gradient.
- front_saturation_index: share of the turn where steering keeps rising and lateral g does not
  follow. High means the front is past its peak: more lock is not buying more grip.
- yaw_gain: how readily the car rotates for the steering applied on entry.
"""

import numpy as np

from paddock_engine.features.phase_stats import lat_gate
from paddock_engine.io.samples import FloatArray, PassSamples
from paddock_engine.phases.segment import Corner, Phase, Segmentation

MIN_SAMPLES = 3
SATURATION_WINDOW_S = 0.2
STEER_RISE_SHARE = 0.03  # of peak steering, within the window: "steering is rising"
LAT_RISE_SHARE = 0.01  # of peak lateral g, within the window: "lateral g is following"
EXIT_SPEED_DISTANCE_M = 100.0


def steer_per_g(s: PassSamples, seg: Segmentation) -> float | None:
    """Mean steering wheel angle per g of lateral acceleration in the mid phase, deg/g."""
    mid = (seg.labels == Phase.MID) & (np.abs(s.lat_g) >= lat_gate(seg))
    if mid.sum() < MIN_SAMPLES:
        return None
    return float(np.abs(s.steer[mid]).mean() / np.abs(s.lat_g[mid]).mean())


def _change_over_window(values: FloatArray, t: FloatArray, window_s: float) -> FloatArray:
    """values[i] minus the value `window_s` earlier (linear interpolation); NaN before that."""
    earlier = np.interp(t - window_s, t, values)
    change = values - earlier
    change[t - t[0] < window_s] = np.nan
    return change


def front_saturation_index(s: PassSamples, seg: Segmentation) -> float | None:
    """Share of entry-plus-mid time with steering rising while lateral g does not follow."""
    turning = (seg.labels == Phase.ENTRY) | (seg.labels == Phase.MID)
    steer_change = _change_over_window(np.abs(s.steer), s.t, SATURATION_WINDOW_S)
    lat_change = _change_over_window(np.abs(s.lat_g), s.t, SATURATION_WINDOW_S)
    usable = turning & ~np.isnan(steer_change) & (np.abs(s.lat_g) >= lat_gate(seg))
    if usable.sum() < MIN_SAMPLES:
        return None
    saturated = (
        usable
        & (steer_change > STEER_RISE_SHARE * seg.steer_peak)
        & (lat_change <= LAT_RISE_SHARE * seg.lat_peak)
    )
    dt = np.diff(s.t, prepend=s.t[0])
    return float(dt[saturated].sum() / dt[usable].sum())


def yaw_gain(s: PassSamples, seg: Segmentation) -> float | None:
    """Mean yaw rate per unit of speed x steering on entry, in 1e-3 rad/(m deg)."""
    entry = (
        (seg.labels == Phase.ENTRY) & (s.v > 5.0) & (np.abs(s.steer) > 0.1 * seg.steer_peak)
    )  # fmt: skip
    if entry.sum() < MIN_SAMPLES:
        return None
    return float(1000 * np.mean(np.abs(s.yaw[entry]) / (s.v[entry] * np.abs(s.steer[entry]))))


def exit_speed(
    s: PassSamples, seg: Segmentation, corner: Corner, track_length: float
) -> float | None:
    """Speed 100 m after the corner ends, km/h; None when the zone ends before that."""
    target = (corner.end_m - corner.zone_start_m) % track_length + EXIT_SPEED_DISTANCE_M
    if target > seg.x[-1]:
        return None
    return float(np.interp(target, seg.x, s.v) * 3.6)


def rear_slip_peak(rear_slip: FloatArray) -> float:
    """95th percentile of rear axle slip over the pass, degrees: how far the rear stepped out."""
    return float(np.percentile(rear_slip, 95))


def wheelspin_time(s: PassSamples, driven_ratio: FloatArray, threshold: float) -> float:
    """Seconds on the throttle with the driven wheels slipping."""
    spinning = (s.gas > 0.3) & (s.brake < 0.04) & (driven_ratio > threshold)
    return float(np.diff(s.t, prepend=s.t[0])[spinning].sum())
