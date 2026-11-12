"""The sensitivity profile, on a simulated car with known effects."""

import math

import pytest
from sensitivity_data import (
    LEVEL,
    TRUE_EFFECTS,
    measurement,
    request_of,
    row,
    sweep,
    transfer,
)

from paddock_engine.contracts.sensitivity_request_schema import Transfer
from paddock_engine.contracts.sensitivity_response_schema import EffectClass
from paddock_engine.sensitivity import priors
from paddock_engine.sensitivity.pooling import Estimate, from_interval, pool
from paddock_engine.sensitivity.profile import MODEL_VERSION, update


def test_acceptance_known_effects_are_recovered_within_their_intervals() -> None:
    """Phase 5 acceptance: a linear, a negligible and a non-linear item."""
    assert len(sweep().effects) == len(TRUE_EFFECTS)
    for (parameter, clicks), truth in TRUE_EFFECTS.items():
        effect = row(sweep(), parameter, clicks)
        assert effect.ci_low <= truth <= effect.ci_high, (parameter, clicks, effect)
        assert not effect.is_prior and effect.prior_source == ""
        assert len(effect.experiment_ids) == 1 and effect.n_passes > 0

    assert row(sweep(), "ARB_FRONT", 1).effect_class is EffectClass.negative
    assert row(sweep(), "ARB_FRONT", -1).effect_class is EffectClass.positive
    assert not any(e.nonlinear for e in sweep().effects if e.parameter == "ARB_FRONT")

    for clicks in (1, -1):
        assert row(sweep(), "ARB_REAR", clicks).effect_class is EffectClass.negligible
        assert not row(sweep(), "ARB_REAR", clicks).nonlinear

    for clicks in (1, -1):
        wing = row(sweep(), "WING_REAR", clicks)
        assert wing.effect_class is EffectClass.positive
        assert wing.nonlinear and "not mirror images" in wing.note
    assert sweep().model_version == MODEL_VERSION


def test_two_clicks_that_do_not_double_one_click_are_non_linear() -> None:
    saturating = [
        measurement(1, "SPRING_F", 1, -0.020),
        measurement(2, "SPRING_F", 2, -0.020),  # the second click adds nothing
    ]

    profile = update(request_of(saturating))

    assert all(e.nonlinear for e in profile.effects)
    assert "do not do twice" in row(profile, "SPRING_F", 2).note


def test_repeated_experiments_are_pooled_into_a_narrower_interval() -> None:
    first, second = measurement(1, "ARB_FRONT", 1, -0.020), measurement(2, "ARB_FRONT", 1, -0.020)

    pooled = update(request_of([first, second])).effects[0]

    assert pooled.experiment_ids == [1, 2]
    assert pooled.n_passes == first["n_passes"] + second["n_passes"]
    narrowest = min(m["ci_high"] - m["ci_low"] for m in (first, second))
    assert pooled.ci_high - pooled.ci_low < narrowest
    assert min(first["estimate"], second["estimate"]) <= pooled.estimate
    assert pooled.estimate <= max(first["estimate"], second["estimate"])


def test_pooling_weights_by_precision() -> None:
    precise, vague = Estimate(mean=1.0, se=0.1), Estimate(mean=3.0, se=1.0)

    pooled = pool([precise, vague])

    assert pooled.mean == pytest.approx((100 * 1.0 + 1 * 3.0) / 101)
    assert pooled.se == pytest.approx(math.sqrt(1 / 101))
    assert from_interval(0.0, -1.645, 1.645, 0.90).se == pytest.approx(1.0, abs=1e-3)
    with pytest.raises(ValueError, match="nothing to pool"):
        pool([])


def test_acceptance_a_transferred_effect_is_shown_as_a_prior_never_as_measured() -> None:
    """Phase 5 acceptance: shrunk towards zero, widened, flagged, and named after its source."""
    profile = update(request_of([], [transfer("ARB_FRONT", 1, -0.030)]))

    (prior,) = profile.effects
    assert prior.is_prior and prior.prior_source == "Formula Alpha 2025"
    assert prior.experiment_ids == [] and prior.n_passes == 0
    assert "not measured on this car" in prior.note
    assert prior.estimate == pytest.approx(-0.015)  # half of what the other car showed
    assert prior.ci_low < -0.030 < prior.ci_high, "what the other car showed stays possible"
    assert prior.ci_low < 0.0 < prior.ci_high, "and so does no effect at all"
    assert prior.effect_class is EffectClass.inconclusive


def test_the_cars_own_measurement_replaces_a_prior() -> None:
    own = measurement(1, "ARB_FRONT", 1, -0.020)
    borrowed = [transfer("ARB_FRONT", 1, -0.060), transfer("ARB_FRONT", -1, 0.060)]

    profile = update(request_of([own], borrowed))

    assert [(e.clicks, e.is_prior) for e in profile.effects] == [(1, False), (-1, True)]
    assert row(profile, "ARB_FRONT", 1).estimate == pytest.approx(own["estimate"])


def test_a_less_similar_car_is_trusted_less() -> None:
    same = priors.prior(Transfer.model_validate(transfer("ARB_FRONT", 1, -0.030)), LEVEL)
    other_fields = transfer("ARB_FRONT", 1, -0.030, same_drivetrain=False, source_car="GT4")
    other = priors.prior(Transfer.model_validate(other_fields), LEVEL)

    assert abs(other.mean) < abs(same.mean)
    assert other.se > same.se
    both = [transfer("ARB_FRONT", 1, -0.030), other_fields]
    assert update(request_of([], both)).effects[0].prior_source == "Formula Alpha 2025"
