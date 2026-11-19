"""Many items, one base setup: false discoveries and the winner's curse (ADR 0033, finding I13).

A base setup takes every change measured better. With eight items that do nothing, each judged
at a one-sided 5 %, a third of base setups would contain at least one of them. And the changes
taken are those whose estimate happened to come out best, so their sum over-promises.

- **Benjamini-Hochberg** across every measured change of the car on the judged channel keeps
  the expected share of false discoveries at `q`. When nothing has an effect, that is the chance
  that anything qualifies at all. q = 0.05 is the one-sided 5 % each row's 90 % interval already
  spends, shared among the items: a car with one measured change is judged as before (tests:
  8 null items, about 5 % instead of 34 %; at q = 0.10 it was 10.2 %).
- **Empirical Bayes shrinkage** of each estimate towards zero, with the spread of the true
  effects estimated from all the car's measured changes (method of moments): the prediction is
  the posterior mean, unbiased given what qualified when that spread is right (tests).
"""

from collections.abc import Sequence

import numpy as np

FDR_Q = 0.05


def benjamini_hochberg(p_values: Sequence[float], q: float = FDR_Q) -> list[bool]:
    """Which hypotheses are rejected at false discovery rate `q` (step-up)."""
    m = len(p_values)
    if m == 0:
        return []
    order = np.argsort(p_values, kind="stable")
    thresholds = q * np.arange(1, m + 1) / m
    passing = np.nonzero(np.asarray(p_values)[order] <= thresholds)[0]
    rejected = [False] * m
    if passing.size:
        for index in order[: passing[-1] + 1]:
            rejected[int(index)] = True
    return rejected


def prior_variance(estimates: Sequence[float], errors: Sequence[float]) -> float:
    """Variance of the true effects across items: the mean of estimate^2 - se^2, at least 0."""
    if not estimates:
        return 0.0
    y, s = np.asarray(estimates), np.asarray(errors)
    return max(0.0, float(np.mean(y * y - s * s)))


def shrink(estimate: float, error: float, variance: float) -> float:
    """Posterior mean of a true effect drawn from N(0, `variance`) and measured with `error`."""
    if variance <= 0.0:
        return 0.0
    return estimate * variance / (variance + error * error)
