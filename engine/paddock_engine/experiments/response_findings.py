"""What the car did, beside what the clock said (ADR 0034).

Each cell the changed parameter is expected to move (experiments/vehicle.py) is estimated by the
effect model, `stats.effects.estimate_effects`, with its block-level interval (ADR 0029) at the
success test's level, and judged against its margin (config/margins.toml: the item's own, else
the metric's). Holm runs across the expected cells at RESPONSE_ALPHA, family-wise and two-sided:
a cell has moved when Holm rejects "no change". In this order:

| Finding | Rule |
| --- | --- |
| increase / decrease | moved, the whole interval beyond the margin, at least 8 blocks |
| neutral | the whole interval inside the margin (two one-sided tests at the level) |
| likely increase / decrease | moved, not by the margin (or fewer than 8 blocks) |
| insufficient | otherwise, or not estimable, or no corner in the regime |

Only a moved cell ever claims a direction, so on identical setups the chance of any false
directional finding over the family stays at RESPONSE_ALPHA. Cells not expected to move are not
judged here: they are descriptive (POST /response) and never drive a finding.
"""

from collections.abc import Sequence
from dataclasses import replace

from paddock_engine.config.margins import margins
from paddock_engine.contracts.evaluate_response_schema import ResponseExpected as Expected
from paddock_engine.contracts.evaluate_response_schema import ResponseFinding
from paddock_engine.contracts.evaluate_response_schema import ResponseFindingClass as Finding
from paddock_engine.contracts.evaluate_response_schema import ResponsePhase as Phase
from paddock_engine.contracts.evaluate_response_schema import ResponseRegime as Regime
from paddock_engine.experiments.vehicle import ExpectedCell, Vehicle
from paddock_engine.response.config import MetricSpec, Observables, observables
from paddock_engine.response.effects import CellEffect, cell_effect, regime_corners
from paddock_engine.stats.multiplicity import holm
from paddock_engine.stats.sequential import FIRST_LOOK_BLOCKS

RESPONSE_ALPHA = 0.05
MOVED = {Finding.increase, Finding.decrease, Finding.likely_increase, Finding.likely_decrease}


def _spec(config: Observables, metric: str, parameter: str | None) -> MetricSpec:
    spec = config.metrics[metric]
    return replace(spec, margin=margins().margin(metric, parameter, 0.0) or spec.margin)


def judge(cell: CellEffect, moved: bool, margin: float) -> Finding:
    if cell.estimate is None or cell.ci_low is None or cell.ci_high is None:
        return Finding.insufficient
    confirmable = cell.blocks >= FIRST_LOOK_BLOCKS
    if moved and confirmable and cell.ci_low > margin:
        return Finding.increase
    if moved and confirmable and cell.ci_high < -margin:
        return Finding.decrease
    if -margin < cell.ci_low and cell.ci_high < margin:
        return Finding.neutral
    if moved:
        return Finding.likely_increase if cell.estimate > 0 else Finding.likely_decrease
    return Finding.insufficient


def _note(cell: CellEffect, moved: bool, family: int) -> str | None:
    if cell.note is not None:
        return cell.note
    low, high = cell.ci_low or 0.0, cell.ci_high or 0.0
    if not moved and (low > 0 or high < 0):
        return f"excludes 0, but not after Holm across {family} expected cells"
    return None


def _out(
    cell: CellEffect, wanted: ExpectedCell, finding: Finding, margin: float, note: str | None
) -> ResponseFinding:
    return ResponseFinding(
        metric=wanted.metric, phase=Phase(wanted.phase), regime=Regime(wanted.regime),
        expected=Expected(wanted.expected), finding=finding, estimate=cell.estimate,
        ci_low=cell.ci_low, ci_high=cell.ci_high, p_value=cell.p_value, margin=margin,
        n_passes=cell.n_passes, corners=cell.corners, note=note,
    )  # fmt: skip


def response_findings(
    data: Vehicle, parameter: str | None, level: float, config: Observables | None = None
) -> list[ResponseFinding]:
    """One finding per expected cell, Holm across those that could be estimated."""
    config = config or observables()
    groups = regime_corners(data.regimes.classes)
    specs = [_spec(config, cell.metric, parameter) for cell in data.expected]
    cells = [
        cell_effect(data.measured, spec, (c.metric, c.phase), c.regime, groups[c.regime], level)
        for c, spec in zip(data.expected, specs, strict=True)
    ]
    tested = [i for i, cell in enumerate(cells) if cell.p_value is not None]
    hits = holm([cells[i].p_value or 1.0 for i in tested], RESPONSE_ALPHA)
    moved = {i for i, hit in zip(tested, hits, strict=True) if hit}
    return [
        _out(
            cell, wanted, judge(cell, i in moved, spec.margin), spec.margin,
            _note(cell, i in moved, len(tested)),
        )
        for i, (cell, wanted, spec) in enumerate(zip(cells, data.expected, specs, strict=True))
    ]  # fmt: skip


def any_moved(findings: Sequence[ResponseFinding] | None) -> bool:
    return any(f.finding in MOVED for f in findings or [])
