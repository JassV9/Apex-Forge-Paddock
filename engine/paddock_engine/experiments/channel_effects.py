"""The variant's effect on single channels of an experiment's passes."""

from collections.abc import Sequence

from paddock_engine.contracts.evaluate_request_schema import Arm, EvaluatePass
from paddock_engine.contracts.evaluate_response_schema import (
    ChannelResult,
    CornerClass,
    Role,
)
from paddock_engine.contracts.evaluate_response_schema import EffectClass as EffectClassOut
from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.observations import Observation

BASELINE = Arm.baseline.value


def observations(passes: Sequence[EvaluatePass], channel: str) -> list[Observation]:
    """One observation per pass that has a value for the channel."""
    out = []
    for item in passes:
        value = item.metrics.get(channel)
        if value is None:
            continue
        out.append(
            Observation(
                corner=item.corner,
                arm=item.arm.value,
                value=value,
                block=item.block,
                lap=item.lap,
                covariates=item.covariates,
            )
        )
    return out


def channel_effect(
    passes: Sequence[EvaluatePass], channel: str, rope: float, level: float
) -> Effect | None:
    """Variant minus baseline on one channel; None when the passes cannot support an estimate."""
    rows = observations(passes, channel)
    if {row.arm for row in rows} != {Arm.baseline.value, Arm.variant.value}:
        return None
    try:
        return estimate_effects(rows, BASELINE, rope=rope, level=level)[0]
    except (NotEstimableError, ValueError):
        return None


def to_result(
    effect: Effect, channel: str, role: Role, corner_class: CornerClass = CornerClass.all
) -> ChannelResult:
    return ChannelResult(
        channel=channel,
        role=role,
        corner_class=corner_class,
        estimate=effect.estimate,
        ci_low=effect.ci_low,
        ci_high=effect.ci_high,
        n_passes=effect.n_passes,
        effect_class=EffectClassOut(effect.effect_class.value),
    )


def per_speed_class(
    passes: Sequence[EvaluatePass], channel: str, rope: float, level: float, role: Role
) -> list[ChannelResult]:
    """The same effect restricted to slow, medium and fast corners, where estimable."""
    results = []
    for corner_class in (CornerClass.slow, CornerClass.medium, CornerClass.fast):
        subset = [
            item
            for item in passes
            if item.speed_class is not None and item.speed_class.value == corner_class.value
        ]
        effect = channel_effect(subset, channel, rope, level)
        if effect is not None:
            results.append(to_result(effect, channel, role, corner_class))
    return results
