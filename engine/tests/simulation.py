"""Simulated A-B-A-B experiments for the statistics tests.

Each corner has its own level (3 to 11 s), the variant arm adds `effect` seconds per corner
pass, and every pass gets Gaussian noise. 28 ms is the per-corner scatter measured on the real
FA26 COTA session (docs/audit-v0.md), the value the phase 2 acceptance criteria name.
"""

import numpy as np

from paddock_engine.stats.observations import Observation

SIGMA_S = 0.028
BASELINE, VARIANT = "A", "B"


def simulate_abab(
    rng: np.random.Generator,
    effect: float,
    corners: int,
    laps_per_block: int = 3,
    sigma: float = SIGMA_S,
    lap_sigma: float = 0.0,
    drift_per_lap: float = 0.0,
) -> list[Observation]:
    """Four blocks A, B, A, B. With 3 laps per block each arm has 6 passes per corner.

    `lap_sigma` is noise shared by all passes of a lap; `drift_per_lap` is a steady change over
    the session (track rubbering in, the driver learning) that has nothing to do with the setup.
    """
    levels = rng.uniform(3.0, 11.0, corners)
    observations = []
    lap = 0
    for block, arm in enumerate([BASELINE, VARIANT, BASELINE, VARIANT]):
        for _ in range(laps_per_block):
            lap += 1
            lap_error = rng.normal(0.0, lap_sigma) if lap_sigma > 0 else 0.0
            for corner in range(corners):
                value = (
                    levels[corner]
                    + (effect if arm == VARIANT else 0.0)
                    + drift_per_lap * lap
                    + lap_error
                    + rng.normal(0.0, sigma)
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
