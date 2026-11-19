"""The observables configuration: metrics and unavailable channels (response/metrics.toml) and
the parameter-to-observable mapping (response/parameters.toml). A metric's margin is not here:
every margin is in config/margins.toml (ADR 0034). Read once; checked against the code by the
tests."""

import tomllib
from collections.abc import Mapping
from dataclasses import dataclass
from functools import cache
from importlib import resources
from typing import Any

from paddock_engine.config.margins import margins

PHASE_NAMES = ("braking", "entry", "mid", "exit", "traction", "corner")
KINDS = ("direct", "proxy")


@dataclass(frozen=True)
class MetricSpec:
    name: str
    kind: str
    unit: str
    margin: float
    phases: tuple[str, ...]
    about: str


@dataclass(frozen=True)
class ParameterRow:
    key: str
    match: tuple[tuple[str, str], ...]  # (role, axle)
    direct: tuple[str, ...]
    proxy: tuple[str, ...]
    phases: tuple[str, ...]
    regimes: tuple[str, ...]
    unavailable: tuple[str, ...]

    def observes(self, metric: str, phase: str, regime: str) -> str | None:
        """'direct' or 'proxy' when this row expects the cell to move; None otherwise."""
        if phase not in self.phases or regime not in (*self.regimes, "all"):
            return None
        if metric in self.direct:
            return "direct"
        return "proxy" if metric in self.proxy else None


@dataclass(frozen=True)
class Observables:
    metrics: dict[str, MetricSpec]
    unavailable: dict[str, str]
    parameters: tuple[ParameterRow, ...]

    def cells(self) -> frozenset[tuple[str, str]]:
        """Every configured (metric, phase)."""
        return frozenset((m.name, phase) for m in self.metrics.values() for phase in m.phases)

    def row_for(self, role: str, axle: str) -> ParameterRow | None:
        wanted = (role.strip().lower(), axle.strip().upper())
        return next((row for row in self.parameters if wanted in row.match), None)


def _metric(name: str, raw: dict[str, Any], margin: float | None) -> MetricSpec:
    phases = tuple(raw["phases"])
    if raw["kind"] not in KINDS or not set(phases) <= set(PHASE_NAMES):
        raise ValueError(f"observables: metric {name} has an unknown kind or phase")
    if margin is None:
        raise ValueError(f"observables: metric {name} has no margin in config/margins.toml")
    return MetricSpec(name, raw["kind"], raw["unit"], margin, phases, raw["about"])


def _row(raw: dict[str, Any]) -> ParameterRow:
    return ParameterRow(
        key=raw["key"],
        match=tuple((m["role"], m["axle"].upper()) for m in raw["match"]),
        direct=tuple(raw["direct"]),
        proxy=tuple(raw["proxy"]),
        phases=tuple(raw["phases"]),
        regimes=tuple(raw["regimes"]),
        unavailable=tuple(raw.get("unavailable", ())),
    )


def parse(raw: dict[str, Any], metric_margins: Mapping[str, float]) -> Observables:
    """The configuration, each metric with its default margin from `metric_margins`."""
    metrics = {
        name: _metric(name, spec, metric_margins.get(name)) for name, spec in raw["metrics"].items()
    }
    unavailable = {name: str(reason) for name, reason in raw["unavailable"].items()}
    rows = tuple(_row(row) for row in raw["parameters"])
    for row in rows:
        unknown = [m for m in (*row.direct, *row.proxy) if m not in metrics]
        unknown += [m for m in row.unavailable if m not in unavailable]
        if unknown:
            raise ValueError(f"observables: parameter {row.key} names unknown {unknown}")
    return Observables(metrics=metrics, unavailable=unavailable, parameters=rows)


@cache
def observables() -> Observables:
    folder = resources.files("paddock_engine.response")
    raw: dict[str, Any] = {}
    for name in ("metrics.toml", "parameters.toml"):
        raw |= tomllib.loads(folder.joinpath(name).read_text("utf-8"))
    return parse(raw, margins().metrics)
