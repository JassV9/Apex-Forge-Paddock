"""Per (metric, phase, speed regime) effect of the variant, through the existing effect model.

The inference is `stats.effects.estimate_effects` (ADR 0008, 0023), called as it is: nothing
here re-implements it. Each cell is one call on the passes that have a value, the corners of
one regime (or all). The finding is read from the 90 % interval against the metric's margin
(response/metrics.toml):

| Interval | Finding |
| --- | --- |
| wholly above +margin | increase |
| wholly below -margin | decrease |
| wholly inside [-margin, +margin] | neutral |
| excludes 0, not the margin | likely increase / likely decrease |
| anything else, or not estimable | insufficient |

These are descriptive per cell. There is no correction for the number of cells, so at 90 %
about one cell in ten excludes zero by chance; a finding reads as "this moved", and the
decision on a setup stays with the experiment's pre-registered test.
"""

from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass

from paddock_engine.features.regimes import CLASSES
from paddock_engine.response.config import MetricSpec
from paddock_engine.response.passes import BASELINE, Cell, Measured
from paddock_engine.stats.effects import estimate_effects
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.multiplicity import two_sided_p
from paddock_engine.stats.observations import Observation

MIN_CORNERS_PER_REGIME = 3  # as for speed classes in the verdict (finding I4)
REGIMES = ("all", *CLASSES)


@dataclass(frozen=True)
class CellEffect:
    metric: str
    phase: str
    regime: str
    kind: str  # direct or proxy, from the configuration
    finding: str
    estimate: float | None
    ci_low: float | None
    ci_high: float | None
    baseline_mean: float | None
    n_passes: int
    corners: int
    note: str | None = None
    p_value: float | None = None  # two-sided, block-level t against 0 (for Holm, ADR 0034)
    blocks: int = 0


def finding(ci_low: float, ci_high: float, margin: float) -> str:
    if ci_low > margin:
        return "increase"
    if ci_high < -margin:
        return "decrease"
    if ci_low >= -margin and ci_high <= margin:
        return "neutral"
    if ci_low > 0:
        return "likely increase"
    return "likely decrease" if ci_high < 0 else "insufficient"


def _rows(passes: Iterable[Measured], cell: Cell, corners: set[int]) -> list[Observation]:
    out = []
    for item in passes:
        value = item.values.get(cell)
        if value is None or item.ref.corner not in corners:
            continue
        out.append(
            Observation(
                corner=item.ref.corner, arm=item.ref.arm, value=value, block=item.ref.block,
                lap=item.ref.lap, covariates=item.ref.covariates,
            )
        )  # fmt: skip
    return out


def _insufficient(
    spec: MetricSpec, cell: Cell, regime: str, rows: Sequence[Observation], note: str
) -> CellEffect:
    base = [row.value for row in rows if row.arm == BASELINE]
    return CellEffect(
        metric=cell[0], phase=cell[1], regime=regime, kind=spec.kind, finding="insufficient",
        estimate=None, ci_low=None, ci_high=None,
        baseline_mean=sum(base) / len(base) if base else None, n_passes=len(rows),
        corners=len({row.corner for row in rows}), note=note,
    )  # fmt: skip


def cell_effect(
    passes: Sequence[Measured],
    spec: MetricSpec,
    cell: Cell,
    regime: str,
    corners: set[int],
    level: float,
) -> CellEffect:
    rows = _rows(passes, cell, corners)
    used = {row.corner for row in rows}
    if {row.arm for row in rows} != {BASELINE, "variant"}:
        return _insufficient(spec, cell, regime, rows, "not measured on both setups")
    if regime != "all" and len(used) < MIN_CORNERS_PER_REGIME:
        return _insufficient(spec, cell, regime, rows, f"{len(used)} corners: too few for a regime")
    try:
        effect = estimate_effects(rows, BASELINE, rope=spec.margin, level=level)[0]
    except (NotEstimableError, ValueError) as error:
        return _insufficient(spec, cell, regime, rows, f"not estimable: {error}")
    base = [row.value for row in rows if row.arm == BASELINE]
    return CellEffect(
        metric=cell[0], phase=cell[1], regime=regime, kind=spec.kind,
        finding=finding(effect.ci_low, effect.ci_high, spec.margin), estimate=effect.estimate,
        ci_low=effect.ci_low, ci_high=effect.ci_high, baseline_mean=sum(base) / len(base),
        n_passes=effect.n_passes, corners=len(used),
        p_value=two_sided_p(effect.estimate, effect.standard_error, effect.block_degrees),
        blocks=effect.blocks,
    )  # fmt: skip


def regime_corners(classes: Mapping[int, str]) -> dict[str, set[int]]:
    groups = {regime: {c for c, name in classes.items() if name == regime} for regime in CLASSES}
    return {"all": set(classes), **groups}


def all_effects(
    passes: Sequence[Measured],
    specs: Mapping[str, MetricSpec],
    classes: Mapping[int, str],
    level: float,
) -> list[CellEffect]:
    """Every configured (metric, phase) over every regime that has corners."""
    groups = regime_corners(classes)
    measured_cells = {cell for item in passes for cell in item.values}
    out = []
    for name, spec in specs.items():
        for phase in spec.phases:
            if (name, phase) not in measured_cells:
                continue
            for regime in REGIMES:
                if groups[regime]:
                    out.append(
                        cell_effect(passes, spec, (name, phase), regime, groups[regime], level)
                    )
    return out
