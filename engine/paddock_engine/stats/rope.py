"""Classify an effect against a region of practical equivalence (ROPE).

The ROPE is the band of effects too small to care about, set per channel (for corner time:
5 ms either way). An interval is judged against the band, not against zero, so "no effect" is
a finding the system can reach, and a tiny real effect is not mistaken for a useful one.
"""

from enum import StrEnum


class EffectClass(StrEnum):
    POSITIVE = "positive"  # interval entirely above the ROPE
    NEGATIVE = "negative"  # interval entirely below the ROPE
    NEGLIGIBLE = "negligible"  # interval entirely inside: the car ignores this change
    INCONCLUSIVE = "inconclusive"  # interval straddles a ROPE edge: drive more, do not act


def classify(ci_low: float, ci_high: float, rope: float) -> EffectClass:
    """`rope` is the half-width of the band around zero, in the channel's unit."""
    if rope < 0 or ci_low > ci_high:
        raise ValueError(f"invalid interval [{ci_low}, {ci_high}] or rope {rope}")
    if ci_low > rope:
        return EffectClass.POSITIVE
    if ci_high < -rope:
        return EffectClass.NEGATIVE
    if ci_low >= -rope and ci_high <= rope:
        return EffectClass.NEGLIGIBLE
    return EffectClass.INCONCLUSIVE
