"""Is an item's response a straight line?

Two checks, each on effects that were measured (docs/design.md, section E):

- one click up and one click down should be mirror images: their sum should be zero;
- two clicks should do about twice what one click does.

A response is flagged non-linear only when the departure is outside the band of practical
equivalence with the same confidence the effects themselves have: noise is not curvature.
"""

import math
from collections.abc import Mapping

from paddock_engine.sensitivity.pooling import Estimate, quantile


def _beyond(departure: float, se: float, rope: float, level: float) -> bool:
    half = quantile(level) * se
    return departure - half > rope or departure + half < -rope


def _mirror(clicks: int, by_clicks: Mapping[int, Estimate], rope: float, level: float) -> str:
    here, opposite = by_clicks[clicks], by_clicks.get(-clicks)
    if opposite is None:
        return ""
    se = math.hypot(here.se, opposite.se)
    if not _beyond(here.mean + opposite.mean, se, rope, level):
        return ""
    return f"{clicks:+d} and {-clicks:+d} clicks are not mirror images of each other."


def _doubling(clicks: int, by_clicks: Mapping[int, Estimate], rope: float, level: float) -> str:
    """Compares `clicks` with half as many (when even) and with twice as many."""
    for small, large in ((clicks // 2, clicks), (clicks, 2 * clicks)):
        if small == 0 or 2 * small != large or small not in by_clicks or large not in by_clicks:
            continue
        one, two = by_clicks[small], by_clicks[large]
        se = math.hypot(two.se, 2.0 * one.se)
        if _beyond(two.mean - 2.0 * one.mean, se, rope, level):
            return f"{large:+d} clicks do not do twice what {small:+d} does."
    return ""


def nonlinearity(clicks: int, by_clicks: Mapping[int, Estimate], rope: float, level: float) -> str:
    """Why the response at `clicks` is not linear; empty when nothing measured says so.

    `by_clicks` holds every measured effect of one item on one channel and corner class.
    """
    notes = [_mirror(clicks, by_clicks, rope, level), _doubling(clicks, by_clicks, rope, level)]
    return " ".join(note for note in notes if note)
