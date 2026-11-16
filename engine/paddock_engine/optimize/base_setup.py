"""A base setup from what this car measured: POST /base-setup.

A change goes in when the car's own pooled measurement on the judged channel, over all
corners, lies wholly on the better side of zero. It does not have to clear the band of
practical equivalence: the combination is judged afterwards as its own A-B-A-B experiment
against the baseline, and that verdict is what makes it the champion (docs/adr/0018).

Never taken: a prior from another car, a row flagged non-linear, a change measured worse on
a guardrail, an item that is one of a pair (per wheel or side), a change that would leave the
item's range. One change per item: the measured click amount with the best estimate.

The prediction is the sum of the estimates with an interval from the summed variances. The
items were measured one at a time; how they interact was not, and the note says so.
"""

import math
from collections import defaultdict
from collections.abc import Sequence

from paddock_engine.contracts.base_setup_request_schema import (
    BaseSetupItem,
    BaseSetupRequest,
    Better,
    EffectClass,
    SensitivityEffect,
    Worse,
)
from paddock_engine.contracts.base_setup_schema import (
    BaseSetup,
    BaseSetupChange,
    BaseSetupLeftOut,
    BaseSetupMeasured,
    BaseSetupPrediction,
)
from paddock_engine.sensitivity.pooling import quantile

ALL_CORNERS = "all"
RANGE_TOLERANCE = 1e-9


def _span(row: SensitivityEffect) -> str:
    return f"{row.estimate:+.4f} [{row.ci_low:+.4f}, {row.ci_high:+.4f}]"


def _gain(row: SensitivityEffect, better: Better) -> float:
    """How much better the row is, positive when it helps."""
    return -row.estimate if better is Better.lower else row.estimate


def _wholly_better(row: SensitivityEffect, better: Better) -> bool:
    return row.ci_high < 0 if better is Better.lower else row.ci_low > 0


def _primary_rows(request: BaseSetupRequest) -> list[SensitivityEffect]:
    rows = [
        row
        for row in request.effects
        if row.channel == request.channel and row.corner_class.value == ALL_CORNERS
    ]
    return sorted(rows, key=lambda row: (row.parameter, row.clicks, row.is_prior))


def _target(item: BaseSetupItem, clicks: int) -> float:
    return round(item.value + clicks * item.step, 9)


def _guardrail_reason(row: SensitivityEffect, request: BaseSetupRequest) -> str:
    """Why the same change is worse on a guardrail, from this car's own measurement."""
    for spec in request.guardrails:
        worse = EffectClass.positive if spec.worse is Worse.higher else EffectClass.negative
        for other in request.effects:
            if (
                other.parameter == row.parameter
                and other.clicks == row.clicks
                and other.channel == spec.channel
                and other.corner_class.value == ALL_CORNERS
                and not other.is_prior
                and other.effect_class is worse
            ):
                return f"Measured worse on the guardrail {spec.channel}: {_span(other)}."
    return ""


def _reason(row: SensitivityEffect, item: BaseSetupItem | None, request: BaseSetupRequest) -> str:
    """Why a row cannot go into the base setup; empty when it can."""
    if row.is_prior:
        return f"Prior from {row.prior_source}, not measured on this car."
    if item is None:
        return "Not a changeable item of the baseline setup."
    if item.paired:
        return "One of a pair (per wheel or side); a pair is not changed one item at a time."
    if row.nonlinear:
        return f"Flagged non-linear: {row.note}"
    if not _wholly_better(row, request.better):
        return f"Not measured better on {request.channel}: {_span(row)} reaches zero."
    if guardrail := _guardrail_reason(row, request):
        return guardrail
    target = _target(item, row.clicks)
    if not item.min - RANGE_TOLERANCE <= target <= item.max + RANGE_TOLERANCE:
        return f"Would set {target:g}, outside {item.min:g} to {item.max:g}."
    return ""


def _left_out(row: SensitivityEffect, label: str, reason: str) -> BaseSetupLeftOut:
    return BaseSetupLeftOut(parameter=row.parameter, label=label, clicks=row.clicks, reason=reason)


def _change(row: SensitivityEffect, item: BaseSetupItem) -> BaseSetupChange:
    return BaseSetupChange(
        parameter=row.parameter,
        label=item.label,
        clicks=row.clicks,
        from_value=item.value,
        to_value=_target(item, row.clicks),
        estimate=row.estimate,
        ci_low=row.ci_low,
        ci_high=row.ci_high,
        n_passes=row.n_passes,
        effect_class=row.effect_class.value,
        experiment_ids=row.experiment_ids,
    )


def _best_per_item(
    candidates: Sequence[SensitivityEffect], better: Better
) -> tuple[list[SensitivityEffect], list[tuple[SensitivityEffect, SensitivityEffect]]]:
    """The best-measured click amount of each item, and each runner-up with its winner."""
    by_item: dict[str, list[SensitivityEffect]] = defaultdict(list)
    for row in candidates:
        by_item[row.parameter].append(row)
    chosen, beaten = [], []
    for parameter in sorted(by_item):
        rows = sorted(by_item[parameter], key=lambda r: (-_gain(r, better), abs(r.clicks)))
        chosen.append(rows[0])
        beaten += [(row, rows[0]) for row in rows[1:]]
    return chosen, beaten


def predict(changes: Sequence[BaseSetupChange], channel: str, level: float) -> BaseSetupPrediction:
    """The sum of the estimates, with the variances implied by the intervals summed."""
    z = quantile(level)
    variance = sum(((c.ci_high - c.ci_low) / (2.0 * z)) ** 2 for c in changes)
    estimate = sum(c.estimate for c in changes)
    half = z * math.sqrt(variance)
    return BaseSetupPrediction(
        channel=channel,
        estimate=estimate,
        ci_low=estimate - half,
        ci_high=estimate + half,
        level=level,
        note=(
            f"Sum of {len(changes)} effects measured one item at a time. How the items "
            "interact was not measured, so the combination is judged as its own experiment."
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
        )
        for row in rows
        if not row.is_prior
    ]


def base_setup(request: BaseSetupRequest) -> BaseSetup:
    items = {item.name: item for item in request.items}
    labels = {item.name: item.label for item in request.items}
    rows = _primary_rows(request)
    left_out, candidates = [], []
    for row in rows:
        reason = _reason(row, items.get(row.parameter), request)
        if reason:
            left_out.append(_left_out(row, labels.get(row.parameter, row.parameter), reason))
        else:
            candidates.append(row)
    chosen, beaten = _best_per_item(candidates, request.better)
    for row, winner in beaten:
        reason = f"{winner.clicks:+d} clicks of this item measured better: {_span(winner)}."
        left_out.append(_left_out(row, labels[row.parameter], reason))
    changes = [_change(row, items[row.parameter]) for row in chosen]
    left_out.sort(key=lambda entry: (entry.parameter, entry.clicks))
    return BaseSetup(
        channel=request.channel,
        changes=changes,
        left_out=left_out,
        measured=_measured(rows, labels),
        prediction=predict(changes, request.channel, request.level) if changes else None,
    )
