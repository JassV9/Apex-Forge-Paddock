"""Judge a driven experiment: the primary effect at the pre-registered look, the speed classes,
the guardrails, whether the driver adapted, which predictions came true, what the car did, the
finding and what to do next. The work behind POST /evaluate.

Every interval is block-level (ADR 0029). The primary channel is judged with the repeated
confidence interval of its look (stats/sequential.py) against its margin (config/margins.py);
the finding and the decision follow experiments/finding.py. When core sends the raw samples'
whereabouts, the cells the changed parameter is expected to move are judged too
(experiments/response_findings.py, ADR 0034): a finding of their own beside the time's, which
only steers the recommendation. Nothing but the numbers and the test fixed before driving enters.
"""

from dataclasses import dataclass

from paddock_engine import __version__
from paddock_engine.config.margins import margins
from paddock_engine.contracts.evaluate_request_schema import Better, EvaluatePass, EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import (
    ChannelResult,
    Confounder,
    Decision,
    EvaluateResponse,
    FindingClass,
    Flag,
    Kind,
    Recommendation,
    Role,
)
from paddock_engine.experiments.channel_effects import (
    BASELINE,
    channel_effect,
    experiment_regimes,
    observations,
    per_speed_class,
    to_result,
)
from paddock_engine.experiments.context import NO_CONTEXT, Context, context, reportable_classes
from paddock_engine.experiments.finding import Evidence, classify, decide, recommend
from paddock_engine.experiments.reasons import confounders, finding_reason, notes
from paddock_engine.experiments.response_findings import any_moved
from paddock_engine.experiments.secondary import Secondary, secondary
from paddock_engine.experiments.vehicle_verdict import VehicleVerdict, vehicle_verdict
from paddock_engine.response.service import Samples
from paddock_engine.stats.effects import Effect
from paddock_engine.stats.observations import build_design
from paddock_engine.stats.sequential import interval_level, look_of

# An experiment with more than this share of its push-lap passes on the wrong setup or from
# another driver is invalid: the comparison is not the one that was planned.
MAX_EXCLUDED_SHARE = 0.05


@dataclass(frozen=True)
class _Request:
    used: list[EvaluatePass]
    missing: int  # passes without the primary channel
    mismatched: tuple[int, int]  # setup, source
    look: int
    margin: float | None


def _prepare(request: EvaluateRequest) -> _Request:
    test = request.success_test
    used = [p for p in request.passes if p.metrics.get(test.channel) is not None]
    excluded = request.excluded
    mismatched = (excluded.setup_mismatch, excluded.source_mismatch) if excluded else (0, 0)
    look = request.look
    if look is None:
        look = look_of(len({p.block for p in request.passes}))
    margin = margins().margin(test.channel, request.parameter, test.rope)
    return _Request(used, len(request.passes) - len(used), mismatched, look, margin)


def _sessions_tangled(passes: list[EvaluatePass], channel: str) -> bool:
    try:
        return build_design(observations(passes, channel), BASELINE).session_confounded
    except ValueError:
        return False


def _invalid(
    request: EvaluateRequest, prepared: _Request, primary: Effect | None, tangled: bool
) -> str | None:
    """Why the experiment cannot be judged, or None."""
    if tangled:
        return "The game sessions cannot be told apart from the setups (ADR 0029)."
    if primary is None:
        return "Not enough passes on both setups to estimate the effect."
    if prepared.margin is None:
        return f"No margin is configured for {request.success_test.channel}."
    offtrack = sum(block.passes for block in request.offtrack or [])
    total = len(request.passes) + sum(prepared.mismatched) + offtrack
    if total and sum(prepared.mismatched) / total > MAX_EXCLUDED_SHARE:
        return (
            f"{sum(prepared.mismatched)} of {total} passes were driven on another setup or by "
            "another driver (ADR 0029)."
        )
    return None


def _gain(primary: Effect, better: Better) -> tuple[float, float, float]:
    if better is Better.lower:
        return -primary.estimate, -primary.ci_high, -primary.ci_low
    return primary.estimate, primary.ci_low, primary.ci_high


def _evidence(
    request: EvaluateRequest,
    prepared: _Request,
    primary: Effect,
    invalid: bool,
    found: Context,
    moved: bool,
) -> Evidence:
    gain, low, high = _gain(primary, request.success_test.better)
    return Evidence(
        gain=gain,
        gain_low=low,
        gain_high=high,
        margin=prepared.margin or 0.0,
        blocks=primary.blocks,
        look=prepared.look,
        extensions_left=request.extensions_left or 0,
        invalid=invalid,
        context=found.found,
        response_moved=moved,
    )


def _not_estimable(prepared: _Request, reason: str, tangled: bool) -> EvaluateResponse:
    tangles = [Confounder(kind=Kind.session_tangled, name="session terms")] if tangled else []
    return EvaluateResponse(
        decision=Decision.inconclusive,
        reason=reason,
        results=[],
        guardrails=[],
        prediction_hits=0,
        prediction_total=0,
        driver_adapted=False,
        passes_used=len(prepared.used),
        passes_excluded=prepared.missing + sum(prepared.mismatched),
        finding_class=FindingClass.invalid,
        flags=[],
        recommendation=Recommendation.stop_invalid,
        look=prepared.look,
        confounders=tangles + confounders(None, [], NO_CONTEXT, prepared.mismatched),
        engine_version=__version__,
    )


def _primary_results(
    request: EvaluateRequest, prepared: _Request, primary: Effect, classes: dict[int, str]
) -> list[ChannelResult]:
    """The primary effect over all corners, then per speed class where a class is worth it."""
    test = request.success_test
    margin = prepared.margin or 0.0
    found = per_speed_class(prepared.used, test.channel, margin, test.level, Role.primary, classes)
    return [
        to_result(primary, test.channel, Role.primary),
        *reportable_classes(found, prepared.used, primary, classes),
    ]


@dataclass(frozen=True)
class _Judged:
    """Everything the verdict reads beside the primary effect."""

    side: Secondary
    car: VehicleVerdict
    classes: dict[int, str]  # the verdict's one regime map
    found: Context


def _judge_around(
    request: EvaluateRequest, prepared: _Request, invalid: str | None, samples: Samples | None
) -> _Judged:
    car = vehicle_verdict(request, samples)
    data = car.data
    classes = data.regimes.classes if data else experiment_regimes(prepared.used).classes
    channel = request.success_test.channel
    found = NO_CONTEXT if invalid else context(prepared.used, channel, classes, data)
    return _Judged(secondary(request, prepared.used), car, classes, found)


def evaluate(request: EvaluateRequest, samples: Samples | None = None) -> EvaluateResponse:
    """`samples`: the raw samples of request.response's sessions, keyed by (session, seq)."""
    test = request.success_test
    prepared = _prepare(request)
    level = interval_level(prepared.look, test.level)
    primary = channel_effect(prepared.used, test.channel, prepared.margin or 0.0, level)
    tangled = primary is None and _sessions_tangled(prepared.used, test.channel)
    invalid = _invalid(request, prepared, primary, tangled)
    if primary is None or prepared.margin is None:
        return _not_estimable(prepared, invalid or "", tangled)
    around = _judge_around(request, prepared, invalid, samples)
    side = around.side
    moved = bool(side.moved) or any_moved(around.car.findings)
    evidence = _evidence(request, prepared, primary, invalid is not None, around.found, moved)
    finding, flags = classify(evidence)
    decision = decide(finding, side.guardrails)
    reason = invalid or finding_reason(
        finding, decision, test.channel, primary, prepared.look, side.guardrails
    )
    return EvaluateResponse(
        decision=decision,
        reason=reason + notes(primary, side.shifted, Flag.few_blocks in flags),
        results=[*_primary_results(request, prepared, primary, around.classes), *side.results],
        guardrails=side.guardrails,
        prediction_hits=side.prediction_hits,
        prediction_total=side.prediction_total,
        driver_adapted=bool(side.shifted),
        passes_used=len(prepared.used),
        passes_excluded=prepared.missing + sum(prepared.mismatched),
        finding_class=finding,
        flags=flags,
        recommendation=recommend(finding, evidence),
        look=prepared.look,
        margin=prepared.margin,
        confounders=confounders(primary, side.shifted, around.found, prepared.mismatched),
        engine_version=__version__,
        responses=around.car.findings,
        response_parameter=around.car.parameter,
        response_notes=around.car.notes,
    )
