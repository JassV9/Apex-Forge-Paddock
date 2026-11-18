"""The variant's effect on single channels of an experiment's passes.

Speed classes belong to corners, frozen from the baseline arm (features/regimes.py, ADR 0031):
a pass's own minimum speed is an outcome of the setup and never picks its class.
"""

from collections.abc import Sequence

from paddock_engine.contracts.evaluate_request_schema import Arm, EvaluatePass
from paddock_engine.contracts.evaluate_response_schema import (
    ChannelResult,
    CornerClass,
    Role,
)
from paddock_engine.contracts.evaluate_response_schema import EffectClass as EffectClassOut
from paddock_engine.features.regimes import Regimes, corner_regimes
from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.observations import Observation

BASELINE = Arm.baseline.value
MIN_CORNERS_PER_CLASS = 3  # fewer corners than this is not a class effect (finding I4)


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


def experiment_regimes(passes: Sequence[EvaluatePass]) -> Regimes:
    """One class per corner from the baseline passes' minimum speed (their stored class only
    where no pass of the corner has one)."""
    return corner_regimes(
        (
            item.corner,
            item.metrics.get("v_min_kmh"),
            item.speed_class.value if item.speed_class is not None else None,
        )
        for item in passes
        if item.arm is Arm.baseline
    )


def per_speed_class(
    passes: Sequence[EvaluatePass], channel: str, rope: float, level: float, role: Role
) -> list[ChannelResult]:
    """The same effect restricted to slow, medium and fast corners, where estimable.

    A class needs MIN_CORNERS_PER_CLASS corners; with fewer it is left out, never estimated.
    """
    classes = experiment_regimes(passes).classes
    results = []
    for corner_class in (CornerClass.slow, CornerClass.medium, CornerClass.fast):
        members = {corner for corner, name in classes.items() if name == corner_class.value}
        if len(members) < MIN_CORNERS_PER_CLASS:
            continue
        subset = [item for item in passes if item.corner in members]
        effect = channel_effect(subset, channel, rope, level)
        if effect is not None:
            results.append(to_result(effect, channel, role, corner_class))
    return results
