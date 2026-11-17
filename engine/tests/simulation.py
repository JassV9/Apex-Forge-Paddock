"""Simulated A-B-A-B experiments for the statistics tests.

Each corner has its own level (3 to 11 s), the variant arm adds `effect` seconds per corner
pass, and every pass gets Gaussian noise. 28 ms is the per-corner scatter measured on the real
FA26 COTA session (docs/audit-v0.md), the value the phase 2 acceptance criteria name.
"""

from collections.abc import Sequence

import numpy as np

from paddock_engine.stats.observations import Observation

SIGMA_S = 0.028
BASELINE, VARIANT = "A", "B"


def simulate_abab(
    rng: np.random.Generator,
    effect: float,
    corners: int,
    laps_per_block: int = 3,
    sigma: float | Sequence[float] = SIGMA_S,
    lap_sigma: float = 0.0,
    drift_per_lap: float = 0.0,
    blocks: int = 4,
) -> list[Observation]:
    """Blocks A, B, A, B (more pairs with `blocks`). With 3 laps per block and four blocks each
    arm has 6 passes per corner.

    `sigma` is one scatter for every corner or one per corner; `lap_sigma` is noise shared by
    all passes of a lap; `drift_per_lap` is a steady change over the session (track rubbering
    in, the driver learning) that has nothing to do with the setup.
    """
    sigmas = np.broadcast_to(np.asarray(sigma, dtype=np.float64), (corners,))
    levels = rng.uniform(3.0, 11.0, corners)
    observations = []
    lap = 0
    for block, arm in enumerate([BASELINE, VARIANT] * (blocks // 2)):
        for _ in range(laps_per_block):
            lap += 1
            lap_error = rng.normal(0.0, lap_sigma) if lap_sigma > 0 else 0.0
            for corner in range(corners):
                value = (
                    levels[corner]
                    + (effect if arm == VARIANT else 0.0)
                    + drift_per_lap * lap
                    + lap_error
                    + rng.normal(0.0, sigmas[corner])
                )
                observations.append(
                    Observation(
                        corner=corner + 1,
                        arm=arm,
                        value=float(value),
                        block=block,
                        lap=lap,
                        covariates={"time_in_session": float(lap)},
                    )
                )
    return observations


# (start time in the session s, fuel at the start l) of each block of a restarted experiment:
# experiment 6 on the Ferrari 488 GT3 was driven over three game sessions, and a new session
# resets the clock and refuels, so the variant blocks sat early in a session on a heavy tank.
RESTARTED_BLOCKS = [(131.0, 39.0), (132.0, 47.0), (680.0, 27.0), (133.0, 47.0)]
SECONDS_PER_LAP, LITRES_PER_LAP = 136.0, 2.95


def simulate_restarted_abab(
    rng: np.random.Generator, effect: float, corners: int = 10, sigma: float = 0.012
) -> list[Observation]:
    """A-B-A-B in which time in session and fuel are correlated but not copies of each other."""
    levels = rng.uniform(3.0, 11.0, corners)
    observations = []
    for block, (arm, (start, fuel)) in enumerate(
        zip([BASELINE, VARIANT, BASELINE, VARIANT], RESTARTED_BLOCKS, strict=True)
    ):
        for lap in range(3):
            for corner in range(corners):
                elapsed = start + lap * SECONDS_PER_LAP + 8.0 * corner
                litres = fuel - lap * LITRES_PER_LAP - 0.17 * corner
                value = levels[corner] + (effect if arm == VARIANT else 0.0)
                observations.append(
                    Observation(
                        corner=corner + 1,
                        arm=arm,
                        value=float(value + rng.normal(0.0, sigma)),
                        block=block,
                        lap=block * 10 + lap,
                        covariates={"time_in_session_s": elapsed, "fuel_l": litres},
                    )
                )
    return observations
