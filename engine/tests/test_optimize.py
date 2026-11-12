"""Candidate ranking, on the simulated car's sensitivity profile."""

from typing import Any

import pytest
from fastapi.testclient import TestClient
from sensitivity_data import ROPE, candidate, measurement, request_of, sweep, transfer
from test_experiments import TIME

from paddock_engine.app import create_app
from paddock_engine.contracts.proposals_schema import Basis, Proposals, Stage
from paddock_engine.contracts.propose_request_schema import ProposeRequest
from paddock_engine.contracts.sensitivity_response_schema import (
    EffectClass,
    SensitivityEffect,
    SensitivityResponse,
)
from paddock_engine.optimize.rank import Belief, expected_gain, laps_needed, propose
from paddock_engine.sensitivity.profile import update


def proposals(
    effects: list[SensitivityEffect], candidates: list[dict[str, Any]], experiments: int = 7
) -> Proposals:
    return propose(
        ProposeRequest.model_validate(
            {
                "candidates": candidates,
                "effects": [e.model_dump(mode="json") for e in effects],
                "channel": TIME,
                "better": "lower",
                "rope": ROPE,
                "sigma": 0.028,
                "corners": 17,
                "laps_per_block": 3,
                "experiments": experiments,
            }
        )
    )


def test_acceptance_a_knob_measured_as_negligible_is_not_proposed() -> None:
    """Phase 5 acceptance. The rear bar was measured on this car and does nothing: it is
    left out with the reason. The front bar helps one way and hurts the other. A knob nobody
    has measured is proposed because nothing is known about it."""
    candidates = [
        candidate("ARB_REAR", 1),
        candidate("ARB_REAR", -1),
        candidate("ARB_FRONT", 1),
        candidate("ARB_FRONT", -1),
        candidate("CAMBER_F", -1),
    ]

    result = proposals(sweep().effects, candidates)

    assert [(p.parameter, p.clicks, p.basis) for p in result.ranked] == [
        ("ARB_FRONT", 1, Basis.measured),
        ("CAMBER_F", -1, Basis.none),
    ]
    reasons = {(s.parameter, s.clicks): s.reason for s in result.skipped}
    assert "negligible" in reasons[("ARB_REAR", 1)] and "negligible" in reasons[("ARB_REAR", -1)]
    assert "worse" in reasons[("ARB_FRONT", -1)]
    best = result.ranked[0]
    assert best.gain_low <= 0.020 <= best.gain_high
    assert best.value_per_lap == pytest.approx(best.expected_gain / best.laps_needed)
    assert result.stage is Stage.learning


def test_acceptance_a_negligible_prior_does_not_rule_a_knob_out() -> None:
    """Phase 5 acceptance, the "unless uncertainty justifies it" half: another car ignoring
    its rear bar is not a measurement of this one."""
    borrowed = transfer("ARB_REAR", 1, 0.0, ci_low=-0.001, ci_high=0.001)
    profile = update(request_of([], [borrowed]))
    assert profile.effects[0].effect_class is EffectClass.negligible

    result = proposals(profile.effects, [candidate("ARB_REAR", 1)], experiments=0)

    assert result.skipped == []
    (kept,) = result.ranked
    assert kept.basis is Basis.prior and kept.prior_source == "Formula Alpha 2025"
    assert result.stage is Stage.cold_start


def test_an_unknown_knob_is_worth_more_than_one_known_to_do_little() -> None:
    known_small = Belief(gain=0.002, sd=0.001, basis=Basis.measured)
    unknown = Belief(gain=0.0, sd=0.015, basis=Basis.none)
    known_good = Belief(gain=0.020, sd=0.002, basis=Basis.measured)

    assert expected_gain(known_small, ROPE) < expected_gain(unknown, ROPE)
    assert expected_gain(unknown, ROPE) < expected_gain(known_good, ROPE)
    assert expected_gain(known_good, ROPE) == pytest.approx(0.015, abs=1e-4)
    assert expected_gain(Belief(0.02, 0.0, Basis.measured), ROPE) == pytest.approx(0.015)


def test_a_small_expected_effect_needs_more_laps_to_decide() -> None:
    request = ProposeRequest.model_validate(
        {
            "candidates": [],
            "effects": [],
            "channel": TIME,
            "better": "lower",
            "rope": ROPE,
            "sigma": 0.028,
            "corners": 17,
            "laps_per_block": 3,
            "experiments": 0,
        }
    )

    big = laps_needed(Belief(0.040, 0.005, Basis.measured), request)
    small = laps_needed(Belief(0.004, 0.005, Basis.measured), request)
    unknown = laps_needed(Belief(0.0, 0.015, Basis.none), request)

    assert big == 4 * 3 + 4, "never fewer than the standard plan with its out-laps"
    assert small > big
    assert unknown == small, "both are sized to decide an effect of two bands"


def test_the_endpoints_speak_the_contracts() -> None:
    client = TestClient(create_app())
    measurements = [measurement(1, "ARB_FRONT", 1, -0.020)]

    profile = client.post(
        "/sensitivity/update", json=request_of(measurements).model_dump(mode="json")
    )
    ranked = client.post(
        "/propose",
        json={
            "candidates": [candidate("ARB_FRONT", 1)],
            "effects": profile.json()["effects"],
            "channel": TIME,
            "better": "lower",
            "rope": ROPE,
            "sigma": 0.028,
            "corners": 17,
            "laps_per_block": 3,
            "experiments": 25,
        },
    )

    assert profile.status_code == 200 and ranked.status_code == 200
    SensitivityResponse.model_validate(profile.json())
    body = Proposals.model_validate(ranked.json())
    assert body.stage is Stage.optimizing
    assert [p.parameter for p in body.ranked] == ["ARB_FRONT"]
