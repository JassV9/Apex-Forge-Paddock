"""The effect per speed class: which classes are worth reporting, and whether the setup acts
differently between speed regimes or between corner phases (the context-dependent finding, ADR
0029, ADR 0034).

A speed class with one or two corners is not a class but those corners, and an interval from a
class whose blocks leave fewer than 3 degrees of freedom is too wide to say anything (experiment
5's fast class came back +-330 s). Neither is stored as a result.

A context-dependent effect is an interaction of the arm with a group of corners, measured at
block level (stats/interaction.py), in two families tested together:

- speed regimes: every pair of regimes with at least 3 corners each. A corner's regime is its
  own, frozen from the baseline (features/regimes.py, ADR 0031): the setup never moves a corner
  between regimes;
- phases (when the primary channel is corner time and the raw samples are there): every pair of
  frozen phases (braking, entry, mid, exit, traction; ADR 0031) present in at least 3 corners,
  on the phase times, which sum to the corner time. A change that loses in braking what it gains
  on exit is context-dependent.

Holm runs across all the pairs of both families at CONTEXT_ALPHA.
"""

from collections import Counter
from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from paddock_engine.contracts.evaluate_request_schema import Arm, EvaluatePass
from paddock_engine.contracts.evaluate_response_schema import ChannelResult, CornerClass
from paddock_engine.experiments.channel_effects import observations
from paddock_engine.experiments.vehicle import PHASED, PHASES, Vehicle
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.interaction import Interaction, group_interactions
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.multiplicity import holm, two_sided_p
from paddock_engine.stats.observations import Observation

MIN_CLASS_CORNERS = 3
MIN_CLASS_DEGREES = 3
CONTEXT_ALPHA = 0.05  # family-wise, two-sided, across the pairs of regimes and of phases
PHASE_CODES = 10  # a (corner, phase) group is coded corner * PHASE_CODES + the phase's index


@dataclass(frozen=True)
class Context:
    regimes: list[Interaction]  # pairs of speed regimes that survived Holm
    phases: list[Interaction]  # pairs of phases that survived Holm

    @property
    def found(self) -> bool:
        return bool(self.regimes or self.phases)


NO_CONTEXT = Context(regimes=[], phases=[])


def reportable_classes(
    results: Sequence[ChannelResult],
    passes: Sequence[EvaluatePass],
    primary: Effect,
    classes: Mapping[int, str],
) -> list[ChannelResult]:
    """The per-class results with at least MIN_CLASS_CORNERS corners and MIN_CLASS_DEGREES block
    degrees of freedom (the primary's, less one for each block the class has no pass in)."""
    blocks = {item.block for item in passes}
    kept = []
    for result in results:
        subset = [p for p in passes if classes.get(p.corner) == result.corner_class.value]
        missing = len(blocks - {p.block for p in subset})
        corners = len({p.corner for p in subset})
        if result.corner_class is CornerClass.all or (
            corners >= MIN_CLASS_CORNERS and primary.block_degrees - missing >= MIN_CLASS_DEGREES
        ):
            kept.append(result)
    return kept


def _pairs(rows: Sequence[Observation], groups: Mapping[int, str]) -> list[Interaction]:
    """Interactions between groups with at least MIN_CLASS_CORNERS members each."""
    sizes = Counter(groups.values())
    eligible = {code: name for code, name in groups.items() if sizes[name] >= MIN_CLASS_CORNERS}
    if len(set(eligible.values())) < 2:
        return []
    try:
        pairs = group_interactions(rows, Arm.baseline.value, eligible)
    except (NotEstimableError, ValueError):
        return []
    return [pair for pair in pairs if pair.degrees >= MIN_CLASS_DEGREES]


def phase_rows(data: Vehicle, metric: str) -> tuple[list[Observation], dict[int, str]]:
    """One observation per pass and phase with a value; the (corner, phase) code's phase."""
    rows, groups = [], {}
    for item in data.measured:
        for index, phase in enumerate(PHASES):
            value = item.values.get((metric, phase))
            if value is None:
                continue
            code = item.ref.corner * PHASE_CODES + index
            groups[code] = phase
            rows.append(
                Observation(
                    corner=code, arm=item.ref.arm, value=value, block=item.ref.block,
                    lap=item.ref.lap, covariates=item.ref.covariates,
                )
            )  # fmt: skip
    return rows, groups


def context(
    passes: Sequence[EvaluatePass],
    channel: str,
    classes: Mapping[int, str],
    data: Vehicle | None,
) -> Context:
    """Pairs of regimes and of phases in which the arm acts differently, after Holm."""
    regimes = _pairs(observations(passes, channel), classes)
    phases: list[Interaction] = []
    if data is not None and channel in PHASED:
        phases = _pairs(*phase_rows(data, PHASED[channel]))
    pairs = regimes + phases
    p_values = [two_sided_p(p.estimate, p.standard_error, p.degrees) for p in pairs]
    hits = holm(p_values, CONTEXT_ALPHA)
    return Context(
        regimes=[pair for pair, hit in zip(regimes, hits, strict=False) if hit],
        phases=[pair for pair, hit in zip(phases, hits[len(regimes) :], strict=True) if hit],
    )
