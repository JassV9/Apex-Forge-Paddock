"""Synthetic experiments for the verdict with the car's response in it (ADR 0034).

`sampled_experiment` drives the synthetic corners of response_data.py in A-B-A-B blocks and
returns both what core sends: the evaluate passes (corner time and minimum speed per pass, from
the samples) and the response context (where the samples are), with the samples themselves.

`measured_experiment` skips the samples: one value per pass and (metric, phase) cell, drawn with
corner levels, block shocks and pass scatter, for the many repetitions an error rate needs.
"""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from typing import Any

import numpy as np
from response_data import ZONE_M, Shape, sampled

from paddock_engine.features.regimes import Regimes
from paddock_engine.io.samples import PassSamples
from paddock_engine.response.config import ParameterRow
from paddock_engine.response.passes import Cell, Measured, PassRef

APEX_SPEEDS = (15.0, 16.0, 17.0, 28.0, 30.0, 32.0, 45.0, 48.0, 51.0)  # m/s: 3 per regime
TRACK = ZONE_M * len(APEX_SPEEDS)
TEST = {"channel": "corner_time_s", "rope": 0.005, "level": 0.9, "better": "lower"}


def _pass(
    seq: int, block: int, lap: int, corner: int, clock: float, one: PassSamples
) -> dict[str, Any]:
    return {"session": 1, "seq": seq, "corner": corner, "arm": "baseline" if block % 2 else
            "variant", "block": block, "lap": block * 10 + lap, "t_start_s": clock,
            "duration_s": float(one.t[-1]) + 0.018, "valid": True,
            "covariates": {"time_since_start_s": clock}}  # fmt: skip


def _evaluate_pass(context_pass: dict[str, Any], one: PassSamples) -> dict[str, Any]:
    keep = ("corner", "arm", "block", "lap", "covariates")
    metrics = {"corner_time_s": float(one.t[-1]), "v_min_kmh": float(one.v.min() * 3.6)}
    return {key: context_pass[key] for key in keep} | {"metrics": metrics}


def sampled_experiment(
    variant: Mapping[str, float], blocks: int = 8, laps: int = 3, seed: int = 5
) -> tuple[dict[str, Any], dict[tuple[int, int], PassSamples]]:
    """An evaluate request with a response context, and the samples it points at."""
    context, passes, samples, seq, clock = [], [], {}, 0, 0.0
    for block in range(1, blocks + 1):
        for lap in range(laps):
            for index, v_apex in enumerate(APEX_SPEEDS):
                seq += 1
                rng = np.random.default_rng((seed, seq))
                extra = {"rear_slip_extra_deg": float(rng.normal(0, 0.05))}
                if block % 2 == 0:
                    extra = {k: extra.get(k, 0.0) + v for k, v in variant.items()}
                noise = (0.0, TRACK, float(rng.normal(1.0, 0.004)))
                shape = Shape(v_apex=v_apex, slow_stretch=noise, **extra)
                one = sampled(shape, rng, zone_start_m=index * ZONE_M, track_length=TRACK)
                samples[(1, seq)] = one
                context.append(_pass(seq, block, lap, index + 1, clock, one))
                passes.append(_evaluate_pass(context[-1], one))
                clock += context[-1]["duration_s"]
    corners = [
        {"number": i + 1, "zone_start_m": i * ZONE_M, "start_m": i * ZONE_M + 180,
         "end_m": i * ZONE_M + 300, "zone_end_m": ((i + 1) * ZONE_M) % TRACK}
        for i in range(len(APEX_SPEEDS))
    ]  # fmt: skip
    response = {"track_length_m": TRACK, "item_role": "arb", "item_axle": "R",
                "corners": corners, "sessions": [{"session": 1, "parquet_dir": "unused"}],
                "passes": context}  # fmt: skip
    request = {"success_test": TEST, "guardrails": [], "predictions": [], "driver_channels": [],
               "passes": passes, "look": 1, "extensions_left": 2, "parameter": "ARB_R",
               "response": response}  # fmt: skip
    return request, samples


@dataclass(frozen=True)
class Truth:
    """Variant minus baseline per cell; cells not named do not move."""

    effects: Mapping[Cell, float] = field(default_factory=dict)
    blocks: int = 8
    laps: int = 3
    corners: int = 9
    block_sd: float = 0.3  # in units of the cell's pass scatter
    lap_sd: float = 0.3


def regimes(corners: int) -> Regimes:
    names = ("slow", "medium", "fast")
    classes = {c: names[(c - 1) * 3 // corners] for c in range(1, corners + 1)}
    return Regimes(classes=classes, medians_kmh={}, breaks_kmh=(100.0, 150.0), method="fixed")


def measured_experiment(
    rng: np.random.Generator, cells: Sequence[Cell], truth: Truth, scale: Mapping[str, float]
) -> list[Measured]:
    """Passes with one value per cell: a corner level, a block shock and a lap shock per cell,
    pass scatter of 1, all times the metric's `scale`; the truth added on the variant."""
    levels = {cell: rng.normal(0.0, 3.0, truth.corners + 1) for cell in cells}
    out, seq = [], 0
    for block in range(1, truth.blocks + 1):
        arm = "baseline" if block % 2 else "variant"
        shocks = {cell: rng.normal(0.0, truth.block_sd) for cell in cells}
        for lap in range(truth.laps):
            laps = {cell: rng.normal(0.0, truth.lap_sd) for cell in cells}
            for corner in range(1, truth.corners + 1):
                seq += 1
                values: dict[Cell, float | None] = {}
                for cell in cells:
                    noise = levels[cell][corner] + shocks[cell] + laps[cell] + rng.normal(0, 1)
                    effect = truth.effects.get(cell, 0.0) if arm == "variant" else 0.0
                    values[cell] = float(noise) * scale[cell[0]] + effect
                ref = PassRef(1, seq, corner, arm, block, float(seq), 1.0, lap=block * 10 + lap)
                out.append(Measured(ref, values))
    return out


def row(direct: Sequence[str], proxy: Sequence[str], phases: Sequence[str]) -> ParameterRow:
    return ParameterRow(
        key="test_row", match=(("arb", "R"),), direct=tuple(direct), proxy=tuple(proxy),
        phases=tuple(phases), regimes=("slow", "medium"), unavailable=(),
    )  # fmt: skip
