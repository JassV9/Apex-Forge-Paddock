"""A car's sensitivity profile: POST /sensitivity/update.

One row per item, clicks, channel and corner class. A row is either pooled from the car's own
finished experiments or a prior from another car, flagged as one. The profile is rebuilt
from the experiments' results every time, so it can always be re-derived when the method
changes; `MODEL_VERSION` says which method made the rows.
"""

from collections import defaultdict
from collections.abc import Sequence

from paddock_engine.contracts.sensitivity_request_schema import (
    Measurement,
    SensitivityRequest,
    Transfer,
)
from paddock_engine.contracts.sensitivity_response_schema import (
    CornerClass,
    EffectClass,
    SensitivityEffect,
    SensitivityResponse,
)
from paddock_engine.sensitivity import priors
from paddock_engine.sensitivity.pooling import Estimate, from_interval, pool
from paddock_engine.sensitivity.shape import nonlinearity
from paddock_engine.stats.rope import classify

MODEL_VERSION = "inverse-variance-pool-1"

Key = tuple[str, int, str, str]  # parameter, clicks, channel, corner class


def _row(key: Key, estimate: Estimate, level: float, rope: float) -> dict[str, object]:
    low, high = estimate.interval(level)
    parameter, clicks, channel, corner_class = key
    return {
        "parameter": parameter,
        "clicks": clicks,
        "channel": channel,
        "corner_class": CornerClass(corner_class),
        "estimate": estimate.mean,
        "ci_low": low,
        "ci_high": high,
        "effect_class": EffectClass(classify(low, high, rope).value),
    }


def _measured(request: SensitivityRequest) -> dict[Key, tuple[Estimate, list[Measurement]]]:
    groups: dict[Key, list[Measurement]] = defaultdict(list)
    for item in request.measurements:
        if item.clicks != 0:
            key = (item.parameter, item.clicks, item.channel, item.corner_class.value)
            groups[key].append(item)
    return {
        key: (
            pool([from_interval(m.estimate, m.ci_low, m.ci_high, request.level) for m in items]),
            items,
        )
        for key, items in groups.items()
    }


def _measured_rows(request: SensitivityRequest) -> list[SensitivityEffect]:
    measured = _measured(request)
    by_item: dict[tuple[str, str, str], dict[int, Estimate]] = defaultdict(dict)
    for (parameter, clicks, channel, corner_class), (estimate, _) in measured.items():
        by_item[(parameter, channel, corner_class)][clicks] = estimate
    rows = []
    for key in sorted(measured):
        estimate, items = measured[key]
        parameter, clicks, channel, corner_class = key
        rope = request.ropes.get(channel, 0.0)
        note = nonlinearity(
            clicks, by_item[(parameter, channel, corner_class)], rope, request.level
        )
        rows.append(
            SensitivityEffect.model_validate(
                {
                    **_row(key, estimate, request.level, rope),
                    "n_passes": sum(m.n_passes for m in items),
                    "nonlinear": note != "",
                    "note": note,
                    "is_prior": False,
                    "prior_source": "",
                    "experiment_ids": sorted({m.experiment_id for m in items}),
                }
            )
        )
    return rows


def _prior_rows(
    transfers: Sequence[Transfer], taken: set[Key], request: SensitivityRequest
) -> list[SensitivityEffect]:
    rows = []
    for key, transfer in sorted(priors.best_sources(transfers).items()):
        if key in taken:
            continue  # the car's own measurement wins
        weight = priors.similarity(transfer)
        rope = request.ropes.get(transfer.channel, 0.0)
        rows.append(
            SensitivityEffect.model_validate(
                {
                    **_row(key, priors.prior(transfer, request.level), request.level, rope),
                    "n_passes": 0,
                    "nonlinear": False,
                    "note": (
                        f"Prior from {transfer.source_car}, not measured on this car: its "
                        f"effect times {weight:g}, with the interval widened to match."
                    ),
                    "is_prior": True,
                    "prior_source": transfer.source_car,
                    "experiment_ids": [],
                }
            )
        )
    return rows


def update(request: SensitivityRequest) -> SensitivityResponse:
    """Measurements first, then priors for changes the car has not measured."""
    measured = _measured_rows(request)
    taken = {(row.parameter, row.clicks, row.channel, row.corner_class.value) for row in measured}
    return SensitivityResponse(
        model_version=MODEL_VERSION,
        effects=measured + _prior_rows(request.transfers, taken, request),
    )
