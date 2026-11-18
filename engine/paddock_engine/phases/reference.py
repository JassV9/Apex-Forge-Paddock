"""The per-corner reference: the pooled baseline passes on the 1 m grid (ADR 0031).

The reference is what the phase cuts are frozen from (phases/cuts.py) and what a pass's
delta-t trace is drawn against. It is built from the baseline arm only, so the variant cannot
move it, and from every baseline block pooled, so one block cannot either.
"""

from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.phases.align import Aligned
from paddock_engine.phases.cuts import CornerCuts, freeze_cuts
from paddock_engine.phases.events import CornerWindow, PassEvents, pass_events


@dataclass(frozen=True)
class Trace:
    """Median over passes at each metre; time counted from the first metre all passes share."""

    x: FloatArray
    t: FloatArray
    v: FloatArray
    reach_m: float  # median of how far the passes go


def _on(x: FloatArray, p: Aligned, values: FloatArray) -> FloatArray:
    out: FloatArray = np.where((x >= p.x[0]) & (x <= p.x[-1]), np.interp(x, p.x, values), np.nan)
    return out


def median_trace(passes: Sequence[Aligned]) -> Trace:
    if not passes:
        raise ValueError("a trace needs at least one pass")
    start = max(float(p.x[0]) for p in passes)
    stop = max(float(p.x[-1]) for p in passes)
    x = np.arange(start, stop + 0.5, 1.0)
    times = np.vstack([_on(x, p, p.t) - p.time_at(start) for p in passes])
    speeds = np.vstack([_on(x, p, p.v) for p in passes])
    return Trace(
        x=x,
        t=np.nanmedian(times, axis=0),
        v=np.nanmedian(speeds, axis=0),
        reach_m=float(np.median([p.x[-1] for p in passes])),
    )


def corner_reference(
    corner: int, baseline: Sequence[Aligned], window: CornerWindow, zone_length_m: float
) -> tuple[CornerCuts, Trace, list[PassEvents]]:
    """The frozen cuts, the baseline's median trace and each baseline pass's events."""
    events = [pass_events(p, window) for p in baseline]
    trace = median_trace(baseline)
    cuts = freeze_cuts(corner, events, trace.x, trace.v, zone_length_m, trace.reach_m)
    return cuts, trace, events
