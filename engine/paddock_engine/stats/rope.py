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


class Finding(StrEnum):
    """What an interval of the gain says against a margin (ADR 0029).

    The gain is the effect turned so that positive is better. The invalid and context-dependent
    findings are not read off one interval (experiments/finding.py).
    """

    CONFIRMED_IMPROVEMENT = "confirmed_improvement"  # whole interval above the margin
    CONFIRMED_REGRESSION = "confirmed_regression"  # whole interval below minus the margin
    NEUTRAL = "neutral"  # whole interval inside the margin: two one-sided tests
    LIKELY_IMPROVEMENT = "likely_improvement"  # whole interval above zero
    LIKELY_REGRESSION = "likely_regression"  # whole interval below zero
    INSUFFICIENT_EVIDENCE = "insufficient_evidence"  # the interval straddles zero


def interval_finding(
    gain_low: float, gain_high: float, margin: float, confirmable: bool
) -> Finding:
    """The finding of a gain interval against a positive margin.

    `confirmable` is False before the first pre-registered look (fewer than 8 blocks): an
    interval beyond the margin is then only likely.
    """
    if margin <= 0 or gain_low > gain_high:
        raise ValueError(f"invalid interval [{gain_low}, {gain_high}] or margin {margin}")
    if confirmable and gain_low > margin:
        return Finding.CONFIRMED_IMPROVEMENT
    if confirmable and gain_high < -margin:
        return Finding.CONFIRMED_REGRESSION
    if gain_low > -margin and gain_high < margin:
        return Finding.NEUTRAL
    if gain_low > 0:
        return Finding.LIKELY_IMPROVEMENT
    if gain_high < 0:
        return Finding.LIKELY_REGRESSION
    return Finding.INSUFFICIENT_EVIDENCE
