"""Candidate ranking, on the simulated car's sensitivity profile."""

from typing import Any

import pytest
from fastapi.testclient import TestClient
from sensitivity_data import (
    ROPE,
    candidate,
    measurement,
    request_of,
    row,
    sweep,
    synthetic,
    transfer,
)
from test_experiments import TIME

from paddock_engine.app import create_app
from paddock_engine.contracts.proposals_schema import Basis, Proposals, Stage
from paddock_engine.contracts.propose_request_schema import ProposeRequest
from paddock_engine.contracts.sensitivity_response_schema import (
    SensitivityEffect,
    SensitivityPrior,
    SensitivityResponse,
)
from paddock_engine.optimize.rank import Belief, expected_gain, laps_needed, propose
from paddock_engine.sensitivity.profile import update


def proposals(
    effects: list[SensitivityEffect],
    candidates: list[dict[str, Any]],
    experiments: int = 7,
    priors: list[SensitivityPrior] | None = None,
) -> Proposals:
    return propose(
        ProposeRequest.model_validate(
            {
                "candidates": candidates,
                "effects": [e.model_dump(mode="json", exclude_none=True) for e in effects],
                "priors": [p.model_dump(mode="json") for p in priors or []],
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


def test_a_single_experiment_ranks_a_knob_by_its_narrow_belief_never_rules_it_out() -> None:
    """One experiment per change is never a pooled claim (ADR 0033): the rear bar, measured once
    as doing nothing, is ranked last instead of skipped; the front bar's good direction first,
    the unmeasured knob in between because nothing is known about it."""
    candidates = [
        candidate("ARB_REAR", 1),
        candidate("ARB_FRONT", 1),
        candidate("ARB_FRONT", -1),
        candidate("CAMBER_F", -1),
    ]

    result = proposals(sweep().effects, candidates)

    assert [(p.parameter, p.clicks, p.basis) for p in result.ranked[:2]] == [
        ("ARB_FRONT", 1, Basis.measured),
        ("CAMBER_F", -1, Basis.none),
    ]
    assert result.skipped == []
    best, front = result.ranked[0], row(sweep(), "ARB_FRONT", 1)
    assert (best.gain_low, best.gain_high) == pytest.approx((-front.ci_high, -front.ci_low))
    assert best.value_per_lap == pytest.approx(best.expected_gain / best.laps_needed)
    assert result.stage is Stage.learning


def test_acceptance_a_knob_pooled_neutral_or_confirmed_worse_is_not_proposed() -> None:
    """Phase 5 acceptance with ADR 0033's levels: three experiments each."""
    neutral = [
        synthetic(i, "ARB_REAR", 1, e, 0.0008) for i, e in ((1, 0.001), (2, -0.0005), (3, 0.0))
    ]
    worse = [synthetic(i, "ARB_FRONT", -1, 0.02, 0.002) for i in (4, 5, 6)]
    profile = update(request_of(neutral + worse))

    result = proposals(profile.effects, [candidate("ARB_REAR", 1), candidate("ARB_FRONT", -1)])

    assert result.ranked == []
    reasons = {(s.parameter, s.clicks): s.reason for s in result.skipped}
    assert "neutral" in reasons[("ARB_REAR", 1)] and "3 experiments" in reasons[("ARB_REAR", 1)]
    assert "worse" in reasons[("ARB_FRONT", -1)]


def test_acceptance_a_neutral_looking_prior_does_not_rule_a_knob_out() -> None:
    """Another car ignoring its rear bar is not a measurement of this one; and the prior is read
    from its own list only."""
    borrowed = transfer("ARB_REAR", 1, 0.0, standard_error=0.0005)
    profile = update(request_of([], [borrowed]))
    assert profile.effects == []

    result = proposals(profile.effects, [candidate("ARB_REAR", 1)], 0, profile.priors)

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
            "priors": [],
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
            "priors": profile.json()["priors"],
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
