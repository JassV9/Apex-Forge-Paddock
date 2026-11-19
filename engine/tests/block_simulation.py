"""Simulated AI experiments with errors shared by whole run blocks (ADR 0029).

The pass scatter of each corner is that of the 14 real Formula Hybrid 2026 corners of experiment
17 (5.5 to 147 ms); a lap shares 4 ms over its corners. A block shock is shared by every pass of
a block and explained by no covariate (a refuel, a restart, the AI planning a little
differently). Fuel is refilled every block (a sawtooth) and time runs from the experiment's
first pass, as core sends them. Options add a game restart and effects by speed class.
"""

from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

import numpy as np

from paddock_engine.stats.observations import Observation

SIGMA_S = (
    np.array([5.5, 6.3, 15.3, 21.1, 21.9, 25.6, 28.3, 28.6, 29.3, 29.4, 44.6, 51.9, 66.1, 146.9])
    / 1000
)
CORNERS = len(SIGMA_S)
# Speed classes of the 14 corners: 5 slow, 5 medium, 4 fast, mixed over the scatter.
CLASSES = ["slow", "medium", "fast", "slow", "medium"] * 2 + ["fast", "slow", "medium", "fast"]
BASELINE, VARIANT = "baseline", "variant"


@dataclass(frozen=True)
class Design:
    effect: float = 0.0  # variant minus baseline on every corner, seconds
    blocks: int = 8
    laps: int = 3
    lap_sd: float = 0.004
    block_sd: float = 0.0
    class_effects: Mapping[str, float] = field(default_factory=dict)  # added to `effect`
    restart_before: int | None = None  # block (1-based) a new game session starts with
    restart_step: float = 0.0  # every pass of the new session is this much slower
    session_term: bool = True  # send a session_2 covariate after a restart
    noise_scale: float = 1.0  # of the pass scatter: 0.1 is a car as steady as the Ferrari


def _covariates(
    design: Design, elapsed: float, session_start: float, lap: int, corner: int, second: bool
) -> dict[str, float]:
    covariates = {"fuel_l": 30.0 - 2.9 * lap - 0.2 * corner}
    if design.session_term:
        covariates["time_since_start_s"] = elapsed
        if design.restart_before is not None:
            covariates["session_2"] = float(second)
    else:
        covariates["time_in_session_s"] = elapsed - session_start
    return covariates


def simulate(rng: np.random.Generator, design: Design) -> list[Observation]:
    levels = rng.uniform(3.0, 11.0, CORNERS)
    observations, elapsed, session_start = [], 0.0, 0.0
    for block in range(1, design.blocks + 1):
        arm = VARIANT if block % 2 == 0 else BASELINE
        second = design.restart_before is not None and block >= design.restart_before
        if design.restart_before == block:
            session_start = elapsed
        shock = rng.normal(0.0, design.block_sd) if design.block_sd > 0 else 0.0
        shock += design.restart_step if second else 0.0
        elapsed += 120.0  # the out-lap
        for lap in range(design.laps):
            shared = rng.normal(0.0, design.lap_sd)
            for corner in range(CORNERS):
                effect = design.effect + design.class_effects.get(CLASSES[corner], 0.0)
                value = (
                    levels[corner]
                    + shock
                    + shared
                    + rng.normal(0.0, design.noise_scale * SIGMA_S[corner])
                )
                value += effect if arm == VARIANT else 0.0
                covariates = _covariates(
                    design, elapsed + 6.0 * corner, session_start, lap, corner, second
                )
                observations.append(
                    Observation(
                        corner + 1, arm, float(value), block, block * 1000 + lap + 1, covariates
                    )
                )
            elapsed += 90.0
    return observations


def as_passes(
    observations: list[Observation], channel: str = "corner_time_s"
) -> list[dict[str, Any]]:
    """EvaluatePass dicts of simulated observations, with the corners' speed classes."""
    return [
        {
            "corner": o.corner,
            "arm": o.arm,
            "block": o.block,
            "lap": o.lap,
            "speed_class": CLASSES[o.corner - 1],
            "metrics": {channel: o.value},
            "covariates": dict(o.covariates),
        }
        for o in observations
    ]
