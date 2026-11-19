"""The car's response inside the verdict (ADR 0034): the experiment's passes on the 1 m grid, cut
at the metres frozen from the baseline (ADR 0031), measured only on the cells the verdict needs.

The cells are the ones the changed parameter is expected to move (response/parameters.toml) and,
when the primary channel is corner time, the phase times of every phase (the arm x phase test of
experiments/context.py). Everything else stays with POST /response, which describes every cell.

The speed regimes come from the same baseline passes: the median of their slowest metre per
corner. The verdict has one regime map, used for the time's speed classes, the context test and
the response cells alike.
"""

from collections.abc import Set
from dataclasses import dataclass

import duckdb

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest, VehicleResponseInput
from paddock_engine.features.regimes import Regimes
from paddock_engine.io.samples import read_session_samples
from paddock_engine.response.config import Observables, ParameterRow, observables
from paddock_engine.response.passes import (
    Cell,
    Geometry,
    Measured,
    PassRef,
    aligned_passes,
    corner_views,
    measured_passes,
)
from paddock_engine.response.service import Loader, Samples, regimes_of, samples_of

PHASES = ("braking", "entry", "mid", "exit", "traction")
PHASED = {"corner_time_s": "phase_time_s"}  # a primary channel and the metric it splits into


@dataclass(frozen=True)
class ExpectedCell:
    metric: str
    phase: str
    regime: str
    expected: str  # direct or proxy


@dataclass(frozen=True)
class Vehicle:
    measured: list[Measured]
    regimes: Regimes
    row: ParameterRow | None
    expected: list[ExpectedCell]


def verdict_samples(
    request: EvaluateRequest, load: Loader = read_session_samples
) -> Samples | None:
    """The raw samples the request points at; None without a response context or when they
    cannot be read (the verdict is then given without response findings, and says so)."""
    if request.response is None:
        return None
    sessions = ((s.session, s.parquet_dir) for s in request.response.sessions)
    try:
        return samples_of(sessions, load)
    except (OSError, ValueError, duckdb.Error):
        return None


def expected_cells(row: ParameterRow | None, config: Observables) -> list[ExpectedCell]:
    """Every (metric, phase, regime) the parameter row expects to move, direct before proxy."""
    if row is None:
        return []
    out: dict[tuple[str, str, str], ExpectedCell] = {}
    for metric in (*row.direct, *row.proxy):
        for phase in config.metrics[metric].phases:
            for regime in ("all", *row.regimes):
                kind = row.observes(metric, phase, regime)
                if kind is not None and (metric, phase, regime) not in out:
                    out[(metric, phase, regime)] = ExpectedCell(metric, phase, regime, kind)
    return list(out.values())


def _refs(context: VehicleResponseInput) -> list[PassRef]:
    return [
        PassRef(
            session=p.session, seq=p.seq, corner=p.corner, arm=p.arm.value, block=p.block,
            t_start_s=p.t_start_s, duration_s=p.duration_s, valid=p.valid, lap=p.lap,
            covariates=p.covariates or {},
        )
        for p in context.passes
    ]  # fmt: skip


def _wanted(cells: list[ExpectedCell], channel: str) -> Set[Cell]:
    wanted = {(cell.metric, cell.phase) for cell in cells}
    if channel in PHASED:
        wanted |= {(PHASED[channel], phase) for phase in PHASES}
    return wanted


def vehicle(
    context: VehicleResponseInput,
    samples: Samples,
    channel: str,
    config: Observables | None = None,
) -> Vehicle | None:
    """The measured passes and the regimes; None when no baseline pass has samples."""
    config = config or observables()
    row = config.row_for(context.item_role or "", context.item_axle or "")
    cells = expected_cells(row, config)
    length = context.track_length_m
    geometry = {
        c.number: Geometry(c.number, c.zone_start_m, c.start_m, c.end_m, c.zone_end_m)
        for c in context.corners
    }
    passes = aligned_passes(_refs(context), samples, geometry, length)
    views = corner_views(passes, geometry, length)
    if not views:
        return None
    measured = measured_passes(passes, views, geometry, length, _wanted(cells, channel))
    regimes, _ = regimes_of(views)
    return Vehicle(measured=measured, regimes=regimes, row=row, expected=cells)
