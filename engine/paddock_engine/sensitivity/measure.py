"""An experiment re-derived for the sensitivity profile: POST /sensitivity/measure (ADR 0033).

The ledger keeps each verdict's intervals, not the standard errors and degrees of freedom behind
them, and verdicts judged before ADR 0029 were computed by another method (finding I2). Pooling
needs both, from one method. Core therefore sends an experiment's evaluate request, built from
the same passes its verdict judged; this judges it again with the current engine for its finding
(an invalid one is never pooled) and measures every pooled channel (the primary and each
guardrail) over all corners and per frozen speed class, with the block-level standard error and
its degrees of freedom. Nothing is written to the ledger: core keeps the result in
`sensitivity_derivation`, once per experiment and engine version.

A speed class needs at least 3 corners and 3 block degrees of freedom, as a stored verdict's
class does (finding I4).
"""

from paddock_engine import __version__
from paddock_engine.config.margins import margins
from paddock_engine.contracts.evaluate_request_schema import EvaluatePass, EvaluateRequest
from paddock_engine.contracts.sensitivity_measure_schema import (
    CornerClass,
    FindingClass,
    MeasuredEffect,
    SensitivityMeasure,
)
from paddock_engine.experiments.channel_effects import (
    MIN_CORNERS_PER_CLASS,
    channel_effect,
    experiment_regimes,
)
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.stats.effects import Effect

LEVEL = 0.90  # the interval is not used: only the estimate, its error and degrees of freedom
MIN_CLASS_DEGREES = 3


def _effect(channel: str, corner_class: CornerClass, effect: Effect) -> MeasuredEffect:
    return MeasuredEffect(
        channel=channel,
        corner_class=corner_class,
        estimate=effect.estimate,
        standard_error=effect.standard_error,
        df=effect.block_degrees,
        n_passes=effect.n_passes,
        blocks=effect.blocks,
        corners=effect.corners,
    )


def _classes(passes: list[EvaluatePass]) -> dict[CornerClass, list[EvaluatePass]]:
    regimes = experiment_regimes(passes).classes
    subsets = {CornerClass.all: passes}
    for corner_class in (CornerClass.slow, CornerClass.medium, CornerClass.fast):
        members = {corner for corner, name in regimes.items() if name == corner_class.value}
        if len(members) >= MIN_CORNERS_PER_CLASS:
            subsets[corner_class] = [p for p in passes if p.corner in members]
    return subsets


def _channel(request: EvaluateRequest, channel: str, frozen: float) -> list[MeasuredEffect]:
    rope = margins().margin(channel, request.parameter, frozen) or 0.0
    out = []
    for corner_class, subset in _classes(request.passes).items():
        effect = channel_effect(subset, channel, rope, LEVEL)
        if effect is None:
            continue
        if corner_class is not CornerClass.all and effect.block_degrees < MIN_CLASS_DEGREES:
            continue
        out.append(_effect(channel, corner_class, effect))
    return out


def measure(request: EvaluateRequest) -> SensitivityMeasure:
    verdict = evaluate(request)
    finding = FindingClass(verdict.finding_class.value) if verdict.finding_class else None
    channels = [(request.success_test.channel, request.success_test.rope)] + [
        (g.channel, g.tolerance) for g in request.guardrails
    ]
    effects = (
        []
        if finding is FindingClass.invalid
        else [
            effect for channel, frozen in channels for effect in _channel(request, channel, frozen)
        ]
    )
    return SensitivityMeasure(
        engine_version=__version__,
        finding_class=finding or FindingClass.insufficient_evidence,
        reason=verdict.reason if finding is FindingClass.invalid else "",
        effects=effects,
    )
