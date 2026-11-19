"""POST /discovery/curve: each observable's response to a swept item (ADR 0032).

The request carries, per observable (metric, phase, speed regime), the effect of every step of
a sweep; each step's interval is read at its own level (the repeated confidence interval of its
pre-registered look for the verdict's primary channel, ADR 0029) and classified by
`discovery.curve`.
"""

from paddock_engine import __version__
from paddock_engine.contracts.discovery_curve_request_schema import (
    CurveObservable,
    DiscoveryCurveRequest,
)
from paddock_engine.contracts.discovery_curve_response_schema import (
    CurveEstimate,
    CurveResult,
    DiscoveryCurve,
)
from paddock_engine.contracts.discovery_curve_response_schema import CurveShape as ShapeOut
from paddock_engine.discovery.curve import Coefficient, Point, classify, standard_error
from paddock_engine.response.config import observables
from paddock_engine.stats.sequential import interval_level

DEFAULT_LEVEL = 0.90
# Units read from a metric's name when the response configuration does not list it.
_SUFFIXES = (
    ("_per_s", "1/s"),
    ("_kmh", "km/h"),
    ("_deg", "deg"),
    ("_mm", "mm"),
    ("_s", "s"),
    ("_m", "m"),
)


def unit_of(metric: str) -> str:
    """The metric's unit: the response configuration's, else its name's suffix, else ''."""
    spec = observables().metrics.get(metric)
    if spec is not None:
        return spec.unit
    return next((unit for suffix, unit in _SUFFIXES if metric.endswith(suffix)), "")


def points(observable: CurveObservable) -> list[Point]:
    """Each step's effect with the standard error its interval implies at its own level."""
    out = []
    for step in observable.steps:
        level = interval_level(step.look or 0, step.level)
        se = standard_error(step.ci_low, step.ci_high, level)
        out.append(Point(step.clicks, step.estimate, se))
    return out


def _estimate(coefficient: Coefficient | None) -> CurveEstimate | None:
    if coefficient is None:
        return None
    return CurveEstimate(
        estimate=coefficient.estimate, ci_low=coefficient.low, ci_high=coefficient.high
    )


def curve_of(observable: CurveObservable, level: float) -> CurveResult:
    found = classify(points(observable), observable.margin, level)
    return CurveResult(
        metric=observable.metric,
        phase=observable.phase.value,
        regime=observable.regime.value,
        source=observable.source.value,
        role=observable.role,
        expected=observable.expected.value if observable.expected else None,
        unit=unit_of(observable.metric),
        shape=ShapeOut(found.shape.value),
        clicks=list(found.clicks),
        slope=_estimate(found.slope),
        curvature=_estimate(found.curvature),
        turn_clicks=found.turn_clicks,
        margin=observable.margin,
        reason=found.reason,
    )


def curves(request: DiscoveryCurveRequest) -> DiscoveryCurve:
    level = request.level or DEFAULT_LEVEL
    return DiscoveryCurve(
        parameter=request.parameter,
        level=level,
        engine_version=__version__,
        curves=[curve_of(observable, level) for observable in request.observables],
    )
