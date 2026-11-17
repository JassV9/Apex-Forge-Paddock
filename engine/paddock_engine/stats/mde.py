"""Minimum detectable effect: what a run plan can and cannot answer, known before driving.

With pass-to-pass scatter sigma, n passes per arm in each of k corners, the smallest average
effect per corner detectable with 80 % power at a two-sided 5 % level is about

    MDE = 2.8 * sigma * sqrt(2 / (n * k))

(2.8 = 1.96 + 0.84). With the 28 ms measured on the real FA26 COTA session that is 14 ms per
corner at n = 6, k = 10, and 26 ms at v0's n = 3, k = 6.

That formula assumes every pass errs on its own. A lap also has an error shared by all its
corners (stats/lap_noise.py, ADR 0025), which averages out over laps only. With one pass per
corner and lap, `lap_inclusive_sigma` is the pass scatter that gives the same precision, so the
formulas here hold with it in place of sigma.
"""

import math

POWER_FACTOR = 2.8  # z(0.975) + z(0.80)


def lap_inclusive_sigma(sigma: float, sigma_lap: float, corners: int) -> float:
    """Pass scatter equivalent to `sigma` per pass plus `sigma_lap` shared by a lap's corners.

    An arm's mean over L laps of k corners has variance (pass^2 / k + sigma_lap^2) / L, which is
    sigma_eff^2 / (k L) with sigma_eff^2 = pass^2 + k sigma_lap^2. `sigma` is measured around
    each corner's level over all laps (stats/noise.py), so it already holds the lap's variance
    once: pass^2 = sigma^2 - sigma_lap^2. Without lap noise the result is sigma.
    """
    if sigma < 0 or sigma_lap < 0 or corners < 1:
        raise ValueError("sigma and sigma_lap must be >= 0 and corners >= 1")
    pass_part = max(sigma**2 - sigma_lap**2, 0.0)
    return math.sqrt(pass_part + corners * sigma_lap**2)


def minimum_detectable_effect(sigma: float, passes_per_arm: int, corners: int) -> float:
    """In the unit of sigma, per corner pass."""
    if sigma < 0 or passes_per_arm < 1 or corners < 1:
        raise ValueError("sigma must be >= 0 and passes_per_arm, corners >= 1")
    return POWER_FACTOR * sigma * math.sqrt(2.0 / (passes_per_arm * corners))


def passes_needed(sigma: float, effect: float, corners: int) -> int:
    """Passes per arm in each corner needed to detect `effect` (same unit as sigma)."""
    if effect <= 0 or corners < 1 or sigma < 0:
        raise ValueError("effect must be > 0, corners >= 1 and sigma >= 0")
    needed = max(1, math.ceil(2.0 * (POWER_FACTOR * sigma / effect) ** 2 / corners))
    # rounding can put the exact budget a hair above a whole number: one pass fewer may do
    if needed > 1 and minimum_detectable_effect(sigma, needed - 1, corners) <= effect:
        needed -= 1
    return needed
