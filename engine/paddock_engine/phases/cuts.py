"""Phase cuts of one corner, frozen at fixed metres for a whole experiment (ADR 0031).

The cuts are the medians of the baseline passes' events (phases/events.py), so every pass of
both arms is cut at the same metres of track. A setup that moves the slowest point cannot move
the phase boundaries, and a pass whose apex jitters by 20 to 46 m (finding I6) is measured over
the same stretch of track as every other.

| Phase | From | To |
| --- | --- | --- |
| braking | B | TI |
| entry | TI | A - w |
| mid | A - w | A + w |
| exit | A + w | E |
| traction | E | E + 150 m (or where the passes end) |

w is half the stretch around A where the baseline's median speed stays within 3 % of its
minimum (on a flat-out corner: +-0.3 s of travel), at least 0.15 s of travel, at most 40 % of
the shorter of entry and exit. Boundaries are made monotonic in that order; a phase shorter than
5 m is merged into its neighbour and flagged. A phase is never silently empty: each of the five
has a status (`ok`, `merged`, `absent`, `unavailable`) and a note when it is not `ok`.
"""

from collections.abc import Sequence
from dataclasses import dataclass, field
from statistics import median

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.phases.events import PassEvents

PHASES = ("braking", "entry", "mid", "exit", "traction")
MIN_PHASE_M = 5.0
TRACTION_M = 150.0
MID_SPEED_BAND = 1.03
MID_MIN_HALF_S = 0.15
MID_FLAT_HALF_S = 0.3
MID_MAX_SHARE = 0.4
BRAKED_SHARE = 0.5  # of baseline passes that braked, for the corner to have a braking phase
MERGE_INTO = {"braking": "entry", "entry": "mid", "exit": "mid"}


@dataclass(frozen=True)
class PhaseSpan:
    name: str
    start_m: float
    end_m: float
    status: str = "ok"  # ok, merged, absent, unavailable
    note: str | None = None


@dataclass(frozen=True)
class CornerCuts:
    corner: int
    braking_m: float | None
    turn_in_m: float
    apex_m: float
    pickup_m: float | None
    end_m: float
    mid_half_m: float
    by_curvature: bool
    phases: list[PhaseSpan]
    flags: list[str] = field(default_factory=list)

    def span(self, name: str) -> PhaseSpan:
        return next(phase for phase in self.phases if phase.name == name)

    def measured(self) -> list[PhaseSpan]:
        """The phases that are cut (status ok), in track order."""
        return [phase for phase in self.phases if phase.status == "ok"]


def _median(values: Sequence[float | None]) -> float | None:
    present = [value for value in values if value is not None]
    return float(median(present)) if present else None


def mid_half_width(
    ref_x: FloatArray, ref_v: FloatArray, apex_m: float, by_curvature: bool, room_m: float
) -> float:
    """w around the apex, from the baseline's median speed trace (see the module docstring)."""
    index = int(np.argmin(np.abs(ref_x - apex_m)))
    v_apex = float(ref_v[index])
    wanted = MID_FLAT_HALF_S * v_apex
    if not by_curvature:
        slow = ref_v <= MID_SPEED_BAND * v_apex
        first, last = index, index
        while first > 0 and slow[first - 1]:
            first -= 1
        while last < len(slow) - 1 and slow[last + 1]:
            last += 1
        wanted = max(MID_MIN_HALF_S * v_apex, (float(ref_x[last]) - float(ref_x[first])) / 2)
    return max(MIN_PHASE_M / 2, min(wanted, MID_MAX_SHARE * max(room_m, 0.0)))


def _raw_spans(
    braking: float | None, turn_in: float, apex: float, w: float, end: float, available_m: float
) -> list[PhaseSpan]:
    """The five spans with boundaries made monotonic; braking absent without B."""
    if braking is not None:
        turn_in = max(turn_in, braking)
    mid_a = max(apex - w, turn_in)
    mid_b = max(apex + w, mid_a + 2 * w)  # a mid moved by the order keeps its width
    end = max(end, mid_b)
    traction_end = max(end, min(end + TRACTION_M, available_m))
    if braking is None:
        first = PhaseSpan("braking", turn_in, turn_in, "absent", "no braking in the baseline")
    else:
        first = PhaseSpan("braking", braking, turn_in)
    return [
        first,
        PhaseSpan("entry", turn_in, mid_a),
        PhaseSpan("mid", mid_a, mid_b),
        PhaseSpan("exit", mid_b, end),
        PhaseSpan("traction", end, traction_end),
    ]


def _merge_short(spans: list[PhaseSpan]) -> tuple[list[PhaseSpan], list[str]]:
    """A phase under MIN_PHASE_M goes into its neighbour; the neighbour takes its metres."""
    by_name = {span.name: span for span in spans}
    flags = []
    for name in ("braking", "entry", "exit"):
        span = by_name[name]
        length = span.end_m - span.start_m
        if span.status != "ok" or length >= MIN_PHASE_M:
            continue
        into = by_name[MERGE_INTO[name]]
        by_name[into.name] = PhaseSpan(
            into.name, min(into.start_m, span.start_m), max(into.end_m, span.end_m)
        )
        note = f"{length:.1f} m long: merged into {into.name}"
        by_name[name] = PhaseSpan(name, span.start_m, span.end_m, "merged", note)
        flags.append(f"{name} {note}")
    traction = by_name["traction"]
    if traction.end_m - traction.start_m < MIN_PHASE_M:
        note = "the passes end within 5 m of the corner end"
        by_name["traction"] = PhaseSpan(
            "traction", traction.start_m, traction.end_m, "unavailable", note
        )
        flags.append(f"traction {note}")
    return [by_name[name] for name in PHASES], flags


def freeze_cuts(
    corner: int,
    baseline: Sequence[PassEvents],
    ref_x: FloatArray,
    ref_v: FloatArray,
    zone_length_m: float,
    available_m: float,
) -> CornerCuts:
    """Cuts from the baseline's events. `available_m`: how far the baseline passes reach."""
    if not baseline:
        raise ValueError(f"corner {corner}: no baseline pass to cut the phases from")
    flags: list[str] = []
    braked = [e.braking_m for e in baseline if e.braking_m is not None]
    braking = _median(braked) if len(braked) >= BRAKED_SHARE * len(baseline) else None
    turn_in = float(median(e.turn_in_m for e in baseline))
    apex = float(median(e.apex_m for e in baseline))
    by_curvature = sum(e.by_curvature for e in baseline) > len(baseline) / 2
    end = _median([e.end_m for e in baseline])
    if end is None:
        end = zone_length_m
        flags.append("corner end not found in the baseline: zone end used")
    end = max(end, apex)
    room = min(apex - turn_in, end - apex)
    w = mid_half_width(ref_x, ref_v, apex, by_curvature, room)
    spans, merged = _merge_short(_raw_spans(braking, turn_in, apex, w, end, available_m))
    return CornerCuts(
        corner=corner, braking_m=braking, turn_in_m=turn_in, apex_m=apex,
        pickup_m=_median([e.pickup_m for e in baseline]), end_m=end, mid_half_m=w,
        by_curvature=by_curvature, phases=spans, flags=flags + merged,
    )  # fmt: skip
