"""The finding classes, recommendations and decisions (ADR 0029), rule by rule."""

import pytest
from hypothesis import given
from hypothesis import strategies as st

from paddock_engine.config.margins import Margins, MarginsError, margins, parse
from paddock_engine.contracts.evaluate_response_schema import (
    Decision,
    FindingClass,
    Flag,
    GuardrailResult,
    Recommendation,
    Status,
)
from paddock_engine.experiments.finding import Evidence, classify, decide, recommend
from paddock_engine.stats.multiplicity import holm, two_sided_p
from paddock_engine.stats.rope import Finding, interval_finding

D = 0.005


def evidence(low: float, high: float, **overrides: object) -> Evidence:
    fields = {
        "gain": (low + high) / 2,
        "gain_low": low,
        "gain_high": high,
        "margin": D,
        "blocks": 8,
        "look": 1,
        "extensions_left": 2,
        "invalid": False,
        "context": False,
        "response_moved": False,
    } | overrides
    return Evidence(**fields)  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("low", "high", "overrides", "finding", "flags"),
    [
        (0.006, 0.010, {}, FindingClass.confirmed_improvement, []),
        (-0.010, -0.006, {}, FindingClass.confirmed_regression, []),
        (-0.004, 0.004, {}, FindingClass.neutral, []),
        (0.001, 0.009, {}, FindingClass.likely_improvement, []),
        (-0.009, -0.001, {}, FindingClass.likely_regression, []),
        (-0.004, 0.009, {}, FindingClass.insufficient_evidence, []),
        (-0.012, 0.012, {}, FindingClass.insufficient_evidence, [Flag.uninformative]),
        (
            0.006,
            0.010,
            {"blocks": 4, "look": 0},
            FindingClass.likely_improvement,
            [Flag.few_blocks],
        ),
        (
            -0.010,
            -0.006,
            {"blocks": 6, "look": 0},
            FindingClass.likely_regression,
            [Flag.few_blocks],
        ),
        (0.006, 0.010, {"invalid": True}, FindingClass.invalid, []),
        (-0.004, 0.004, {"context": True}, FindingClass.context_dependent, []),
        (0.006, 0.010, {"context": True}, FindingClass.confirmed_improvement, []),
    ],
)
def test_each_finding_follows_its_rule(
    low: float,
    high: float,
    overrides: dict[str, object],
    finding: FindingClass,
    flags: list[Flag],
) -> None:
    assert classify(evidence(low, high, **overrides)) == (finding, flags)


@pytest.mark.parametrize(
    ("finding", "low", "high", "overrides", "next_step"),
    [
        (FindingClass.invalid, 0.0, 0.0, {}, Recommendation.stop_invalid),
        (FindingClass.confirmed_improvement, 0.006, 0.01, {}, None),
        (FindingClass.context_dependent, -0.004, 0.004, {}, Recommendation.investigate_context),
        (FindingClass.neutral, -0.004, 0.004, {}, Recommendation.stop_neutral),
        (
            FindingClass.neutral,
            -0.004,
            0.004,
            {"response_moved": True},
            Recommendation.switch_metric,
        ),
        (FindingClass.likely_regression, -0.009, -0.001, {}, Recommendation.stop_neutral),
        (FindingClass.insufficient_evidence, -0.009, 0.004, {}, Recommendation.stop_neutral),
        (FindingClass.insufficient_evidence, -0.004, 0.009, {}, Recommendation.continue_),
        (FindingClass.likely_improvement, 0.001, 0.009, {}, Recommendation.continue_),
        (
            FindingClass.likely_improvement,
            0.001,
            0.009,
            {"extensions_left": 0},
            Recommendation.larger_perturbation,
        ),
        (
            FindingClass.insufficient_evidence,
            -0.004,
            0.009,
            {"extensions_left": 0, "response_moved": True},
            Recommendation.switch_metric,
        ),
    ],
)
def test_the_next_step_follows_the_finding(
    finding: FindingClass,
    low: float,
    high: float,
    overrides: dict[str, object],
    next_step: Recommendation | None,
) -> None:
    assert recommend(finding, evidence(low, high, **overrides)) is next_step


def guardrails(*statuses: Status) -> list[GuardrailResult]:
    return [GuardrailResult(channel=f"g{i}", status=s) for i, s in enumerate(statuses)]


@pytest.mark.parametrize(
    ("finding", "statuses", "decision"),
    [
        (FindingClass.confirmed_improvement, (Status.holds,), Decision.accepted),
        (FindingClass.confirmed_improvement, (Status.holds, Status.moved), Decision.tradeoff),
        (FindingClass.confirmed_regression, (), Decision.rejected),
        (FindingClass.neutral, (), Decision.rejected),  # nothing to gain: the baseline stays
        (FindingClass.likely_regression, (), Decision.rejected),
        (FindingClass.likely_improvement, (Status.holds,), Decision.inconclusive),
        (FindingClass.likely_improvement, (Status.broke,), Decision.rejected),
        (FindingClass.context_dependent, (), Decision.inconclusive),
        (FindingClass.insufficient_evidence, (Status.unknown,), Decision.inconclusive),
        (FindingClass.invalid, (Status.broke,), Decision.inconclusive),
    ],
)
def test_the_ledger_decision_keeps_its_meaning(
    finding: FindingClass, statuses: tuple[Status, ...], decision: Decision
) -> None:
    assert decide(finding, guardrails(*statuses)) is decision


@given(low=st.floats(-0.05, 0.05), width=st.floats(0.0, 0.05), confirmable=st.booleans())
def test_a_neutral_interval_lies_inside_the_margin_and_a_confirmed_one_beyond_it(
    low: float, width: float, confirmable: bool
) -> None:
    high = low + width
    found = interval_finding(low, high, D, confirmable)

    if found is Finding.NEUTRAL:
        assert low > -D and high < D
    if found in (Finding.CONFIRMED_IMPROVEMENT, Finding.CONFIRMED_REGRESSION):
        assert confirmable and (low > D or high < -D)
    if found is Finding.LIKELY_IMPROVEMENT:
        assert low > 0
    if found is Finding.INSUFFICIENT_EVIDENCE:
        assert low <= 0 <= high


def test_a_margin_of_zero_is_refused() -> None:
    with pytest.raises(ValueError):
        interval_finding(-0.001, 0.001, 0.0, True)


def test_holm_steps_down_and_keeps_the_family_error() -> None:
    assert holm([0.01, 0.04, 0.03], 0.05) == [True, False, False]
    assert holm([0.001, 0.02, 0.04], 0.05) == [True, True, True]
    assert holm([0.2, 0.01], 0.05) == [False, True]
    assert holm([], 0.05) == []
    assert two_sided_p(0.0, 0.0, 5) == 1.0 and two_sided_p(0.1, 0.0, 5) == 0.0


def test_margins_come_from_the_item_then_the_experiment_then_the_metric() -> None:
    config = parse("[metrics]\ncorner_time_s = 0.005\n[parameters.WING_1]\ncorner_time_s = 0.003\n")

    assert config.margin("corner_time_s", "WING_1", 0.005) == 0.003
    assert config.margin("corner_time_s", "ARB_R", 0.004) == 0.004
    assert config.margin("corner_time_s", None, 0.0) == 0.005  # never a band of 0
    assert config.margin("v_min_kmh", None, 0.0) is None


@pytest.mark.parametrize("value", ["0", "-0.1", "true", "'5 ms'"])
def test_a_margin_must_be_a_positive_number(value: str) -> None:
    with pytest.raises(MarginsError):
        parse(f"[metrics]\ncorner_time_s = {value}\n")


def test_the_configured_defaults_are_the_documented_ones() -> None:
    assert margins() == Margins(
        metrics={
            "corner_time_s": 0.005,
            "rear_slip_peak_deg": 0.5,
            "countersteer_s": 0.05,
            "offtrack_rate": 0.05,
        }
    )
