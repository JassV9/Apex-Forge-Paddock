"""The real AI experiments of 2026-11-17 and 18 (fixtures/real_experiments.csv.gz), judged again.

A/A on real data: inside one arm's blocks of an experiment of 8 or more blocks the setup never
changed, so relabelling half of them as the variant is an experiment whose truth is zero. The
replay checks what the new verdicts say about experiments 11, 12 and 15 to 19; the ledger keeps
what was decided then (ADR 0029).
"""

from functools import cache
from itertools import combinations
from typing import Any

import numpy as np
import pytest
import real_experiments as real

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import EvaluateResponse, FindingClass, Role
from paddock_engine.experiments.channel_effects import observations
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.stats.effects import Effect, estimate_effects
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.sequential import look_of

LONG = (12, 15, 16, 17, 18, 19)  # 8 or more blocks: 4 or more on each arm


def splits(experiment: int) -> list[list[dict[str, Any]]]:
    """Every split of one arm's blocks into two halves, each once, the halves as arms."""
    out = []
    passes = real.passes(experiment)
    for arm in ("baseline", "variant"):
        own = [p for p in passes if p["arm"] == arm]
        blocks = sorted({p["block"] for p in own})
        for fake in combinations(blocks, len(blocks) // 2):
            if blocks[0] in fake:
                continue
            out.append([p | {"arm": "variant" if p["block"] in fake else "baseline"} for p in own])
    return out


@cache
def real_aa() -> list[Effect | None]:
    """None where four blocks cannot carry the covariates that change between them (fuel in
    experiment 15, whose first block was driven on half a tank)."""
    effects: list[Effect | None] = []
    for experiment in LONG:
        for split in splits(experiment):
            rows = observations(
                EvaluateRequest.model_validate(real.request(experiment, passes=split)).passes,
                "corner_time_s",
            )
            try:
                effects.append(estimate_effects(rows, "baseline", rope=0.005)[0])
            except NotEstimableError:
                effects.append(None)
    return effects


def test_real_identical_setups_exclude_zero_about_as_often_as_ninety_percent_should() -> None:
    effects = [e for e in real_aa() if e is not None]
    excluded = np.mean([e.ci_low > 0 or e.ci_high < 0 for e in effects])

    assert len(real_aa()) >= 40 and len(effects) >= 30
    assert excluded <= 0.20  # nominal 10 %; 40-odd splits sharing blocks
    assert all(e.block_degrees >= 1 for e in effects)


@cache
def replay(experiment: int) -> EvaluateResponse:
    request = real.request(experiment)
    request["look"] = look_of(len({p["block"] for p in request["passes"]}))
    request["extensions_left"] = 0
    return evaluate(EvaluateRequest.model_validate(request))


# What the ledger stored for the primary effect, ms (the engine of the day, before ADR 0029).
LEDGER_ESTIMATE_MS = {11: -4.67, 12: -3.09, 15: -0.01, 17: 1.27, 18: 4.51, 19: 1.29}


@pytest.mark.parametrize("experiment", real.EXPERIMENTS)
def test_no_real_experiment_is_confirmed_and_the_estimates_stand(experiment: int) -> None:
    verdict = replay(experiment)
    primary = verdict.results[0]

    assert primary.role is Role.primary
    assert verdict.finding_class in (
        FindingClass.insufficient_evidence,
        FindingClass.context_dependent,
        FindingClass.likely_improvement,
        FindingClass.likely_regression,
        FindingClass.neutral,
    )
    if experiment in LEDGER_ESTIMATE_MS:  # one game session: the same covariates as then
        assert 1000 * primary.estimate == pytest.approx(LEDGER_ESTIMATE_MS[experiment], abs=0.01)


def test_rear_wing_minus_one_moved_the_rear_slip_but_not_the_time() -> None:
    """Experiment 15 (rear wing -1 on both linked items): corner time 0.0 ms, rear slip peak up
    by about 0.16 degrees, its interval clear of zero (investigation section 1)."""
    verdict = replay(15)
    (slip,) = [r for r in verdict.results if r.channel == "rear_slip_peak_deg"]

    assert abs(verdict.results[0].estimate) < 0.0005
    assert slip.ci_low > 0.05 and slip.estimate == pytest.approx(0.158, abs=0.01)


def test_the_restarted_experiment_is_judged_with_its_session_term() -> None:
    verdict = replay(16)

    assert verdict.look == 2
    assert any(c.name == "session_2" for c in verdict.confounders or [])
