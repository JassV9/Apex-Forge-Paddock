"""The channels judged beside the primary one: guardrails, driver inputs and predictions.

Every interval here is block-level (ADR 0029) at the success test's nominal level; only the
primary channel is judged at the sequential look's level. Several tests at once are
Holm-corrected at 1 - level: the driver inputs (does the driver drive the setups differently?)
and the response channels (did anything but the primary move?). A driver input has shifted only
when, beyond that, its whole interval lies outside its margin (config/margins.toml).
"""

from collections.abc import Sequence
from dataclasses import dataclass

from paddock_engine.config.margins import margins
from paddock_engine.contracts.evaluate_request_schema import (
    Direction,
    EvaluatePass,
    EvaluateRequest,
    GuardrailSpec,
    Worse,
)
from paddock_engine.contracts.evaluate_response_schema import (
    ChannelResult,
    CornerClass,
    GuardrailResult,
    Role,
    Status,
)
from paddock_engine.contracts.evaluate_response_schema import EffectClass as EffectClassOut
from paddock_engine.experiments.channel_effects import channel_effect, to_result
from paddock_engine.experiments.offtrack import OFFTRACK, offtrack_effect
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.multiplicity import holm, two_sided_p
from paddock_engine.stats.rope import classify


@dataclass(frozen=True)
class Secondary:
    guardrails: list[GuardrailResult]
    results: list[ChannelResult]
    shifted: list[str]  # driver inputs that moved, after Holm
    moved: list[str]  # guardrail and prediction channels that moved, after Holm
    prediction_hits: int
    prediction_total: int


def guardrail_status(estimate: float, low: float, high: float, spec: GuardrailSpec) -> Status:
    sign = 1.0 if spec.worse is Worse.higher else -1.0
    bad_low = sign * (low if sign > 0 else high)
    if bad_low > spec.tolerance:
        return Status.broke
    return Status.moved if sign * estimate > spec.tolerance else Status.holds


def _p(effect: Effect) -> float:
    return two_sided_p(effect.estimate, effect.standard_error, effect.block_degrees)


def _specs(request: EvaluateRequest) -> list[GuardrailSpec]:
    """The experiment's guardrails with their configured margins, and the off-track share when
    core counted it and the experiment has no guardrail of its own on it."""
    specs = list(request.guardrails)
    if request.offtrack is not None and all(s.channel != OFFTRACK for s in specs):
        specs.append(GuardrailSpec(channel=OFFTRACK, tolerance=0.0, worse=Worse.higher))
    resolved = []
    for spec in specs:
        tolerance = margins().margin(spec.channel, request.parameter, spec.tolerance)
        resolved.append(spec.model_copy(update={"tolerance": tolerance or 0.0}))
    return resolved


def _offtrack(request: EvaluateRequest, spec: GuardrailSpec) -> tuple[Status, ChannelResult | None]:
    found = offtrack_effect(request.offtrack or [], request.success_test.level)
    if found is None:
        return Status.unknown, None
    estimate, low, high, passes = found
    result = ChannelResult(
        channel=OFFTRACK,
        role=Role.guardrail,
        corner_class=CornerClass.all,
        estimate=estimate,
        ci_low=low,
        ci_high=high,
        n_passes=passes,
        effect_class=EffectClassOut(classify(low, high, spec.tolerance).value),
    )
    return guardrail_status(estimate, low, high, spec), result


def _guardrails(
    request: EvaluateRequest, passes: Sequence[EvaluatePass]
) -> tuple[list[GuardrailResult], list[ChannelResult], list[tuple[str, Effect]]]:
    statuses, results, effects = [], [], []
    for spec in _specs(request):
        if spec.channel == OFFTRACK:
            status, result = _offtrack(request, spec)
        else:
            effect = channel_effect(
                passes, spec.channel, spec.tolerance, request.success_test.level
            )
            status = Status.unknown
            result = None
            if effect is not None and spec.tolerance > 0:
                status = guardrail_status(effect.estimate, effect.ci_low, effect.ci_high, spec)
            if effect is not None:
                result = to_result(effect, spec.channel, Role.guardrail)
                effects.append((spec.channel, effect))
        statuses.append(GuardrailResult(channel=spec.channel, status=status))
        results += [result] if result is not None else []
    return statuses, results, effects


def _measured(
    passes: Sequence[EvaluatePass],
    channels: Sequence[str],
    level: float,
    bands: dict[str, float] | None = None,
) -> list[tuple[str, Effect]]:
    bands = bands or {}
    found = (
        (channel, channel_effect(passes, channel, bands.get(channel, 0.0), level))
        for channel in channels
    )
    return [(channel, effect) for channel, effect in found if effect is not None]


def _holm_hits(measured: Sequence[tuple[str, Effect]], alpha: float) -> list[str]:
    hits = holm([_p(effect) for _, effect in measured], alpha)
    return [channel for (channel, _), hit in zip(measured, hits, strict=True) if hit]


def _driver_margins(request: EvaluateRequest) -> dict[str, float]:
    """Each driver channel's margin; 0 (judged against zero alone) where none is configured."""
    channels = (c.root for c in request.driver_channels)
    return {c: margins().margin(c, request.parameter, 0.0) or 0.0 for c in channels}


def _shifted(
    drivers: Sequence[tuple[str, Effect]], bands: dict[str, float], alpha: float
) -> list[str]:
    """Driver inputs that moved after Holm and by more than their margin: the whole interval
    beyond it, as for a response cell's increase or decrease (ADR 0034, amendment)."""
    effects = dict(drivers)
    return [
        channel
        for channel in _holm_hits(drivers, alpha)
        if effects[channel].ci_low > bands[channel] or effects[channel].ci_high < -bands[channel]
    ]


def _hit(effect: Effect, direction: Direction) -> bool:
    """A prediction comes true when the interval excludes zero on the predicted side."""
    return effect.ci_high < 0 if direction is Direction.decrease else effect.ci_low > 0


def secondary(request: EvaluateRequest, passes: Sequence[EvaluatePass]) -> Secondary:
    level = request.success_test.level
    statuses, results, guarded = _guardrails(request, passes)
    bands = _driver_margins(request)
    drivers = _measured(passes, list(bands), level, bands)
    results += [to_result(effect, channel, Role.driver_input) for channel, effect in drivers]
    reported = {request.success_test.channel} | {r.channel for r in results}
    predicted = _measured(passes, [p.channel for p in request.predictions], level)
    direction = {p.channel: p.direction for p in request.predictions}
    hits = sum(_hit(effect, direction[channel]) for channel, effect in predicted)
    results += [
        to_result(effect, channel, Role.prediction)
        for channel, effect in predicted
        if channel not in reported
    ]
    responses = dict(guarded) | {c: e for c, e in predicted if c != request.success_test.channel}
    return Secondary(
        guardrails=statuses,
        results=results,
        shifted=_shifted(drivers, bands, 1.0 - level),
        moved=_holm_hits(sorted(responses.items()), 1.0 - level),
        prediction_hits=hits,
        prediction_total=len(predicted),
    )
