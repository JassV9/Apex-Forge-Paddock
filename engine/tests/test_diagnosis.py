"""Diagnosis: evidence, symptoms, the driver-input alternative and candidate experiments."""

import pytest
from diagnosis_data import car, cota_request, item, passes_of
from hypothesis import given, settings
from hypothesis import strategies as st

from paddock_engine.contracts.diagnose_request_schema import DiagnosePass, DiagnoseRequest
from paddock_engine.contracts.diagnosis_schema import Hypothesis, Kind
from paddock_engine.diagnosis import evidence
from paddock_engine.diagnosis.diagnose import DRIVER_DISCOUNT, diagnose


def hypothesis_of(request: DiagnoseRequest, key: str) -> Hypothesis:
    _, hypotheses = diagnose(request)
    return next(h for h in hypotheses if h.key == key)


def driver_input(hypothesis: Hypothesis) -> tuple[bool, list[str]]:
    alternative = next(a for a in hypothesis.alternatives if a.kind is Kind.driver_input)
    return alternative.supported, alternative.evidence


def test_acceptance_slow_corner_understeer_at_cota_lists_front_saturation_with_its_evidence() -> (
    None
):
    """Phase 4 acceptance. v0 read this session as a balance problem and softened the front
    bar. The top slow-corner understeer hypothesis must say the driver's input explains it
    too, and point at the numbers that say so."""
    pack, hypotheses = diagnose(cota_request())
    ids = {e.id for e in pack}

    slow = [h for h in hypotheses if h.corner_class.value == "slow" and "understeer" in h.key]
    top = slow[0]
    alternative = next(a for a in top.alternatives if a.kind is Kind.driver_input)

    assert top.key == "mid_understeer.slow"
    assert top.statement.startswith("Understeer mid-corner in slow corners")
    assert alternative.supported
    assert "front saturation" in alternative.statement
    assert {"front_slip_mid_deg.slow", "front_slip_mid_deg.fast"} <= set(alternative.evidence)
    assert {"rear_slip_mid_deg.slow", "rear_slip_mid_deg.fast"} <= set(alternative.evidence)
    assert set(alternative.evidence) <= ids
    assert set(top.evidence) <= ids


def test_every_hypothesis_carries_driver_input_and_conditions() -> None:
    _, hypotheses = diagnose(cota_request())

    assert len(hypotheses) >= 3
    for hypothesis in hypotheses:
        assert [a.kind for a in hypothesis.alternatives] == [Kind.driver_input, Kind.conditions]


def test_a_symptom_the_driver_explains_ranks_at_half_its_strength() -> None:
    pack, hypotheses = diagnose(cota_request())
    values = {e.id: e.value for e in pack}
    front, rear = values["front_slip_mid_deg.slow"], values["rear_slip_mid_deg.slow"]
    strength = (front - rear) / (0.12 * (front + rear) / 2)

    slow = next(h for h in hypotheses if h.key == "mid_understeer.slow")

    assert slow.score == pytest.approx(strength * DRIVER_DISCOUNT)
    assert [h.score for h in hypotheses] == sorted((h.score for h in hypotheses), reverse=True)


def test_a_high_front_saturation_index_supports_driver_input() -> None:
    passes = passes_of(
        "medium", front_slip_mid_deg=6.0, rear_slip_mid_deg=3.0, front_saturation_index=0.4
    )

    supported, cited = driver_input(
        hypothesis_of(DiagnoseRequest(passes=passes, items=[]), "mid_understeer.medium")
    )

    assert supported
    assert cited == ["front_saturation_index.medium"]


def test_understeer_at_every_speed_is_not_blamed_on_the_driver() -> None:
    passes = [
        *passes_of("slow", front_slip_mid_deg=6.0, rear_slip_mid_deg=3.0),
        *passes_of("fast", front_slip_mid_deg=5.8, rear_slip_mid_deg=3.0),
    ]

    supported, cited = driver_input(
        hypothesis_of(DiagnoseRequest(passes=passes, items=[]), "mid_understeer.slow")
    )

    assert not supported
    assert cited == []


def test_oversteer_never_gets_a_supported_driver_alternative() -> None:
    passes = passes_of(
        "slow", front_slip_mid_deg=3.0, rear_slip_mid_deg=6.0, front_saturation_index=0.9
    )

    supported, _ = driver_input(
        hypothesis_of(DiagnoseRequest(passes=passes, items=[]), "mid_oversteer.slow")
    )

    assert not supported


def test_balanced_axles_are_no_symptom() -> None:
    passes = passes_of("slow", front_slip_mid_deg=4.2, rear_slip_mid_deg=4.0)

    pack, hypotheses = diagnose(DiagnoseRequest(passes=passes, items=[]))

    assert hypotheses == []
    assert len(pack) > 0


def test_evidence_needs_three_confident_passes() -> None:
    confident = passes_of("slow", count=2, front_slip_mid_deg=8.0, rear_slip_mid_deg=3.0)
    unsure = [
        DiagnosePass(
            corner=2,
            speed_class=confident[0].speed_class,
            metrics={"confidence_mid": 0.2, "front_slip_mid_deg": 8.0, "rear_slip_mid_deg": 3.0},
        )
    ]

    assert evidence.build(confident + unsure) == {}
    assert "front_slip_mid_deg.slow" in evidence.build(confident + confident[:1])


def test_evidence_is_the_mean_over_the_passes_that_have_a_reading() -> None:
    passes = [
        *passes_of("fast", count=2, front_slip_mid_deg=2.0),
        *passes_of("fast", count=2, front_slip_mid_deg=4.0),
        *passes_of("fast", count=1, front_slip_mid_deg=None),
    ]

    reading = evidence.build(passes)["front_slip_mid_deg.fast"]

    assert reading.value == pytest.approx(3.0)
    assert reading.passes == 4
    assert reading.metric == "front_slip_mid_deg"


def test_fast_corner_oversteer_is_tested_with_wings_first() -> None:
    passes = passes_of("fast", front_slip_mid_deg=2.0, rear_slip_mid_deg=4.0)

    hypothesis = hypothesis_of(DiagnoseRequest(passes=passes, items=car()), "mid_oversteer.fast")

    assert [(c.parameter, c.clicks) for c in hypothesis.candidates] == [
        ("WING_2", 1),
        ("WING_1", -1),
        ("ARB_REAR", -1),
    ]
    assert [(e.channel, e.direction.value) for e in hypothesis.candidates[0].expects] == [
        ("corner_time_s", "decrease"),
        ("rear_slip_mid_deg", "decrease"),
    ]


def test_slow_corner_understeer_names_what_cannot_be_tested_and_why() -> None:
    passes = passes_of("slow", front_slip_mid_deg=7.0, rear_slip_mid_deg=3.0)

    hypothesis = hypothesis_of(DiagnoseRequest(passes=passes, items=car()), "mid_understeer.slow")

    assert [(c.parameter, c.clicks) for c in hypothesis.candidates] == [
        ("ARB_FRONT", -1),
        ("ARB_REAR", 1),
    ]
    assert hypothesis.not_testable == [
        "Camber Lf: a left/right pair cannot be changed in one experiment yet.",
        "Camber Rf: a left/right pair cannot be changed in one experiment yet.",
        "Spring Rate F: already at the end of its range.",
    ]


def test_a_car_that_stores_a_value_inverted_gets_the_click_the_other_way() -> None:
    passes = passes_of("slow", front_slip_mid_deg=7.0, rear_slip_mid_deg=3.0)
    inverted = [item("ARB_FRONT", "arb", "F", shown_scale=-1.0)]

    hypothesis = hypothesis_of(
        DiagnoseRequest(passes=passes, items=inverted), "mid_understeer.slow"
    )

    assert [(c.parameter, c.clicks) for c in hypothesis.candidates] == [("ARB_FRONT", 1)]


def test_brake_bias_rearward_is_not_proposed_while_the_rears_lock() -> None:
    passes = passes_of("slow", front_slip_entry_deg=6.0, rear_slip_entry_deg=3.0, rear_lockups=1.0)

    _, hypotheses = diagnose(DiagnoseRequest(passes=passes, items=car()))
    by_key = {h.key: h for h in hypotheses}

    assert "BRAKE_BIAS" not in [c.parameter for c in by_key["entry_understeer.all"].candidates]
    assert (
        "Brake Bias: ruled out by another symptom (lock_r)."
        in by_key["entry_understeer.all"].not_testable
    )
    assert [(c.parameter, c.clicks) for c in by_key["lock_r.all"].candidates] == [("BRAKE_BIAS", 1)]
    assert by_key["lock_r.all"].phase.value == "brake"


def test_caught_slides_on_exit_are_oversteer_even_with_balanced_slip() -> None:
    passes = passes_of(
        "medium", front_slip_exit_deg=3.0, rear_slip_exit_deg=3.1, countersteer_exit_s=0.3
    )

    hypothesis = hypothesis_of(DiagnoseRequest(passes=passes, items=[]), "exit_oversteer.all")

    assert hypothesis.evidence == ["countersteer_exit_s.all"]
    assert hypothesis.score == pytest.approx(0.3 / 0.12)
    assert hypothesis.not_testable == [
        "This session has no setup data, so no item of the car can be named."
    ]


def test_no_passes_is_an_empty_diagnosis() -> None:
    assert diagnose(DiagnoseRequest(passes=[], items=car())) == ([], [])


slip = st.floats(min_value=0.0, max_value=15.0, allow_nan=False)


@settings(max_examples=60, deadline=None)
@given(
    readings=st.lists(
        st.tuples(st.sampled_from(["slow", "medium", "fast"]), slip, slip, slip, slip),
        max_size=12,
    )
)
def test_whatever_the_passes_every_reference_is_in_the_evidence_pack(
    readings: list[tuple[str, float, float, float, float]],
) -> None:
    passes = [
        one
        for speed_class, front, rear, saturation, events in readings
        for one in passes_of(
            speed_class,
            count=3,
            front_slip_mid_deg=front,
            rear_slip_mid_deg=rear,
            front_slip_entry_deg=rear,
            rear_slip_entry_deg=front,
            front_saturation_index=saturation / 15.0,
            front_lockups=events / 15.0,
        )
    ]

    pack, hypotheses = diagnose(DiagnoseRequest(passes=passes, items=car()))
    ids = {e.id for e in pack}

    assert len({h.key for h in hypotheses}) == len(hypotheses)
    for hypothesis in hypotheses:
        assert set(hypothesis.evidence) <= ids
        for alternative in hypothesis.alternatives:
            assert set(alternative.evidence) <= ids
            assert alternative.supported == bool(alternative.evidence)
