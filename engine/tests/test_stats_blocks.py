"""Block-level intervals on experiments with known truth (ADR 0029).

Identical setups (A/A) with block shocks of 0, 3 and 6 ms at 4, 8 and 12 blocks: the 90 %
interval covers 0 in 88 to 92 % of experiments, to within the simulation's own error (1000
experiments each: one standard error is 0.95 points, so the band checked is 87 to 93 %; the
fixed seeds make the rates exact), and a confirmed finding is made in at most 5 %. The
laps-are-independent interval of ADR 0008 (laps - blocks - 1 degrees of freedom) is shown
failing at 6 ms.
"""

from dataclasses import replace
from functools import cache

import numpy as np
import pytest
from block_simulation import BASELINE, Design, simulate

from paddock_engine.stats.block_level import arm_error, cell_means, corner_residuals, row_arms
from paddock_engine.stats.effects import Effect, estimate_effects, t_quantile, weighted_fit
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.observations import Observation, build_design
from paddock_engine.stats.sequential import interval_level, look_of

ROPE = 0.005
TRIALS = 1000


def lap_level_half_width(observations: list[Observation]) -> float:
    """ADR 0008's view: laps are the independent units, laps - blocks - 1 degrees of freedom."""
    design = build_design(observations, BASELINE)
    weights, fit = weighted_fit(design)
    arms = row_arms(design)
    keys = [(o.lap or 0, int(arm)) for o, arm in zip(observations, arms, strict=True)]
    means = cell_means(corner_residuals(design, fit.coefficients), weights[design.groups], keys)
    cells = list(means)
    outcome = np.array([means[cell] for cell in cells])
    regressors = np.column_stack([np.ones(len(cells)), [float(arm) for _, arm in cells]])
    _, error, _ = arm_error(outcome, regressors)
    blocks = len({o.block for o in observations})
    return t_quantile(0.9, len(cells) - blocks - 1) * error


@cache
def trials(blocks: int, shock_ms: int) -> tuple[list[Effect], list[float]]:
    rng = np.random.default_rng(1000 * blocks + shock_ms)
    effects, lap_level = [], []
    for _ in range(TRIALS):
        observations = simulate(rng, Design(blocks=blocks, block_sd=shock_ms / 1000))
        effects.append(estimate_effects(observations, BASELINE, rope=ROPE)[0])
        lap_level.append(abs(effects[-1].estimate) <= lap_level_half_width(observations))
    return effects, [float(hit) for hit in lap_level]


def coverage(effects: list[Effect]) -> float:
    return float(np.mean([e.ci_low <= 0.0 <= e.ci_high for e in effects]))


def false_decisions(effects: list[Effect]) -> float:
    """Confirmed findings: the look's repeated confidence interval beyond the band, at 8 blocks or
    more. Before the first look nothing is confirmed, whatever the interval."""
    decided = 0
    for e in effects:
        look = look_of(e.blocks)
        stretch = t_quantile(interval_level(look, 0.9), e.block_degrees) / t_quantile(
            0.9, e.block_degrees
        )
        half_width = stretch * (e.ci_high - e.estimate)
        decided += look >= 1 and abs(e.estimate) - half_width > ROPE
    return decided / len(effects)


def beyond_the_band(effects: list[Effect]) -> float:
    return float(np.mean([e.ci_low > ROPE or e.ci_high < -ROPE for e in effects]))


@pytest.mark.parametrize("blocks", [4, 8, 12])
@pytest.mark.parametrize("shock_ms", [0, 3, 6])
def test_identical_setups_are_covered_at_ninety_percent(blocks: int, shock_ms: int) -> None:
    effects, _ = trials(blocks, shock_ms)

    assert 0.87 <= coverage(effects) <= 0.93
    assert false_decisions(effects) <= 0.05
    assert beyond_the_band(effects) <= 0.06  # 4 blocks, 6 ms: 5.3 %, never confirmed
    assert all(e.block_degrees == blocks - 3 for e in effects)  # time costs one
    assert all(e.block_varying == ["time_since_start_s"] for e in effects)


@pytest.mark.parametrize("blocks", [8, 12])
def test_laps_as_independent_units_undercover_block_shocks(blocks: int) -> None:
    _, calm = trials(blocks, 0)
    _, shaken = trials(blocks, 6)

    assert np.mean(calm) >= 0.86
    assert np.mean(shaken) < 0.82


def test_a_restart_is_absorbed_by_the_session_term() -> None:
    """Blocks 4 to 8 in a new session that runs 15 ms slower everywhere. The new session holds
    3 variant and 2 baseline blocks, the first 1 and 2: without the term (the clock restarting
    at 0, as before ADR 0029) a quarter of the step lands on the arm."""
    restart = Design(restart_before=4, restart_step=0.015)
    rng = np.random.default_rng(44)
    effects = [estimate_effects(simulate(rng, restart), BASELINE, rope=ROPE)[0] for _ in range(200)]
    rng = np.random.default_rng(44)
    clock = Design(restart_before=4, restart_step=0.015, session_term=False)
    without = [estimate_effects(simulate(rng, clock), BASELINE, rope=ROPE)[0] for _ in range(200)]

    assert all("session_2" in e.covariates and "session_2" in e.block_varying for e in effects)
    assert 0.86 <= coverage(effects) <= 0.95
    assert abs(np.mean([e.estimate for e in effects])) < 0.002
    assert np.mean([e.estimate for e in without]) > 0.003


def test_a_session_that_is_one_arm_cannot_be_judged() -> None:
    """Every variant block in a session of its own: the session term and the arm cannot be
    told apart. The term is never dropped to make the fit work; the effect is flagged."""
    observations = [
        replace(o, covariates={**o.covariates, "session_2": float(o.arm != BASELINE)})
        for o in simulate(np.random.default_rng(3), Design())
    ]

    assert build_design(observations, BASELINE).session_confounded
    with pytest.raises(NotEstimableError, match="sessions"):
        estimate_effects(observations, BASELINE, ROPE)


def test_a_restart_with_both_arms_in_each_session_is_judged() -> None:
    observations = simulate(np.random.default_rng(3), Design(restart_before=8, restart_step=0.015))

    (effect,) = estimate_effects(observations, BASELINE, ROPE)

    assert not build_design(observations, BASELINE).session_confounded
    assert "session_2" in effect.covariates and effect.block_degrees == 4
