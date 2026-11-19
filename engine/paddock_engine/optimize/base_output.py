"""The base setup's answer: the changes, what was measured, and the shrunk prediction."""

import math
from collections.abc import Sequence
from dataclasses import dataclass

from paddock_engine.contracts.base_setup_request_schema import (
    BaseSetupItem,
    BaseSetupRequest,
    SensitivityEffect,
)
from paddock_engine.contracts.base_setup_schema import (
    BaseSetup,
    BaseSetupChange,
    BaseSetupLeftOut,
    BaseSetupMeasured,
    BaseSetupPrediction,
    KnowledgeLevel,
)
from paddock_engine.sensitivity.pooling import quantile


@dataclass(frozen=True)
class Candidate:
    row: SensitivityEffect
    p_value: float  # one-sided, of the change being no better
    discovered: bool  # passed Benjamini-Hochberg
    shrunk: float  # empirical Bayes posterior mean of the effect


def target_value(item: BaseSetupItem, clicks: int) -> float:
    return round(item.value + clicks * item.step, 9)


def _change(candidate: Candidate, item: BaseSetupItem) -> BaseSetupChange:
    row = candidate.row
    return BaseSetupChange(
        parameter=row.parameter,
        label=item.label,
        clicks=row.clicks,
        from_value=item.value,
        to_value=target_value(item, row.clicks),
        estimate=row.estimate,
        ci_low=row.ci_low,
        ci_high=row.ci_high,
        n_passes=row.n_passes,
        effect_class=row.effect_class.value,
        experiment_ids=row.experiment_ids,
        shrunk_estimate=candidate.shrunk,
        knowledge_level=KnowledgeLevel(row.knowledge_level.value),
        p_value=min(max(candidate.p_value, 0.0), 1.0),
        track=row.track,
        hint_bucket=row.hint_bucket,
    )


def predict(chosen: Sequence[Candidate], channel: str, level: float) -> BaseSetupPrediction:
    """The sum of the shrunk estimates; the interval from the summed measured variances."""
    estimate = sum(c.shrunk for c in chosen)
    half = quantile(level) * math.sqrt(sum(c.row.standard_error**2 for c in chosen))
    return BaseSetupPrediction(
        channel=channel,
        estimate=estimate,
        raw_estimate=sum(c.row.estimate for c in chosen),
        ci_low=estimate - half,
        ci_high=estimate + half,
        level=level,
        note=(
            f"Sum of {len(chosen)} effects measured one item at a time, each shrunk towards zero "
            "(empirical Bayes across this car's measured changes). How the items interact was not "
            "measured, so the combination is judged as its own experiment."
        ),
    )


def _measured(rows: Sequence[SensitivityEffect], labels: dict[str, str]) -> list[BaseSetupMeasured]:
    return [
        BaseSetupMeasured(
            parameter=row.parameter,
            label=labels.get(row.parameter, row.parameter),
            clicks=row.clicks,
            estimate=row.estimate,
            ci_low=row.ci_low,
            ci_high=row.ci_high,
            n_passes=row.n_passes,
            effect_class=row.effect_class.value,
            knowledge_level=KnowledgeLevel(row.knowledge_level.value),
            experiments=row.experiments,
        )
        for row in rows
    ]


def assemble(
    request: BaseSetupRequest,
    rows: Sequence[SensitivityEffect],
    chosen: Sequence[Candidate],
    left_out: list[BaseSetupLeftOut],
    items: dict[str, BaseSetupItem],
    labels: dict[str, str],
) -> BaseSetup:
    left_out.sort(key=lambda entry: (entry.parameter, entry.clicks))
    return BaseSetup(
        channel=request.channel,
        changes=[_change(c, items[c.row.parameter]) for c in chosen],
        left_out=left_out,
        measured=_measured(rows, labels),
        prediction=predict(chosen, request.channel, request.level) if chosen else None,
    )
