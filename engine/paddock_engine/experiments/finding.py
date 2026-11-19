"""What an experiment found, what to do next, and the ledger decision (ADR 0029).

The finding is read off the primary interval against the margin D, the interval turned so that
a gain is positive ([L, U], at the look's level), in this order:

| Finding | Rule |
| --- | --- |
| invalid | the experiment cannot be judged (see `Evidence.invalid`) |
| confirmed improvement | L > D, at least 8 blocks (a pre-registered look) |
| confirmed regression | U < -D, at least 8 blocks |
| context-dependent | the arm acts differently between speed classes (Holm, block level) |
| neutral | -D < L and U < D: two one-sided tests |
| likely improvement | L > 0 |
| likely regression | U < 0 |
| insufficient evidence | otherwise; flagged uninformative when (U - L) / 2 > 2 D |

The finding is orthogonal to the ledger's decision, which keeps its meaning for the champion
and for training:

| Finding | Decision |
| --- | --- |
| confirmed improvement | accepted; tradeoff when a guardrail moved or broke |
| confirmed regression, neutral, likely regression | rejected: the baseline stays |
| likely improvement, context-dependent, insufficient | inconclusive (rejected: a guardrail broke) |
| invalid | inconclusive: nothing is decided on numbers that cannot be trusted |

Ties go to the baseline.
"""

from collections.abc import Sequence
from dataclasses import dataclass

from paddock_engine.contracts.evaluate_response_schema import (
    Decision,
    FindingClass,
    Flag,
    GuardrailResult,
    Recommendation,
    Status,
)
from paddock_engine.stats.rope import Finding, interval_finding
from paddock_engine.stats.sequential import FIRST_LOOK_BLOCKS

UNINFORMATIVE_MARGINS = 2.0  # half-width, in margins, above which an interval says little
REJECTED = {
    FindingClass.confirmed_regression,
    FindingClass.neutral,
    FindingClass.likely_regression,
}


@dataclass(frozen=True)
class Evidence:
    gain: float  # the primary effect, positive when better
    gain_low: float
    gain_high: float
    margin: float
    blocks: int
    look: int
    extensions_left: int
    invalid: bool
    context: bool  # a speed-class interaction survived Holm
    response_moved: bool  # a guardrail or predicted channel moved (Holm)


def classify(evidence: Evidence) -> tuple[FindingClass, list[Flag]]:
    flags = [Flag.few_blocks] if evidence.blocks < FIRST_LOOK_BLOCKS else []
    if evidence.invalid:
        return FindingClass.invalid, flags
    confirmable = evidence.look >= 1 and evidence.blocks >= FIRST_LOOK_BLOCKS
    found = interval_finding(evidence.gain_low, evidence.gain_high, evidence.margin, confirmable)
    confirmed = found in (Finding.CONFIRMED_IMPROVEMENT, Finding.CONFIRMED_REGRESSION)
    if evidence.context and not confirmed:
        return FindingClass.context_dependent, flags
    half_width = (evidence.gain_high - evidence.gain_low) / 2.0
    if (
        found is Finding.INSUFFICIENT_EVIDENCE
        and half_width > UNINFORMATIVE_MARGINS * evidence.margin
    ):
        flags.append(Flag.uninformative)
    return FindingClass(found.value), flags


def recommend(finding: FindingClass, evidence: Evidence) -> Recommendation | None:
    """The next step. Futile: an improvement beyond the margin is ruled out (U < D)."""
    stop = Recommendation.switch_metric if evidence.response_moved else Recommendation.stop_neutral
    match finding:
        case FindingClass.invalid:
            return Recommendation.stop_invalid
        case FindingClass.confirmed_improvement | FindingClass.confirmed_regression:
            return None
        case FindingClass.context_dependent:
            return Recommendation.investigate_context
        case FindingClass.neutral | FindingClass.likely_regression:
            return stop
    if evidence.gain_high < evidence.margin:
        return stop
    if evidence.extensions_left > 0:
        return Recommendation.continue_
    if finding is FindingClass.insufficient_evidence and evidence.response_moved:
        return Recommendation.switch_metric
    return Recommendation.larger_perturbation


def decide(finding: FindingClass, guardrails: Sequence[GuardrailResult]) -> Decision:
    statuses = {guardrail.status for guardrail in guardrails}
    if finding is FindingClass.confirmed_improvement:
        worse = statuses & {Status.moved, Status.broke}
        return Decision.tradeoff if worse else Decision.accepted
    if finding in REJECTED:
        return Decision.rejected
    if finding is not FindingClass.invalid and Status.broke in statuses:
        return Decision.rejected
    return Decision.inconclusive
