"""The effect per speed class: which classes are worth reporting, and whether the setup acts
differently in them (the context-dependent finding, ADR 0029).

A speed class with one or two corners is not a class but those corners, and an interval from a
class whose blocks leave fewer than 3 degrees of freedom is too wide to say anything (experiment
5's fast class came back +-330 s). Neither is stored as a result.

A context-dependent effect is an interaction of the arm with the class, measured at block level
(stats/interaction.py) for every pair of classes with at least 3 corners each, and Holm-corrected
across the pairs at CONTEXT_ALPHA. A corner's class here is the one most of its baseline passes
have: fixed by the baseline, so the setup cannot move a corner between classes.
"""

from collections import Counter
from collections.abc import Sequence

from paddock_engine.contracts.evaluate_request_schema import Arm, EvaluatePass
from paddock_engine.contracts.evaluate_response_schema import ChannelResult, CornerClass
from paddock_engine.experiments.channel_effects import observations
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.interaction import Interaction, group_interactions
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.multiplicity import holm, two_sided_p

MIN_CLASS_CORNERS = 3
MIN_CLASS_DEGREES = 3
CONTEXT_ALPHA = 0.05  # family-wise, two-sided, across the pairs of classes


def _class_of(item: EvaluatePass) -> str | None:
    return item.speed_class.value if item.speed_class is not None else None


def reportable_classes(
    results: Sequence[ChannelResult], passes: Sequence[EvaluatePass], primary: Effect
) -> list[ChannelResult]:
    """The per-class results with at least MIN_CLASS_CORNERS corners and MIN_CLASS_DEGREES block
    degrees of freedom (the primary's, less one for each block the class has no pass in)."""
    blocks = {item.block for item in passes}
    kept = []
    for result in results:
        subset = [p for p in passes if _class_of(p) == result.corner_class.value]
        missing = len(blocks - {p.block for p in subset})
        corners = len({p.corner for p in subset})
        if result.corner_class is CornerClass.all or (
            corners >= MIN_CLASS_CORNERS and primary.block_degrees - missing >= MIN_CLASS_DEGREES
        ):
            kept.append(result)
    return kept


def baseline_classes(passes: Sequence[EvaluatePass]) -> dict[int, str]:
    """Corner number -> the speed class most of its baseline passes have (ties: first by name)."""
    counts: dict[int, Counter[str]] = {}
    for item in passes:
        speed = _class_of(item)
        if item.arm is Arm.baseline and speed is not None:
            counts.setdefault(item.corner, Counter())[speed] += 1
    return {
        corner: min(tally, key=lambda name: (-tally[name], name))
        for corner, tally in counts.items()
    }


def interactions(passes: Sequence[EvaluatePass], channel: str) -> list[Interaction]:
    """Pairs of speed classes in which the arm acts differently on `channel`, after Holm."""
    classes = baseline_classes(passes)
    sizes = Counter(classes.values())
    eligible = {c: name for c, name in classes.items() if sizes[name] >= MIN_CLASS_CORNERS}
    if len(set(eligible.values())) < 2:
        return []
    try:
        pairs = group_interactions(observations(passes, channel), Arm.baseline.value, eligible)
    except (NotEstimableError, ValueError):
        return []
    pairs = [pair for pair in pairs if pair.degrees >= MIN_CLASS_DEGREES]
    p_values = [two_sided_p(p.estimate, p.standard_error, p.degrees) for p in pairs]
    return [pair for pair, hit in zip(pairs, holm(p_values, CONTEXT_ALPHA), strict=True) if hit]
