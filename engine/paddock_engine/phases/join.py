"""Join a pass with the next pass of the same lap, when their zones touch (ADR 0031).

The agent records a pass from its zone start to its zone end, and on a tight track the zones
are contiguous: one corner's zone ends where the next one's starts. A corner's exit and the
150 m of traction after it then lie in the next pass. Joined, every corner has the samples its
exit and traction phases need, chicanes included.
"""

from dataclasses import fields

import numpy as np

from paddock_engine.io.samples import PassSamples

CONTIGUOUS_WITHIN_M = 2.0  # the next zone starts within this of the zone end
SAME_LAP_WITHIN_S = 1.0  # and its first sample follows within this of the last one


def zones_touch(zone_end_m: float, next_zone_start_m: float, track_length: float) -> bool:
    gap = (next_zone_start_m - zone_end_m) % track_length
    return min(gap, track_length - gap) <= CONTIGUOUS_WITHIN_M


def follows(this_t_start_s: float, this_duration_s: float, next_t_start_s: float) -> bool:
    """The next pass started where this one ended: no pit stop, restart or skipped corner."""
    return abs(next_t_start_s - (this_t_start_s + this_duration_s)) <= SAME_LAP_WITHIN_S


def joined(this: PassSamples, after: PassSamples, offset_s: float) -> PassSamples:
    """This pass's samples, then the next pass's, its time shifted by `offset_s`.

    `offset_s` is when the next pass started, in seconds after this one started.
    """
    columns = {}
    for item in fields(PassSamples):
        own, other = getattr(this, item.name), getattr(after, item.name)
        if item.name == "t":
            other = other + offset_s
        columns[item.name] = np.concatenate([own, other])
    return PassSamples(**columns)
