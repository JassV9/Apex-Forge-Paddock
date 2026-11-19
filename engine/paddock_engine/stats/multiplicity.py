"""Several tests at once without several chances of a false alarm (ADR 0029).

An experiment reports about ten intervals. Testing three driver-input channels each at 10 %
flags "the driver adapted" about 27 % of the time when nothing changed. Holm's step-down keeps
the chance of any false flag in a family at its alpha, and it is never less powerful than
Bonferroni.
"""

from collections.abc import Sequence

from scipy import stats


def two_sided_p(estimate: float, standard_error: float, degrees: int) -> float:
    """p-value of estimate = 0 with Student's t. With no error at all, any change is certain."""
    if standard_error <= 0:
        return 1.0 if estimate == 0 else 0.0
    return float(2.0 * stats.t.sf(abs(estimate) / standard_error, degrees))


def holm(p_values: Sequence[float], alpha: float) -> list[bool]:
    """Which hypotheses Holm's procedure rejects at family-wise `alpha`, in input order."""
    order = sorted(range(len(p_values)), key=lambda i: p_values[i])
    rejected = [False] * len(p_values)
    for rank, index in enumerate(order):
        if p_values[index] > alpha / (len(p_values) - rank):
            break
        rejected[index] = True
    return rejected
