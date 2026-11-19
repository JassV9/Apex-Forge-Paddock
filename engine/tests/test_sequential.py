"""Pre-registered looks at 8, 10 and 12 blocks with O'Brien-Fleming spending (ADR 0029)."""

from functools import cache

import numpy as np
import pytest
from block_simulation import Design, as_passes, simulate
from scipy import stats

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import (
    Decision,
    EvaluateResponse,
    FindingClass,
    Recommendation,
)
from paddock_engine.experiments.evaluate import evaluate
from paddock_engine.stats.observations import Observation
from paddock_engine.stats.sequential import boundaries, interval_level, look_of

MARGIN = 0.005
TEST = {"channel": "corner_time_s", "rope": MARGIN, "level": 0.9, "better": "lower"}
TRIALS = 400


def test_boundaries_spend_five_percent_over_three_correlated_looks() -> None:
    bounds = boundaries(0.05)
    rng = np.random.default_rng(29)
    increments = rng.normal(size=(1_000_000, 3)) * np.sqrt([8.0, 2.0, 2.0])
    looks = np.cumsum(increments, axis=1) / np.sqrt([8.0, 10.0, 12.0])
    crossed = looks > np.array(bounds)

    assert bounds == pytest.approx((2.1351, 1.9260, 1.7523), abs=5e-4)
    assert crossed.any(axis=1).mean() == pytest.approx(0.05, abs=1e-3)
    first = crossed[:, 0].mean()
    assert first == pytest.approx(2 - 2 * stats.norm.cdf(1.959964 / np.sqrt(2 / 3)), abs=5e-4)


def test_looks_and_their_levels() -> None:
    assert [look_of(blocks) for blocks in (4, 6, 7, 8, 9, 10, 12, 14)] == [0, 0, 0, 1, 1, 2, 3, 3]
    levels = [interval_level(look, 0.9) for look in range(4)]
    assert levels[0] == 0.9
    assert levels[1] > levels[2] > levels[3] > 0.9  # the first look is the most demanding


def _without_classes(observations: list[Observation]) -> list[dict[str, object]]:
    return [item | {"speed_class": None} for item in as_passes(observations)]


def _judge(passes: list[dict[str, object]], blocks: int, extensions_left: int) -> EvaluateResponse:
    request = {
        "success_test": TEST,
        "guardrails": [],
        "predictions": [],
        "driver_channels": [],
        "passes": [p for p in passes if int(p["block"]) <= blocks],  # type: ignore[call-overload]
        "look": look_of(blocks),
        "extensions_left": extensions_left,
    }
    return evaluate(EvaluateRequest.model_validate(request))


@cache
def _band_edge_runs() -> list[list[EvaluateResponse]]:
    """Verdicts at 8, 10 and 12 blocks of experiments whose true gain is exactly the margin."""
    rng = np.random.default_rng(2029)
    runs = []
    for _ in range(TRIALS):
        passes = _without_classes(simulate(rng, Design(-MARGIN, blocks=12)))
        runs.append([_judge(passes, blocks, left) for blocks, left in ((8, 2), (10, 1), (12, 0))])
    return runs


def _accepted(verdicts: list[EvaluateResponse], extend: bool | None) -> bool:
    """Accepted at any look reached; `extend` None follows the recommendation (training)."""
    for verdict in verdicts:
        if verdict.finding_class is FindingClass.confirmed_improvement:
            return True
        go_on = verdict.recommendation is Recommendation.continue_ if extend is None else extend
        if verdict.decision is not Decision.inconclusive or not go_on:
            return False
    return False


def test_extending_by_the_rules_accepts_at_the_band_edge_at_most_five_percent() -> None:
    runs = _band_edge_runs()
    training = np.mean([_accepted(verdicts, extend=None) for verdicts in runs])
    human = np.mean([_accepted(verdicts, extend=True) for verdicts in runs])
    single = np.mean([v[0].finding_class is FindingClass.confirmed_improvement for v in runs])

    assert single <= training <= human <= 0.05


def test_a_change_without_promise_is_not_driven_further() -> None:
    """Futility: once an improvement beyond the margin is ruled out, the look says stop."""
    passes = _without_classes(simulate(np.random.default_rng(5), Design(+0.012)))

    verdict = _judge(passes, 8, extensions_left=2)

    assert verdict.recommendation in (Recommendation.stop_neutral, Recommendation.switch_metric)
    assert verdict.decision is not Decision.accepted
