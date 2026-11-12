"""Minimum detectable effect: what a run plan can and cannot answer, known before driving.

With pass-to-pass scatter sigma, n passes per arm in each of k corners, the smallest average
effect per corner detectable with 80 % power at a two-sided 5 % level is about

    MDE = 2.8 * sigma * sqrt(2 / (n * k))

(2.8 = 1.96 + 0.84). With the 28 ms measured on the real FA26 COTA session that is 14 ms per
corner at n = 6, k = 10, and 26 ms at v0's n = 3, k = 6.
"""

import math

POWER_FACTOR = 2.8  # z(0.975) + z(0.80)


def minimum_detectable_effect(sigma: float, passes_per_arm: int, corners: int) -> float:
    """In the unit of sigma, per corner pass."""
    if sigma < 0 or passes_per_arm < 1 or corners < 1:
        raise ValueError("sigma must be >= 0 and passes_per_arm, corners >= 1")
    return POWER_FACTOR * sigma * math.sqrt(2.0 / (passes_per_arm * corners))


def passes_needed(sigma: float, effect: float, corners: int) -> int:
    """Passes per arm in each corner needed to detect `effect` (same unit as sigma)."""
    if effect <= 0 or corners < 1 or sigma < 0:
        raise ValueError("effect must be > 0, corners >= 1 and sigma >= 0")
    return max(1, math.ceil(2.0 * (POWER_FACTOR * sigma / effect) ** 2 / corners))
