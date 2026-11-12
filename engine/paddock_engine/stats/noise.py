"""Noise model: how much a channel scatters from pass to pass, per corner.

Measured, not assumed: v0 used fixed constants. The scatter is taken within each (corner, arm)
cell around the cell's median and scaled from the median absolute deviation, so a few slow
passes (traffic, a mistake, a cool-down lap) do not inflate it.
"""

from collections.abc import Sequence
from dataclasses import dataclass

import numpy as np

from paddock_engine.stats.observations import Observation

MAD_TO_SIGMA = 1.4826  # MAD of a normal distribution is sigma / 1.4826
MIN_PASSES_PER_CELL = 3


@dataclass(frozen=True)
class NoiseModel:
    per_corner: dict[int, float]  # sigma per corner, in the channel's unit
    typical: float  # median over corners: the sigma to plan with

    def quietest(self, count: int) -> list[int]:
        """The `count` corners with the least scatter: the best ones to test on."""
        return sorted(self.per_corner, key=lambda corner: self.per_corner[corner])[:count]


def noise_model(observations: Sequence[Observation]) -> NoiseModel:
    """Raises ValueError when no corner has enough passes on one setup."""
    cells: dict[tuple[int, str], list[float]] = {}
    for observation in observations:
        cells.setdefault((observation.corner, observation.arm), []).append(observation.value)
    deviations: dict[int, list[float]] = {}
    for (corner, _arm), values in cells.items():
        if len(values) < MIN_PASSES_PER_CELL:
            continue
        array = np.asarray(values)
        deviations.setdefault(corner, []).extend(np.abs(array - np.median(array)).tolist())
    if not deviations:
        raise ValueError(f"no corner has {MIN_PASSES_PER_CELL} passes on one setup")
    per_corner = {
        corner: MAD_TO_SIGMA * float(np.median(values)) for corner, values in deviations.items()
    }
    return NoiseModel(per_corner=per_corner, typical=float(np.median(list(per_corner.values()))))
