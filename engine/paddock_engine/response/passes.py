"""An experiment's passes on the 1 m grid, cut at frozen metres, measured per phase (ADR 0031).

1. Each pass is joined with the next pass of the same lap when their zones touch
   (phases/join.py), then aligned to whole metres from its zone start (phases/align.py).
2. Per corner, the baseline arm's valid passes give the reference and the frozen cuts
   (phases/reference.py). The variant never moves them.
3. Every valid pass of both arms is measured over those same metres: one value per metric and
   cut phase (response/metrics.py), and its events and corner time.
"""

from collections.abc import Mapping, Sequence, Set
from dataclasses import dataclass, field

import numpy as np

from paddock_engine.io.samples import FloatArray, PassSamples
from paddock_engine.phases.align import Aligned, align, delta_t
from paddock_engine.phases.cuts import CornerCuts
from paddock_engine.phases.events import CornerWindow, pass_events
from paddock_engine.phases.join import follows, joined, zones_touch
from paddock_engine.phases.reference import Trace, corner_reference
from paddock_engine.response.metrics import PHASE_METRICS, phase_slice

BASELINE = "baseline"
DELTA_STEP_M = 10.0

Cell = tuple[str, str]  # (metric, phase)


@dataclass(frozen=True)
class Geometry:
    number: int
    zone_start_m: float
    start_m: float
    end_m: float
    zone_end_m: float


@dataclass(frozen=True)
class PassRef:
    session: int
    seq: int
    corner: int
    arm: str
    block: int
    t_start_s: float
    duration_s: float
    valid: bool = True
    lap: int | None = None
    covariates: Mapping[str, float] = field(default_factory=dict)


@dataclass(frozen=True)
class Measured:
    ref: PassRef
    values: dict[Cell, float | None]


@dataclass(frozen=True)
class CornerView:
    cuts: CornerCuts
    trace: Trace
    baseline_v_min_kmh: float  # median of the baseline passes' slowest metre
    baseline_lat_peak_g: float  # median of the baseline passes' p95 lateral g
    delta_t: list[tuple[float, float]]  # (metre, variant minus baseline, s): descriptive


def _with_next(
    ref: PassRef,
    samples: Mapping[tuple[int, int], PassSamples],
    by_key: Mapping[tuple[int, int], PassRef],
    geometry: Mapping[int, Geometry],
    track_length: float,
) -> PassSamples:
    own = samples[(ref.session, ref.seq)]
    after = by_key.get((ref.session, ref.seq + 1))
    if after is None or (ref.session, ref.seq + 1) not in samples:
        return own
    touch = zones_touch(
        geometry[ref.corner].zone_end_m, geometry[after.corner].zone_start_m, track_length
    )
    if not touch or not follows(ref.t_start_s, ref.duration_s, after.t_start_s):
        return own
    return joined(own, samples[(after.session, after.seq)], after.t_start_s - ref.t_start_s)


def aligned_passes(
    refs: Sequence[PassRef],
    samples: Mapping[tuple[int, int], PassSamples],
    geometry: Mapping[int, Geometry],
    track_length: float,
) -> list[tuple[PassRef, Aligned]]:
    """The valid passes that have samples, each joined with its successor where it can be."""
    by_key = {(r.session, r.seq): r for r in refs}
    out = []
    for ref in refs:
        if not ref.valid or (ref.session, ref.seq) not in samples or ref.corner not in geometry:
            continue
        corner = geometry[ref.corner]
        zone_length = (corner.zone_end_m - corner.zone_start_m) % track_length
        whole = _with_next(ref, samples, by_key, geometry, track_length)
        out.append((ref, align(whole, corner.zone_start_m, track_length, zone_length)))
    return out


def _window(corner: Geometry, track_length: float) -> CornerWindow:
    return CornerWindow(
        (corner.start_m - corner.zone_start_m) % track_length,
        (corner.end_m - corner.zone_start_m) % track_length,
    )


def _corner_metrics(p: Aligned, cuts: CornerCuts, window: CornerWindow) -> dict[Cell, float | None]:
    events = pass_events(p, window)
    measured = cuts.measured()
    start, end = measured[0].start_m, measured[-1].end_m
    corner_time = p.time_at(end) - p.time_at(start) if p.covers(start, end) else None
    return {
        ("corner_time_s", "corner"): corner_time,
        ("brake_point_m", "corner"): events.braking_m,
        ("turn_in_point_m", "corner"): events.turn_in_m,
        ("apex_point_m", "corner"): events.apex_m,
        ("pickup_point_m", "corner"): events.pickup_m,
        ("end_point_m", "corner"): events.end_m,
    }


def measure(
    p: Aligned, cuts: CornerCuts, window: CornerWindow, wanted: Set[Cell]
) -> dict[Cell, float | None]:
    """The corner metrics, and each wanted phase metric over each cut phase."""
    values = {cell: v for cell, v in _corner_metrics(p, cuts, window).items() if cell in wanted}
    for span in cuts.measured():
        piece = phase_slice(p, span.start_m, span.end_m)
        for name, compute in PHASE_METRICS.items():
            if (name, span.name) in wanted:
                values[(name, span.name)] = None if piece is None else compute(piece)
    return values


def _delta_trace(
    variant: Sequence[Aligned], trace: Trace, cuts: CornerCuts
) -> list[tuple[float, float]]:
    measured = cuts.measured()
    start, end = measured[0].start_m, measured[-1].end_m
    grid = np.arange(start, end + 0.5, DELTA_STEP_M)
    if not variant or len(grid) == 0:
        return []
    deltas = np.vstack([delta_t(p, trace.x, trace.t, start) for p in variant])
    at_grid = [np.interp(grid, trace.x, row) for row in deltas]
    median: FloatArray = np.nanmedian(np.vstack(at_grid), axis=0)
    return [(float(x), float(d)) for x, d in zip(grid, median, strict=True) if np.isfinite(d)]


def corner_views(
    passes: Sequence[tuple[PassRef, Aligned]], geometry: Mapping[int, Geometry], track_length: float
) -> dict[int, CornerView]:
    """Per corner with baseline passes: the frozen cuts and what the baseline looks like."""
    views = {}
    for number in sorted({ref.corner for ref, _ in passes}):
        baseline = [p for ref, p in passes if ref.corner == number and ref.arm == BASELINE]
        variant = [p for ref, p in passes if ref.corner == number and ref.arm != BASELINE]
        if not baseline:
            continue
        corner = geometry[number]
        zone = (corner.zone_end_m - corner.zone_start_m) % track_length
        cuts, trace, events = corner_reference(
            number, baseline, _window(corner, track_length), zone
        )
        peaks = [np.percentile(np.abs(p.lat_g[p.x <= p.zone_length_m]), 95) for p in baseline]
        views[number] = CornerView(
            cuts=cuts,
            trace=trace,
            baseline_v_min_kmh=float(np.median([e.v_min_kmh for e in events])),
            baseline_lat_peak_g=float(np.median(peaks)),
            delta_t=_delta_trace(variant, trace, cuts),
        )
    return views


def measured_passes(
    passes: Sequence[tuple[PassRef, Aligned]],
    views: Mapping[int, CornerView],
    geometry: Mapping[int, Geometry],
    track_length: float,
    wanted: Set[Cell],
) -> list[Measured]:
    out = []
    for ref, p in passes:
        view = views.get(ref.corner)
        if view is None:
            continue
        window = _window(geometry[ref.corner], track_length)
        out.append(Measured(ref, measure(p, view.cuts, window, wanted)))
    return out
