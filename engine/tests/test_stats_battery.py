"""A hybrid's battery charge as a covariate (ADR 0024): it goes through the same rule as fuel
(present on every pass, varying within corners, not entangled with the arm, ADR 0022)."""

from collections.abc import Sequence
from dataclasses import replace

import numpy as np
import pytest
from simulation import BASELINE, VARIANT, simulate_battery_abab

from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.observations import Observation

EFFECT_S = 0.010
BATTERY = "ers_charge_start"


def _without_battery(observations: Sequence[Observation]) -> list[Observation]:
    """The same passes as a car without KERS/ERS records them: no battery charge at all."""
    return [
        replace(o, covariates={k: v for k, v in o.covariates.items() if k != BATTERY})
        for o in observations
    ]


def _effect(observations: Sequence[Observation]) -> Effect:
    (effect,) = estimate_effects(observations, BASELINE, rope=0.005)
    return effect


def _width(effect: Effect) -> float:
    return effect.ci_high - effect.ci_low


@pytest.mark.parametrize("seed", range(8))
def test_adjusting_for_the_battery_narrows_the_interval(seed: int) -> None:
    observations = simulate_battery_abab(np.random.default_rng(seed), effect=EFFECT_S)

    adjusted = _effect(observations)
    unadjusted = _effect(_without_battery(observations))

    assert adjusted.covariates == [BATTERY] and not adjusted.dropped_covariates
    assert _width(adjusted) < 0.5 * _width(unadjusted)
    assert adjusted.ci_low < EFFECT_S < adjusted.ci_high


def test_a_car_without_a_battery_has_nothing_to_adjust_for() -> None:
    observations = _without_battery(simulate_battery_abab(np.random.default_rng(7), EFFECT_S))

    effect = _effect(observations)

    assert effect.covariates == [] and effect.dropped_covariates == []


def test_a_battery_that_never_changes_is_not_adjusted_for() -> None:
    """A hybrid that never deploys (or is always full) records a constant charge."""
    observations = _without_battery(simulate_battery_abab(np.random.default_rng(8), EFFECT_S))
    constant = [replace(o, covariates={**o.covariates, BATTERY: 1.0}) for o in observations]

    assert _effect(constant) == _effect(observations)


def test_passes_recorded_before_the_battery_was_leave_it_out() -> None:
    """An agent updated mid-experiment: the earlier passes have no charge, so none is used."""
    observations = simulate_battery_abab(np.random.default_rng(9), EFFECT_S)
    first_lap_without = _without_battery(observations[:12]) + observations[12:]

    assert _effect(first_lap_without) == _effect(_without_battery(observations))


def test_a_charge_that_follows_the_setup_is_left_out_and_named() -> None:
    """A setup that harvests more leaves the variant on a fuller battery: then the charge cannot
    be told from the arm and adjusting for it would remove part of the setup's own effect."""
    observations = simulate_battery_abab(np.random.default_rng(10), EFFECT_S)
    rng = np.random.default_rng(11)
    tangled = [
        replace(
            o,
            covariates={
                **o.covariates,
                BATTERY: 0.5 + 0.3 * (o.arm == VARIANT) + rng.normal(0.0, 0.01),
            },
        )
        for o in observations
    ]

    effect = _effect(tangled)

    assert effect.dropped_covariates == [BATTERY]
    assert effect.covariates == []
