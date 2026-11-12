"""Judge a driven experiment: the primary effect, the guardrails, whether the driver adapted,
and which predictions came true. The work behind POST /evaluate.

The decision follows from the numbers and the test fixed before driving, nothing else:

| Primary effect | Guardrails | Decision |
| --- | --- | --- |
| better, beyond the equivalence band | all hold | accepted |
| better, beyond the band | one moved or broke | tradeoff (the driver decides) |
| worse, beyond the band | any | rejected |
| inside the band (negligible) | any | rejected: nothing to gain, the baseline stays |
| unclear | one broke | rejected |
| unclear | otherwise | inconclusive: drive more or revert |

Ties go to the baseline.
"""

from collections.abc import Sequence
from dataclasses import dataclass

from paddock_engine.contracts.evaluate_request_schema import (
    Better,
    Direction,
    EvaluatePass,
    EvaluateRequest,
    GuardrailSpec,
    PredictionSpec,
    Worse,
)
from paddock_engine.contracts.evaluate_response_schema import (
    ChannelResult,
    Decision,
    EvaluateResponse,
    GuardrailResult,
    Role,
    Status,
)
from paddock_engine.experiments.channel_effects import channel_effect, per_speed_class, to_result
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.rope import EffectClass


@dataclass(frozen=True)
class _Judgement:
    decision: Decision
    reason: str


def _span(effect: Effect) -> str:
    return f"{effect.estimate:+.4g} [{effect.ci_low:+.4g}, {effect.ci_high:+.4g}]"


def _guardrail_status(effect: Effect | None, spec: GuardrailSpec) -> Status:
    if effect is None:
        return Status.unknown
    sign = 1.0 if spec.worse is Worse.higher else -1.0
    bad_estimate = sign * effect.estimate
    bad_low = sign * (effect.ci_low if sign > 0 else effect.ci_high)
    if bad_low > spec.tolerance:
        return Status.broke
    return Status.moved if bad_estimate > spec.tolerance else Status.holds


def _judge(
    primary: Effect, better: Better, channel: str, guardrails: list[GuardrailResult]
) -> _Judgement:
    improved_class = EffectClass.NEGATIVE if better is Better.lower else EffectClass.POSITIVE
    worse_class = EffectClass.POSITIVE if better is Better.lower else EffectClass.NEGATIVE
    broke = [g.channel for g in guardrails if g.status is Status.broke]
    moved = [g.channel for g in guardrails if g.status is Status.moved]
    unchecked = [g.channel for g in guardrails if g.status is Status.unknown]
    measured = f"{channel} {_span(primary)}"
    if primary.effect_class is improved_class:
        if broke or moved:
            return _Judgement(
                Decision.tradeoff,
                f"Better on {measured}, but {', '.join(broke + moved)} got worse.",
            )
        note = f" Not checked: {', '.join(unchecked)}." if unchecked else ""
        return _Judgement(Decision.accepted, f"Better on {measured}; guardrails hold.{note}")
    if primary.effect_class is worse_class:
        return _Judgement(Decision.rejected, f"Worse on {measured}.")
    if primary.effect_class is EffectClass.NEGLIGIBLE:
        return _Judgement(
            Decision.rejected, f"No meaningful effect on {measured}; the baseline stays."
        )
    if broke:
        return _Judgement(
            Decision.rejected, f"No clear gain on {measured}, and {', '.join(broke)} got worse."
        )
    return _Judgement(
        Decision.inconclusive, f"Unclear on {measured}: drive more laps or keep the baseline."
    )


def _prediction_hit(effect: Effect, direction: Direction) -> bool:
    """A prediction comes true when the interval excludes zero on the predicted side."""
    if direction is Direction.decrease:
        return effect.ci_high < 0
    return effect.ci_low > 0


def _check_guardrails(
    passes: Sequence[EvaluatePass], specs: Sequence[GuardrailSpec], level: float
) -> tuple[list[GuardrailResult], list[ChannelResult]]:
    statuses, results = [], []
    for spec in specs:
        effect = channel_effect(passes, spec.channel, spec.tolerance, level)
        statuses.append(
            GuardrailResult(channel=spec.channel, status=_guardrail_status(effect, spec))
        )
        if effect is not None:
            results.append(to_result(effect, spec.channel, Role.guardrail))
    return statuses, results


def _shifted_driver_inputs(
    passes: Sequence[EvaluatePass], channels: Sequence[str], level: float
) -> tuple[list[str], list[ChannelResult]]:
    """Driver-input channels whose interval excludes zero: the setups were driven differently."""
    shifted, results = [], []
    for channel in channels:
        effect = channel_effect(passes, channel, 0.0, level)
        if effect is None:
            continue
        results.append(to_result(effect, channel, Role.driver_input))
        if effect.effect_class in (EffectClass.POSITIVE, EffectClass.NEGATIVE):
            shifted.append(channel)
    return shifted, results


def _score_predictions(
    passes: Sequence[EvaluatePass],
    predictions: Sequence[PredictionSpec],
    level: float,
    reported: set[str],
) -> tuple[int, int, list[ChannelResult]]:
    """Hits, checkable predictions, and results for channels not reported in another role."""
    hits, total, results = 0, 0, []
    for prediction in predictions:
        effect = channel_effect(passes, prediction.channel, 0.0, level)
        if effect is None:
            continue
        total += 1
        hits += _prediction_hit(effect, prediction.direction)
        if prediction.channel not in reported:
            results.append(to_result(effect, prediction.channel, Role.prediction))
    return hits, total, results


def _not_estimable(used: int, excluded: int) -> EvaluateResponse:
    return EvaluateResponse(
        decision=Decision.inconclusive,
        reason="Not enough passes on both setups to estimate the effect.",
        results=[],
        guardrails=[],
        prediction_hits=0,
        prediction_total=0,
        driver_adapted=False,
        passes_used=used,
        passes_excluded=excluded,
    )


def evaluate(request: EvaluateRequest) -> EvaluateResponse:
    test = request.success_test
    used = [p for p in request.passes if p.metrics.get(test.channel) is not None]
    excluded = len(request.passes) - len(used)
    primary = channel_effect(used, test.channel, test.rope, test.level)
    if primary is None:
        return _not_estimable(len(used), excluded)
    results = [to_result(primary, test.channel, Role.primary)]
    results += per_speed_class(used, test.channel, test.rope, test.level, Role.primary)
    guardrails, guardrail_results = _check_guardrails(used, request.guardrails, test.level)
    driver_channels = [channel.root for channel in request.driver_channels]
    shifted, driver_results = _shifted_driver_inputs(used, driver_channels, test.level)
    results += guardrail_results + driver_results
    hits, total, prediction_results = _score_predictions(
        used, request.predictions, test.level, {result.channel for result in results}
    )
    judgement = _judge(primary, test.better, test.channel, guardrails)
    reason = judgement.reason
    if shifted:
        reason += f" Driver inputs shifted ({', '.join(shifted)}): part of this may be the driver."
    return EvaluateResponse(
        decision=judgement.decision,
        reason=reason,
        results=results + prediction_results,
        guardrails=guardrails,
        prediction_hits=hits,
        prediction_total=total,
        driver_adapted=bool(shifted),
        passes_used=len(used),
        passes_excluded=excluded,
    )
