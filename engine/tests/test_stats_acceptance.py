"""Phase 2 acceptance: the verdict statistics on simulated experiments.

The v0 rules called two identical setups a regression in 39 % of comparisons
(tools/verdict_noise_sim.py). These tests hold Paddock to the brief's numbers:

- identical setups, sigma 28 ms, 17 corners: a decision in at most 10 % of experiments;
- a 20 ms per corner effect, 6 passes per arm in each of 10 corners: detected at least 80 %.

Two words, two meanings:

- "detected": the 90 % interval excludes zero on the right side. The direction is established.
  This is what the brief's MDE formula describes.
- "decision": the 90 % interval lies entirely outside the +-5 ms equivalence band. This is what
  an experiment needs to be accepted or rejected, and it takes a larger effect or more laps.

The seeds are fixed, so the rates below are exact, not flaky.
"""

import numpy as np
import pytest
from simulation import BASELINE, simulate_abab

from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.rope import EffectClass

ROPE_S = 0.005
TRIALS = 200
DECISIONS = (EffectClass.POSITIVE, EffectClass.NEGATIVE)


def run_trials(seed: int, effect: float, corners: int, **design: float) -> list[Effect]:
    rng = np.random.default_rng(seed)
    return [
        estimate_effects(
            simulate_abab(rng, effect, corners, **design),  # type: ignore[arg-type]
            BASELINE,
            rope=ROPE_S,
            replicates=200,
        )[0]
        for _ in range(TRIALS)
    ]


def rate(effects: list[Effect], classes: tuple[EffectClass, ...]) -> float:
    return sum(effect.effect_class in classes for effect in effects) / len(effects)


def detected(effects: list[Effect], direction: int) -> float:
    """Share of experiments whose interval excludes zero on the given side."""
    if direction > 0:
        return sum(effect.ci_low > 0 for effect in effects) / len(effects)
    return sum(effect.ci_high < 0 for effect in effects) / len(effects)


def coverage(effects: list[Effect], truth: float) -> float:
    return sum(effect.ci_low <= truth <= effect.ci_high for effect in effects) / len(effects)


def test_identical_setups_rarely_produce_a_decision() -> None:
    effects = run_trials(seed=1, effect=0.0, corners=17)

    assert rate(effects, DECISIONS) <= 0.10
    assert detected(effects, direction=+1) + detected(effects, direction=-1) <= 0.15


def test_a_20_ms_effect_is_detected_with_six_passes_in_ten_corners() -> None:
    effects = run_trials(seed=2, effect=0.020, corners=10)

    assert detected(effects, direction=+1) >= 0.80
    assert rate(effects, (EffectClass.NEGATIVE,)) == 0.0
    # Clearing the 5 ms band as well is harder: about 7 in 10 at this budget. The planner must
    # show this before driving, and an experiment that falls short is extended, not accepted.
    assert 0.60 <= rate(effects, (EffectClass.POSITIVE,)) < detected(effects, direction=+1)


@pytest.mark.parametrize(
    ("design", "truth"),
    [
        ({"laps_per_block": 3}, 0.0),
        ({"laps_per_block": 2}, 0.020),
        ({"laps_per_block": 3, "lap_sigma": 0.015}, 0.0),
    ],
    ids=["3 laps per block", "2 laps per block", "noise shared within a lap"],
)
def test_ninety_percent_intervals_cover_the_truth_about_ninety_percent(
    design: dict[str, float], truth: float
) -> None:
    effects = run_trials(seed=3, effect=truth, corners=12, **design)

    assert 0.84 <= coverage(effects, truth) <= 0.97


def test_session_drift_is_not_mistaken_for_a_setup_effect() -> None:
    """The track gaining 10 ms per lap must not look like the later-driven setup being faster."""
    effects = run_trials(seed=4, effect=0.0, corners=17, drift_per_lap=-0.010)

    assert rate(effects, DECISIONS) <= 0.10
    assert abs(float(np.mean([effect.estimate for effect in effects]))) < 0.003
