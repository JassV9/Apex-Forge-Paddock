"""Synthetic corner passes with known events, sampled like the agent samples (ADR 0031 tests).

A pass is defined on distance: straight at 60 m/s, braking from B, turn-in at TI, the slowest
point at the apex, unwinding to the corner end, then accelerating. It is sampled in time at
15 to 22 ms steps (the agent's frame-rate jitter), so no two passes share sample positions.
"""

from dataclasses import dataclass
from typing import Any

import numpy as np

from paddock_engine.io.samples import COLUMN_NAMES, FloatArray, PassSamples

ZONE_M = 400.0
FINE_M = 0.05


@dataclass(frozen=True)
class Shape:
    braking_m: float | None = 150.0
    turn_in_m: float = 190.0
    apex_m: float = 230.0
    end_m: float = 290.0
    v_top: float = 60.0
    v_apex: float = 25.0
    v_exit: float = 50.0
    rear_slip_extra_deg: float = 0.0  # added to the rear slip angle from turn-in to the end
    slow_stretch: tuple[float, float, float] | None = None  # from, to, speed factor


def _profiles(shape: Shape, x: FloatArray) -> dict[str, FloatArray]:
    start = shape.braking_m if shape.braking_m is not None else shape.turn_in_m
    down = np.clip((x - start) / max(shape.apex_m - start, 1.0), 0, 1)
    up = np.clip((x - shape.apex_m) / (ZONE_M - shape.apex_m), 0, 1)
    v = np.where(
        x < shape.apex_m,
        shape.v_top - (shape.v_top - shape.v_apex) * down,
        shape.v_apex + (shape.v_exit - shape.v_apex) * up,
    )
    if shape.slow_stretch is not None:
        start, stop, factor = shape.slow_stretch
        v = np.where((x >= start) & (x < stop), v * factor, v)
    rise = np.clip((x - shape.turn_in_m) / 25.0, 0, 1)
    fall = np.clip((shape.end_m - x) / 40.0, 0, 1)
    turn = rise * fall
    braking = (
        np.zeros_like(x)
        if shape.braking_m is None
        else np.where((x >= shape.braking_m) & (x < shape.turn_in_m + 20), 0.8, 0.0)
    )
    gas = np.where((braking > 0) | ((x > shape.turn_in_m) & (x < shape.apex_m + 10)), 0.0, 1.0)
    lat = 1.5 * turn
    rear = 3.0 * turn + shape.rear_slip_extra_deg * (turn > 0)
    return {"v": v, "steer": 90.0 * turn, "lat_g": lat, "brake": braking, "gas": gas,
            "yaw": lat * 9.81 / v, "front": 5.0 * turn, "rear": rear}  # fmt: skip


def sampled(
    shape: Shape, rng: np.random.Generator, zone_start_m: float = 0.0, track_length: float = 5000.0
) -> PassSamples:
    """The pass sampled at jittered time steps, distance on the track (wrapping at the line)."""
    x = np.arange(0.0, ZONE_M + FINE_M, FINE_M)
    lines = _profiles(shape, x)
    t_of_x = np.concatenate([[0.0], np.cumsum(FINE_M / lines["v"][:-1])])
    steps = rng.uniform(0.015, 0.022, size=int(t_of_x[-1] / 0.015) + 2)
    t = np.concatenate([[0.0], np.cumsum(steps)])
    t = t[t <= t_of_x[-1]]
    at = np.interp(t, t_of_x, x)
    columns: dict[str, Any] = {name: np.zeros_like(t) for name in COLUMN_NAMES}
    for name in ("v", "steer", "lat_g", "brake", "gas", "yaw"):
        columns[name] = np.interp(at, x, lines[name])
    for wheel in ("fl", "fr", "rl", "rr"):
        side = "front" if wheel[0] == "f" else "rear"
        columns[f"sa_{wheel}"] = np.interp(at, x, lines[side])
        columns[f"load_{wheel}"] = np.full_like(t, 3000.0)
    columns.update(t=t, d=np.mod(at + zone_start_m, track_length), rh_f=np.full_like(t, 0.03),
                   rh_r=np.full_like(t, 0.05))  # fmt: skip
    return PassSamples.from_columns(columns)


def analytic_time(shape: Shape, start_m: float, end_m: float) -> float:
    x = np.arange(start_m, end_m, FINE_M / 10)
    return float(np.sum((FINE_M / 10) / _profiles(shape, x)["v"]))
