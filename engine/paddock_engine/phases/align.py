"""A corner pass on a 1 m distance grid, so passes can be compared metre by metre (ADR 0031).

The agent samples at frame rate, about 55 Hz with 15 to 22 ms of jitter, so two passes never
share sample positions. Comparing them in time mixes "where" with "when". On a distance grid,
the same metre of track is the same index in every pass, and the time a pass took between two
points is t(x_end) - t(x_start).

Steps: unwrap the spline distance across the start line, express it in metres from the zone
start, drop samples that do not move forward (the spline position can step back by a few
centimetres, and a resampling needs increasing positions), and interpolate time, speed, the
inputs and the axle channels onto whole metres.
"""

from dataclasses import dataclass

import numpy as np
import numpy.typing as npt

from paddock_engine.features.axles import axle_channels
from paddock_engine.io.samples import FloatArray, PassSamples

GRID_STEP_M = 1.0


@dataclass(frozen=True)
class Aligned:
    """Channels of one pass at whole metres from the zone start. `x` is the grid."""

    x: FloatArray
    t: FloatArray  # seconds since the pass started
    v: FloatArray  # m/s
    gas: FloatArray
    brake: FloatArray
    steer: FloatArray  # degrees, signed
    lat_g: FloatArray
    lon_g: FloatArray
    yaw: FloatArray  # rad/s
    front_slip: FloatArray  # load-weighted |slip angle| of the axle, degrees (CSP, direct)
    rear_slip: FloatArray
    front_ratio: FloatArray  # larger |slip ratio| of the axle's two tyres
    rear_ratio: FloatArray
    rh_f: FloatArray  # ride height, metres
    rh_r: FloatArray
    zone_length_m: float  # where the pass's own zone ends; beyond it is the next pass (joined)

    @property
    def n(self) -> int:
        return len(self.x)

    def time_at(self, x_m: float) -> float:
        return float(np.interp(x_m, self.x, self.t))

    def covers(self, start_m: float, end_m: float) -> bool:
        return self.n > 1 and self.x[0] <= start_m and self.x[-1] >= end_m


def unwrap_distance(d: FloatArray, zone_start_m: float, track_length: float) -> FloatArray:
    """Metres from the zone start, continuous across the start line."""
    steps = np.diff(d)
    laps = np.concatenate([[0.0], np.cumsum((steps < -track_length / 2).astype(np.float64))])
    laps -= np.concatenate([[0.0], np.cumsum((steps > track_length / 2).astype(np.float64))])
    x = d + laps * track_length - zone_start_m
    # the first sample sits within half a lap of the zone start
    shift = np.round(x[0] / track_length) * track_length
    out: FloatArray = x - shift
    return out


def forward_only(x: FloatArray) -> npt.NDArray[np.bool_]:
    """Mask of samples that move forward: each beyond every sample kept before it."""
    before: FloatArray = np.concatenate([[-np.inf], np.maximum.accumulate(x)[:-1]])
    keep: npt.NDArray[np.bool_] = x > before
    return keep


def align(
    s: PassSamples, zone_start_m: float, track_length: float, zone_length_m: float | None = None
) -> Aligned:
    """Resample a pass (possibly joined with the next one) onto whole metres.

    `zone_length_m` is the length of the pass's own zone; by default the whole pass.
    """
    x = unwrap_distance(s.d, zone_start_m, track_length)
    keep = forward_only(x)
    x = x[keep]
    if len(x) < 2:
        raise ValueError("a pass needs two samples that move forward to be aligned")
    grid = np.arange(np.ceil(x[0]), np.floor(x[-1]) + GRID_STEP_M / 2, GRID_STEP_M)
    axles = axle_channels(s)

    def on_grid(values: FloatArray) -> FloatArray:
        resampled: FloatArray = np.interp(grid, x, values[keep])
        return resampled

    return Aligned(
        x=grid, t=on_grid(s.t), v=on_grid(s.v), gas=on_grid(s.gas), brake=on_grid(s.brake),
        steer=on_grid(s.steer), lat_g=on_grid(s.lat_g), lon_g=on_grid(s.lon_g),
        yaw=on_grid(s.yaw), front_slip=on_grid(axles.front_slip),
        rear_slip=on_grid(axles.rear_slip), front_ratio=on_grid(axles.front_ratio),
        rear_ratio=on_grid(axles.rear_ratio), rh_f=on_grid(s.rh_f), rh_r=on_grid(s.rh_r),
        zone_length_m=float(x[-1]) if zone_length_m is None else zone_length_m,
    )  # fmt: skip


def delta_t(
    aligned: Aligned, reference_x: FloatArray, reference_t: FloatArray, start_m: float
) -> FloatArray:
    """Time lost (+) or gained (-) against the reference since `start_m`, at reference_x.

    Descriptive only: the inference is on scalar per-phase metrics, never on this trace.
    NaN where the pass does not cover the grid point.
    """
    own = np.interp(reference_x, aligned.x, aligned.t) - aligned.time_at(start_m)
    ref = reference_t - float(np.interp(start_m, reference_x, reference_t))
    out: FloatArray = np.where(
        (reference_x >= aligned.x[0]) & (reference_x <= aligned.x[-1]), own - ref, np.nan
    )
    return out
