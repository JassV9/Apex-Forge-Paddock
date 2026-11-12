"""Everything the engine measures on one corner pass, as named metrics.

A metric's name is its id everywhere else: in corner_pass.features, in evidence that a
hypothesis cites, and in experiment results. Passes recorded by the agent get the full set
from raw samples; passes imported from Apex Forge v0 get the subset v0 stored.
"""

from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

import numpy as np

from paddock_engine.features import channels
from paddock_engine.features.axles import axle_channels
from paddock_engine.features.inputs import LOCK_SLIP_RATIO, DriverInputs, driver_inputs
from paddock_engine.features.phase_stats import PhaseStats, phase_stats
from paddock_engine.io.samples import PassSamples
from paddock_engine.phases.segment import Corner, segment

MIN_SAMPLES = 10
OFF_TRACK_WHEELS = 3

Metrics = dict[str, float | None]


@dataclass(frozen=True)
class PassFeatures:
    metrics: Metrics
    speed_class: str  # slow, medium or fast: the band of the corner's minimum speed
    valid: bool = True
    invalid_reason: str | None = None
    flags: list[str] = field(default_factory=list)


def speed_class(v_min_kmh: float) -> str:
    """Corner class by minimum speed: slow (mechanical grip) to fast (aero)."""
    if v_min_kmh < 100:
        return "slow"
    return "medium" if v_min_kmh < 150 else "fast"


def _phase_metrics(phases: Mapping[str, PhaseStats]) -> Metrics:
    out: Metrics = {}
    for name, stats in phases.items():
        out[f"confidence_{name}"] = stats.confidence
        out[f"front_slip_{name}_deg"] = stats.front_slip_deg
        out[f"rear_slip_{name}_deg"] = stats.rear_slip_deg
        out[f"steer_rate_{name}"] = stats.steer_rate
    return out


def _input_metrics(inputs: DriverInputs) -> Metrics:
    return {name: (None if value is None else float(value)) for name, value in vars(inputs).items()}


def compute(
    samples: PassSamples,
    corner: Corner,
    track_length: float,
    duration_s: float,
    driven: str = "rear",
) -> PassFeatures:
    """Features of a pass recorded by the agent. `driven` is the driven axle."""
    if samples.n < MIN_SAMPLES:
        return PassFeatures({"corner_time_s": duration_s}, "slow", False, "too few samples")
    seg = segment(samples, corner, track_length)
    axles = axle_channels(samples)
    phases = phase_stats(samples, seg, axles, corner, track_length)
    driven_ratio = {
        "front": axles.front_ratio,
        "all": np.maximum(axles.front_ratio, axles.rear_ratio),
    }.get(driven, axles.rear_ratio)
    metrics: Metrics = {
        "corner_time_s": duration_s,
        "v_min_kmh": seg.v_min * 3.6,
        "v_entry_kmh": seg.v_entry * 3.6,
        "exit_speed_100m_kmh": channels.exit_speed(samples, seg, corner, track_length),
        "lat_g_peak": seg.lat_peak,
        "steer_per_g_mid": channels.steer_per_g(samples, seg),
        "front_saturation_index": channels.front_saturation_index(samples, seg),
        "yaw_gain_entry": channels.yaw_gain(samples, seg),
        "rear_slip_peak_deg": channels.rear_slip_peak(axles.rear_slip),
        "wheelspin_s": channels.wheelspin_time(samples, driven_ratio, LOCK_SLIP_RATIO),
        **_phase_metrics(phases),
        **_input_metrics(driver_inputs(samples, seg, axles, driven)),
    }
    off_track = float(samples.out.max()) >= OFF_TRACK_WHEELS
    return PassFeatures(
        metrics=metrics,
        speed_class=speed_class(seg.v_min * 3.6),
        valid=not off_track,
        invalid_reason="off track" if off_track else None,
        flags=["flat-out corner"] if seg.apex_by_lat else [],
    )


def from_legacy(legacy: Mapping[str, Any], duration_s: float) -> PassFeatures:
    """Features of a pass imported from Apex Forge v0 (contract: LegacySummary)."""
    metrics: Metrics = {"corner_time_s": duration_s, "v_min_kmh": float(legacy["v_min_kmh"])}
    for name, phase in legacy.get("phases", {}).items():
        metrics[f"confidence_{name}"] = float(phase["confidence"])
        metrics[f"front_slip_{name}_deg"] = float(phase["front_slip_deg"])
        metrics[f"rear_slip_{name}_deg"] = float(phase["rear_slip_deg"])
        metrics[f"steer_rate_{name}"] = float(phase["steer_rate"])
    off_track = bool(legacy.get("off_track", False))
    return PassFeatures(
        metrics=metrics,
        speed_class=speed_class(float(legacy["v_min_kmh"])),
        valid=not off_track,
        invalid_reason="off track" if off_track else None,
        flags=[f"imported from {legacy['source']}: no raw samples"],
    )
