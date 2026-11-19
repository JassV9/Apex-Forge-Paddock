"""A car's sensitivity profile: POST /sensitivity/update (ADR 0011, 0033).

One measured row per stratum (sensitivity/strata.py): the car's own experiments pooled by random
effects (sensitivity/pooling.py), with an explicit knowledge level (sensitivity/knowledge.py) and
its provenance (experiments, engine versions, stale or not). Priors from other cars come back in
their own list, without a class, only for changes the car has not measured in any stratum.
Experiments that were not pooled are listed with the reason. `MODEL_VERSION` says which method
made the rows.
"""

from collections import defaultdict

from paddock_engine.config.margins import margins
from paddock_engine.contracts.sensitivity_request_schema import SensitivityRequest
from paddock_engine.contracts.sensitivity_response_schema import (
    CornerClass,
    Driver,
    EffectClass,
    SensitivityEffect,
    SensitivityPrior,
    SensitivityResponse,
)
from paddock_engine.sensitivity import priors
from paddock_engine.sensitivity.knowledge import combine, knowledge
from paddock_engine.sensitivity.pooling import Estimate, Pooled
from paddock_engine.sensitivity.shape import nonlinearity
from paddock_engine.sensitivity.strata import Group, Stratum, estimate_of, group
from paddock_engine.stats.rope import classify

MODEL_VERSION = "random-effects-hksj-1"
STALE_NOTE = (
    "Not re-derived by the current engine (the ledger's numbers): never more than insufficient."
)


def _margin(stratum: Stratum, request: SensitivityRequest) -> float | None:
    frozen = request.ropes.get(stratum.channel, 0.0)
    return margins().margin(stratum.channel, stratum.parameter, frozen)


def _pooled(groups: list[Group], request: SensitivityRequest) -> dict[Stratum, Pooled]:
    return {
        g.stratum: combine(
            [estimate_of(m, request.level) for m in g.measurements],
            _margin(g.stratum, request),
            request.level,
        )
        for g in groups
    }


def _row(g: Group, pooled: Pooled, margin: float | None, curve: str) -> SensitivityEffect:
    """`curve` says why the item's response is not a straight line; empty when it is."""
    s = g.stratum
    note = " ".join(text for text in (STALE_NOTE if g.stale else "", curve) if text)
    versions = sorted({m.engine_version for m in g.measurements if m.engine_version})
    prediction = pooled.prediction or (None, None)
    return SensitivityEffect(
        parameter=s.parameter,
        clicks=s.clicks,
        channel=s.channel,
        corner_class=CornerClass(s.corner_class),
        phase=s.phase,
        track=s.track,
        track_name=g.track_name,
        driver=Driver(s.driver),
        hint_bucket=s.hint_bucket,
        baseline_value=s.baseline_value,
        click_step=s.click_step,
        estimate=pooled.estimate.mean,
        ci_low=pooled.low,
        ci_high=pooled.high,
        standard_error=pooled.estimate.se,
        p_increase=min(max(pooled.p(), 0.0), 1.0),
        pi_low=prediction[0],
        pi_high=prediction[1],
        tau=pooled.tau,
        method=pooled.method,
        experiments=pooled.experiments,
        knowledge_level=knowledge(pooled, margin, g.stale),
        effect_class=EffectClass(classify(pooled.low, pooled.high, margin or 0.0).value),
        margin=margin,
        n_passes=sum(m.n_passes for m in g.measurements),
        nonlinear=curve != "",
        note=note,
        stale=g.stale,
        experiment_ids=sorted({m.experiment_id for m in g.measurements}),
        engine_versions=versions,
    )


def _measured_rows(groups: list[Group], request: SensitivityRequest) -> list[SensitivityEffect]:
    pooled = _pooled(groups, request)
    curves: dict[Stratum, dict[int, Estimate]] = defaultdict(dict)
    for stratum, result in pooled.items():
        curves[stratum.response()][stratum.clicks] = result.estimate
    rows = []
    for g in groups:
        margin = _margin(g.stratum, request)
        s = g.stratum
        curve = nonlinearity(s.clicks, curves[s.response()], margin or 0.0, request.level)
        rows.append(_row(g, pooled[s], margin, curve))
    return rows


def _prior_rows(
    request: SensitivityRequest, measured: set[priors.Change]
) -> list[SensitivityPrior]:
    chosen = priors.best_sources(request.transfers)
    return [
        priors.prior(chosen[change], request.level)
        for change in sorted(chosen)
        if change not in measured  # the car's own measurement, in any stratum, wins
    ]


def update(request: SensitivityRequest) -> SensitivityResponse:
    """Measured rows per stratum, then priors for changes the car has not measured."""
    groups, excluded = group(request.measurements)
    rows = _measured_rows(groups, request)
    measured = {g.stratum.change() for g in groups}
    return SensitivityResponse(
        model_version=MODEL_VERSION,
        effects=rows,
        priors=_prior_rows(request, measured),
        excluded=excluded,
    )
