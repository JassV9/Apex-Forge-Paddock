"""The verdict in words, and what besides the setup may be in its number."""

from collections.abc import Sequence

from paddock_engine.contracts.evaluate_response_schema import (
    Confounder,
    Decision,
    FindingClass,
    GuardrailResult,
    Kind,
    Status,
)
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.interaction import Interaction
from paddock_engine.stats.sequential import LOOKS

NAMED_BELOW_WEIGHT = 0.5  # corners that counted less than half are named in the reason
NAMED_CORNERS = 3  # at most, the least-weighted first

_FINDING_WORDS = {
    FindingClass.confirmed_improvement: "Better on",
    FindingClass.confirmed_regression: "Worse on",
    FindingClass.neutral: "No meaningful effect on",
    FindingClass.context_dependent: "Acts differently by speed class on",
    FindingClass.likely_improvement: "Probably better on",
    FindingClass.likely_regression: "Probably worse on",
    FindingClass.insufficient_evidence: "Unclear on",
}


def span(effect: Effect) -> str:
    return f"{effect.estimate:+.4g} [{effect.ci_low:+.4g}, {effect.ci_high:+.4g}]"


def _where(effect: Effect, look: int) -> str:
    at = f"look {look} of {LOOKS}" if look else "before the first look"
    return f"{effect.blocks} blocks, {effect.block_degrees} degrees of freedom, {at}"


def _guardrail_words(decision: Decision, guardrails: Sequence[GuardrailResult]) -> str:
    worse = [g.channel for g in guardrails if g.status in (Status.moved, Status.broke)]
    unchecked = [g.channel for g in guardrails if g.status is Status.unknown]
    if decision is Decision.tradeoff:
        return f" But {', '.join(worse)} got worse."
    if decision is Decision.accepted:
        note = f" Not checked: {', '.join(unchecked)}." if unchecked else ""
        return " Guardrails hold." + note
    broke = [g.channel for g in guardrails if g.status is Status.broke]
    if decision is Decision.rejected and broke:
        return f" And {', '.join(broke)} got worse."
    return ""


def finding_reason(
    finding: FindingClass,
    decision: Decision,
    channel: str,
    primary: Effect,
    look: int,
    guardrails: Sequence[GuardrailResult],
) -> str:
    head = f"{_FINDING_WORDS[finding]} {channel} {span(primary)} ({_where(primary, look)})."
    if finding is FindingClass.neutral:
        head += " The baseline stays."
    return head + _guardrail_words(decision, guardrails)


def _weighting_note(primary: Effect) -> str:
    """Which corners counted least (ADR 0023); nothing when every corner counted fully."""
    weights = primary.corner_weights
    if all(weight == 1.0 for weight in weights.values()):
        return ""
    light = sorted(
        (corner for corner, weight in weights.items() if weight < NAMED_BELOW_WEIGHT),
        key=lambda corner: (weights[corner], corner),
    )
    least = (
        f"least weight: {', '.join(str(corner) for corner in light[:NAMED_CORNERS])}"
        if light
        else "none counted below half"
    )
    return f" Corners weighted by their own scatter (ADR 0023); {least}."


def notes(primary: Effect, shifted: Sequence[str], few_blocks: bool) -> str:
    """What the driver should know about how the number was reached, appended to the reason."""
    text = ""
    if primary.dropped_covariates:
        text += (
            " Not adjusted for "
            f"{', '.join(primary.dropped_covariates)}: too entangled with which setup was on the "
            "car (see ADR 0022)."
        )
    text += _weighting_note(primary)
    if shifted:
        text += f" Driver inputs shifted ({', '.join(shifted)}): part of this may be the driver."
    if few_blocks:
        text += " Fewer than 8 blocks: never confirmed automatically (ADR 0029)."
    return text


def confounders(
    primary: Effect | None,
    shifted: Sequence[str],
    interactions: Sequence[Interaction],
    excluded: tuple[int, int],
) -> list[Confounder]:
    items = []
    if primary is not None:
        items += [
            Confounder(kind=Kind.covariate_left_out, name=c) for c in primary.dropped_covariates
        ]
        items += [
            Confounder(kind=Kind.covariate_between_blocks, name=c) for c in primary.block_varying
        ]
    items += [Confounder(kind=Kind.driver_input_shifted, name=c) for c in shifted]
    items += [
        Confounder(kind=Kind.speed_class_interaction, name=f"{i.first} vs {i.second}")
        for i in interactions
    ]
    for kind, count in zip((Kind.setup_mismatch, Kind.source_mismatch), excluded, strict=True):
        if count:
            items.append(Confounder(kind=kind, name=kind.value.replace("_", " "), count=count))
    return items
