"""The base setup's multiplicity control and shrinkage, with known truth (ADR 0033, finding I13).

Each simulated car has eight items, each measured by one experiment (or three) with a block-level
standard error of 3 ms; the profile is pooled by the real engine and the base setup chosen from
it. Corner time, lower is better.
"""

from typing import Any

import numpy as np
from sensitivity_data import request_of, synthetic

from paddock_engine.contracts.base_setup_request_schema import BaseSetupRequest
from paddock_engine.contracts.base_setup_schema import BaseSetup
from paddock_engine.optimize.base_setup import base_setup
from paddock_engine.optimize.fdr import benjamini_hochberg, prior_variance, shrink
from paddock_engine.sensitivity.profile import update

ITEMS = [f"ITEM_{n}" for n in range(8)]
SE = 0.003


def car(rng: np.random.Generator, truths: list[float], per_item: int, df: int) -> BaseSetup:
    measurements = []
    for index, (name, truth) in enumerate(zip(ITEMS, truths, strict=True)):
        for repeat in range(per_item):
            reported = SE * np.sqrt(rng.chisquare(df) / df)
            estimate = float(rng.normal(truth, SE))
            measurements.append(
                synthetic(10 * index + repeat + 1, name, 1, estimate, reported, df=df)
            )
    profile = update(request_of(measurements))
    effects = [e.model_dump(mode="json", exclude_none=True) for e in profile.effects]
    return base_setup(BaseSetupRequest.model_validate(request(effects)))


def request(effects: list[dict[str, Any]]) -> dict[str, Any]:
    items = [
        {"name": n, "label": n, "min": 0, "max": 6, "step": 1, "value": 3, "paired": False}
        for n in ITEMS
    ]
    return {
        "effects": effects,
        "priors": [],
        "items": items,
        "channel": "corner_time_s",
        "better": "lower",
        "rope": 0.005,
        "level": 0.90,
        "guardrails": [],
    }


def test_eight_null_items_qualify_anything_at_most_ten_percent_of_the_time() -> None:
    rng = np.random.default_rng(2026)
    runs = 2000
    taken = [len(car(rng, [0.0] * 8, 1, 5).changes) for _ in range(runs)]
    unadjusted = 1 - 0.95**8  # each row's 90 % interval alone: 34 %

    assert np.mean([n > 0 for n in taken]) <= 0.10  # measured 5.1 % (q = 0.05)
    assert np.mean([n > 0 for n in taken]) < unadjusted / 3


def test_eight_null_items_pooled_from_three_experiments_each_rarely_qualify() -> None:
    rng = np.random.default_rng(7)
    taken = [len(car(rng, [0.0] * 8, 3, 5).changes) for _ in range(300)]

    assert np.mean([n > 0 for n in taken]) <= 0.10


def test_the_shrunk_prediction_is_unbiased_where_the_raw_sum_over_promises() -> None:
    """True effects of the items drawn from N(0, 6 ms); the errors are nearly known (df 60),
    the setting empirical Bayes is unbiased in."""
    rng = np.random.default_rng(11)
    shrunk, raw = [], []
    for _ in range(600):
        truths = list(rng.normal(0.0, 0.006, 8))
        for change in car(rng, truths, 1, 60).changes:
            truth = truths[ITEMS.index(change.parameter)]
            shrunk.append(change.shrunk_estimate - truth)
            raw.append(change.estimate - truth)

    sem = np.std(shrunk) / np.sqrt(len(shrunk))
    assert len(shrunk) > 500
    assert abs(np.mean(shrunk)) < max(3 * sem, 0.0003)  # measured -0.2 ms
    assert np.mean(raw) < -0.0010  # the winner's curse: the raw estimates promise 2.1 ms too much


def test_benjamini_hochberg_steps_up_and_shrinkage_moves_towards_zero() -> None:
    assert benjamini_hochberg([0.001, 0.2, 0.013, 0.04], q=0.10) == [True, False, True, True]
    assert benjamini_hochberg([0.5, 0.6], q=0.10) == [False, False]
    assert benjamini_hochberg([]) == []
    assert prior_variance([0.001, -0.001], [0.003, 0.003]) == 0.0
    assert prior_variance([], []) == 0.0
    assert shrink(-0.01, 0.003, 0.0) == 0.0
    assert -0.01 < shrink(-0.01, 0.003, 0.006**2) < 0
