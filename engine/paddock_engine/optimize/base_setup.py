"""A base setup from what this car measured: POST /base-setup (ADR 0018, 0033).

A change goes in when its row of the car's own profile, on the judged channel over all corners
and the whole corner on this track (never the all-track scope, never a stale row):

1. is a **confirmed** or **preliminary** measurement whose random-effects interval lies wholly
   on the better side of zero (a confirmed or likely improvement; sensitivity/knowledge.py);
2. passes **Benjamini-Hochberg** at q = 0.05 with its one-sided p-value, across every measured
   change of the car on the judged channel (optimize/fdr.py);
3. is not one of a pair, not flagged non-linear, not measured worse on a guardrail and stays in
   the item's range. One change per item: the click amount with the best shrunk estimate.

It does not have to clear the band of practical equivalence: the combination is judged
afterwards as its own A-B-A-B experiment against the baseline, and that verdict is what makes
it the champion. A prior from another car is never taken; it is listed as left out, labelled.

The prediction is the sum of the estimates **shrunk towards zero** (empirical Bayes across the
car's measured changes; the winner's curse, finding I13), with an interval from the summed
measured variances: shrinking moves the centre, it does not buy certainty. The items were measured
one at a time; how they interact was not, and the note says so.
"""

from collections import defaultdict
from collections.abc import Sequence

from paddock_engine.contracts.base_setup_request_schema import (
    BaseSetupItem,
    BaseSetupRequest,
    Better,
    EffectClass,
    KnowledgeLevel,
    SensitivityEffect,
    Worse,
)
from paddock_engine.contracts.base_setup_schema import BaseSetup, BaseSetupLeftOut
from paddock_engine.optimize import fdr
from paddock_engine.optimize.base_output import Candidate, assemble, target_value

ALL_CORNERS = "all"
WHOLE_CORNER = "corner"
ALL_TRACKS = "all"
RANGE_TOLERANCE = 1e-9
QUALIFYING = (KnowledgeLevel.confirmed, KnowledgeLevel.preliminary)


def span(row: SensitivityEffect) -> str:
    return f"{row.estimate:+.4f} [{row.ci_low:+.4f}, {row.ci_high:+.4f}]"


def _p_better(row: SensitivityEffect, better: Better) -> float:
    return row.p_increase if better is Better.higher else 1.0 - row.p_increase


def _wholly_better(row: SensitivityEffect, better: Better) -> bool:
    return row.ci_high < 0 if better is Better.lower else row.ci_low > 0


def _overall(row: SensitivityEffect) -> bool:
    return row.corner_class.value == ALL_CORNERS and row.phase == WHOLE_CORNER


def _primary_rows(request: BaseSetupRequest) -> list[SensitivityEffect]:
    rows = [
        row
        for row in request.effects
        if row.channel == request.channel and _overall(row) and row.track != ALL_TRACKS
    ]
    return sorted(rows, key=lambda row: (row.parameter, row.clicks, row.hint_bucket))


def _same_change(other: SensitivityEffect, row: SensitivityEffect, channel: str) -> bool:
    """The same change, in the same stratum, on another channel."""
    here = (row.parameter, row.clicks, channel, row.track, row.hint_bucket, row.driver)
    return here == (
        other.parameter,
        other.clicks,
        other.channel,
        other.track,
        other.hint_bucket,
        other.driver,
    )


def _guardrail_reason(row: SensitivityEffect, request: BaseSetupRequest) -> str:
    """Why the same change is worse on a guardrail, from this car's own measurement."""
    for spec in request.guardrails:
        worse = EffectClass.positive if spec.worse is Worse.higher else EffectClass.negative
        for other in request.effects:
            if (
                _same_change(other, row, spec.channel)
                and _overall(other)
                and other.effect_class is worse
            ):
                return f"Measured worse on the guardrail {spec.channel}: {span(other)}."
    return ""


def _evidence_reason(row: SensitivityEffect, request: BaseSetupRequest, discovered: bool) -> str:
    if row.stale:
        return f"Not re-derived by the current engine (the ledger's numbers): {span(row)}."
    if row.knowledge_level not in QUALIFYING or not _wholly_better(row, request.better):
        return f"Not measured better on {request.channel}: {span(row)} ({row.knowledge_level})."
    if not discovered:
        p = _p_better(row, request.better)
        return f"Better at p = {p:.3f}, but not after Benjamini-Hochberg at q = {fdr.FDR_Q:g}."
    return ""


def _reason(
    row: SensitivityEffect, item: BaseSetupItem | None, request: BaseSetupRequest, found: bool
) -> str:
    """Why a row cannot go into the base setup; empty when it can."""
    if item is None:
        return "Not a changeable item of the baseline setup."
    if item.paired:
        return "One of a pair (per wheel or side); a pair is not changed one item at a time."
    if row.nonlinear:
        return f"Flagged non-linear: {row.note}"
    if evidence := _evidence_reason(row, request, found):
        return evidence
    if guardrail := _guardrail_reason(row, request):
        return guardrail
    target = target_value(item, row.clicks)
    if not item.min - RANGE_TOLERANCE <= target <= item.max + RANGE_TOLERANCE:
        return f"Would set {target:g}, outside {item.min:g} to {item.max:g}."
    return ""


def _candidates(rows: Sequence[SensitivityEffect], request: BaseSetupRequest) -> list[Candidate]:
    """Every primary row with its one-sided p, whether it passed BH and its shrunk estimate."""
    usable = [row for row in rows if not row.stale]
    variance = fdr.prior_variance([r.estimate for r in usable], [r.standard_error for r in usable])
    p_values = [_p_better(row, request.better) for row in rows]
    found = fdr.benjamini_hochberg(p_values)
    return [
        Candidate(row, p, hit, fdr.shrink(row.estimate, row.standard_error, variance))
        for row, p, hit in zip(rows, p_values, found, strict=True)
    ]


def _best_per_item(
    candidates: Sequence[Candidate], better: Better
) -> tuple[list[Candidate], list[tuple[Candidate, Candidate]]]:
    """The best click amount of each item by shrunk gain, and each runner-up with its winner."""
    by_item: dict[str, list[Candidate]] = defaultdict(list)
    for candidate in candidates:
        by_item[candidate.row.parameter].append(candidate)
    sign = -1.0 if better is Better.lower else 1.0
    chosen, beaten = [], []
    for parameter in sorted(by_item):
        ranked = sorted(by_item[parameter], key=lambda c: (-sign * c.shrunk, abs(c.row.clicks)))
        chosen.append(ranked[0])
        beaten += [(c, ranked[0]) for c in ranked[1:]]
    return chosen, beaten


def _prior_left_out(request: BaseSetupRequest, labels: dict[str, str]) -> list[BaseSetupLeftOut]:
    return [
        BaseSetupLeftOut(
            parameter=p.parameter,
            label=labels.get(p.parameter, p.parameter),
            clicks=p.clicks,
            reason=f"Prior from {p.source_car}, not measured on this car; never taken.",
        )
        for p in request.priors
        if p.channel == request.channel and p.corner_class.value == ALL_CORNERS
    ]


def _left_out(row: SensitivityEffect, labels: dict[str, str], reason: str) -> BaseSetupLeftOut:
    label = labels.get(row.parameter, row.parameter)
    return BaseSetupLeftOut(parameter=row.parameter, label=label, clicks=row.clicks, reason=reason)


def base_setup(request: BaseSetupRequest) -> BaseSetup:
    items = {item.name: item for item in request.items}
    labels = {item.name: item.label for item in request.items}
    rows = _primary_rows(request)
    left_out, taken = _prior_left_out(request, labels), []
    for candidate in _candidates(rows, request):
        row = candidate.row
        reason = _reason(row, items.get(row.parameter), request, candidate.discovered)
        if reason:
            left_out.append(_left_out(row, labels, reason))
        else:
            taken.append(candidate)
    chosen, beaten = _best_per_item(taken, request.better)
    for runner_up, winner in beaten:
        reason = f"{winner.row.clicks:+d} clicks of this item measured better: {span(winner.row)}."
        left_out.append(_left_out(runner_up.row, labels, reason))
    return assemble(request, rows, chosen, left_out, items, labels)
