"""The sensitivity profile, on a simulated car with known effects (ADR 0011, 0033)."""

import pytest
from sensitivity_data import (
    LEVEL,
    TRUE_EFFECTS,
    measurement,
    request_of,
    row,
    sweep,
    synthetic,
    transfer,
)

from paddock_engine.contracts.sensitivity_request_schema import Transfer
from paddock_engine.contracts.sensitivity_response_schema import (
    EffectClass,
    KnowledgeLevel,
    Method,
    SensitivityPrior,
)
from paddock_engine.sensitivity import priors
from paddock_engine.sensitivity.profile import MODEL_VERSION, update


def test_acceptance_known_effects_are_recovered_within_their_intervals() -> None:
    """Phase 5 acceptance, one experiment per change: a linear, a negligible and a non-linear
    item. One experiment is reported as it is and is never a pooled confirmation."""
    assert len(sweep().effects) == len(TRUE_EFFECTS)
    covered = 0
    for (parameter, clicks), truth in TRUE_EFFECTS.items():
        effect = row(sweep(), parameter, clicks)
        covered += effect.ci_low <= truth <= effect.ci_high
        assert abs(effect.estimate - truth) < 4 * effect.standard_error, (parameter, clicks)
        assert effect.method is Method.single and effect.experiments == 1
        assert effect.knowledge_level is not KnowledgeLevel.confirmed
        assert effect.knowledge_level is not KnowledgeLevel.neutral
        assert len(effect.experiment_ids) == 1 and effect.n_passes > 0
    assert covered >= len(TRUE_EFFECTS) - 2  # seven 90 % intervals: three misses is a 3 % event
    assert sweep().priors == [] and sweep().excluded == []

    front = row(sweep(), "ARB_FRONT", 1)
    assert front.effect_class is EffectClass.negative
    assert front.knowledge_level is KnowledgeLevel.preliminary
    assert row(sweep(), "ARB_FRONT", -1).effect_class is EffectClass.positive
    assert not any(e.nonlinear for e in sweep().effects if e.parameter == "ARB_FRONT")
    for clicks in (1, -1):
        rear = row(sweep(), "ARB_REAR", clicks)
        assert rear.knowledge_level is KnowledgeLevel.insufficient and not rear.nonlinear
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


def test_three_experiments_beyond_the_margin_are_confirmed_with_hksj() -> None:
    repeated = [measurement(i, "ARB_FRONT", 1, -0.020) for i in (1, 2, 3)]

    pooled = update(request_of(repeated)).effects[0]

    assert pooled.method is Method.hksj and pooled.experiments == 3
    assert pooled.knowledge_level is KnowledgeLevel.confirmed
    assert pooled.ci_low <= -0.020 <= pooled.ci_high
    assert pooled.pi_low is not None and pooled.pi_high is not None
    assert pooled.pi_low <= pooled.ci_low and pooled.ci_high <= pooled.pi_high
    assert pooled.p_increase > 0.99  # clearly a decrease
    assert pooled.engine_versions == ["0.2.0"]


def test_two_experiments_are_pooled_with_a_half_normal_tau_and_never_confirmed() -> None:
    first, second = measurement(1, "ARB_FRONT", 1, -0.020), measurement(2, "ARB_FRONT", 1, -0.020)

    pooled = update(request_of([first, second])).effects[0]

    assert pooled.method is Method.half_normal and pooled.experiment_ids == [1, 2]
    assert pooled.knowledge_level is KnowledgeLevel.preliminary
    assert pooled.n_passes == first["n_passes"] + second["n_passes"]
    assert min(first["estimate"], second["estimate"]) <= pooled.estimate
    assert pooled.estimate <= max(first["estimate"], second["estimate"])
    assert pooled.tau is not None and pooled.tau > 0


def test_three_tight_experiments_inside_the_margin_are_neutral_two_are_not() -> None:
    tight = [
        synthetic(i, "ARB_REAR", 1, e, 0.0008) for i, e in ((1, 0.001), (2, -0.0005), (3, 0.0))
    ]

    assert update(request_of(tight)).effects[0].knowledge_level is KnowledgeLevel.neutral
    two = update(request_of(tight[:2])).effects[0]
    assert two.knowledge_level is KnowledgeLevel.insufficient
    assert two.effect_class is EffectClass.negligible  # the interval is inside; the claim is not


def test_acceptance_a_transferred_effect_is_a_prior_in_its_own_list_never_a_measurement() -> None:
    """Shrunk towards zero, widened, named after its source, and without any class."""
    profile = update(request_of([], [transfer("ARB_FRONT", 1, -0.030)]))

    assert profile.effects == []
    (prior,) = profile.priors
    assert prior.source_car == "Formula Alpha 2025" and "not measured on this car" in prior.note
    assert prior.estimate == pytest.approx(-0.015)  # half of what the other car showed
    assert prior.ci_low < -0.030 < prior.ci_high, "what the other car showed stays possible"
    assert prior.ci_low < 0.0 < prior.ci_high, "and so does no effect at all"
    assert not {"effect_class", "knowledge_level", "experiment_ids"} & set(
        SensitivityPrior.model_fields
    )


def test_the_cars_own_measurement_in_any_stratum_replaces_a_prior() -> None:
    own = measurement(1, "ARB_FRONT", 1, -0.020) | {"track": "fn_spa/layout_f1_2026"}
    borrowed = [transfer("ARB_FRONT", 1, -0.060), transfer("ARB_FRONT", -1, 0.060)]

    profile = update(request_of([own], borrowed))

    assert [(e.clicks, e.track) for e in profile.effects] == [(1, "fn_spa/layout_f1_2026")]
    assert [p.clicks for p in profile.priors] == [-1]


def test_a_less_similar_car_is_trusted_less() -> None:
    same = priors.prior(Transfer.model_validate(transfer("ARB_FRONT", 1, -0.030)), LEVEL)
    other_fields = transfer("ARB_FRONT", 1, -0.030, same_drivetrain=False, source_car="GT4")
    other = priors.prior(Transfer.model_validate(other_fields), LEVEL)

    assert abs(other.estimate) < abs(same.estimate)
    assert other.standard_error > same.standard_error
    both = [transfer("ARB_FRONT", 1, -0.030), other_fields]
    assert update(request_of([], both)).priors[0].source_car == "Formula Alpha 2025"
