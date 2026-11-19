"""Pre-registered looks at an experiment and the error each may spend (ADR 0029).

An experiment is judged after 8 blocks, and may be extended twice by two blocks: looks at 8, 10
and 12 blocks. Judging after each with the same 90 % interval would accept a change at the edge
of the equivalence band 8.5 to 12 % of the time instead of 5 % (simulated, ADR 0029). The looks
therefore share the one-sided 5 % by the Lan-DeMets O'Brien-Fleming spending function
alpha(t) = 2 - 2 Phi(z_{1 - alpha / 2} / sqrt(t)), with t the share of the 12 blocks driven.

The boundary of each look is solved so that the chance of first crossing at that look is what
the function allows it, given that the earlier looks did not cross (the increments of the score
are independent: numerical integration on Gauss-Legendre nodes). For a total of 5 % the
boundaries are z = 2.135, 1.926 and 1.752; tests/test_sequential.py checks them against a
Monte Carlo of correlated looks. The verdict spends SPENT_SHARE of it (3.5 %): z = 2.333, 2.096
and 1.906, intervals of 98.0, 96.4 and 94.3 %. The interval reported at a look is the repeated
confidence interval: estimate +- t x standard error at the look's one-sided level, for efficacy
and for equivalence alike.

Fewer than 8 blocks is before the first look: such an interval is never a confirmed finding.
"""

from functools import lru_cache

import numpy as np
from scipy import optimize, stats

FIRST_LOOK_BLOCKS = 8
BLOCKS_PER_LOOK = 2  # an extension adds a baseline and a variant block
LOOKS = 3
# The looks spend 70 % of the one-sided error. The spending function is exact for z statistics;
# the verdict's are block-level t statistics on 5 to 9 degrees of freedom, refitted at every
# look, and with the full 5 % a change at the band edge was accepted 5.9 to 6.5 % of the time
# under the extension rules (simulated, ADR 0029). With 70 %: 3.8 %.
SPENT_SHARE = 0.7
_NODES = 64
_FAR = 10.0  # standard deviations below the boundary the integration starts


def look_of(blocks: int) -> int:
    """Look number of a judgement after `blocks` run blocks: 0 before the first, at most LOOKS."""
    if blocks < FIRST_LOOK_BLOCKS:
        return 0
    return min(LOOKS, 1 + (blocks - FIRST_LOOK_BLOCKS) // BLOCKS_PER_LOOK)


def _fractions() -> list[float]:
    last = FIRST_LOOK_BLOCKS + BLOCKS_PER_LOOK * (LOOKS - 1)
    return [(FIRST_LOOK_BLOCKS + BLOCKS_PER_LOOK * k) / last for k in range(LOOKS)]


def _spent(alpha: float) -> list[float]:
    """Cumulative one-sided error spent by each look."""
    z = stats.norm.isf(alpha / 2.0)
    return [float(2.0 - 2.0 * stats.norm.cdf(z / np.sqrt(t))) for t in _fractions()]


def _nodes(root: float, bound: float) -> tuple[np.ndarray, np.ndarray]:
    """Gauss-Legendre nodes on the score below a boundary, and their quadrature weights."""
    nodes, weights = np.polynomial.legendre.leggauss(_NODES)
    low, high = -_FAR * root, bound * root
    return (high - low) / 2.0 * nodes + (high + low) / 2.0, weights * (high - low) / 2.0


def _excess(
    z: float, score: np.ndarray, mass: np.ndarray, root: float, spread: float, target: float
) -> float:
    """Chance of first crossing boundary z at this look, less what the look may spend."""
    return float(np.sum(mass * stats.norm.sf(z * root, loc=score, scale=spread))) - target


@lru_cache(maxsize=8)
def boundaries(alpha: float) -> tuple[float, ...]:
    """One-sided efficacy boundaries (z) of each look for a total one-sided error `alpha`."""
    fractions, spent = _fractions(), _spent(alpha)
    bounds = [float(stats.norm.isf(spent[0]))]
    score, quadrature = _nodes(np.sqrt(fractions[0]), bounds[0])
    mass = quadrature * stats.norm.pdf(score, scale=np.sqrt(fractions[0]))
    for k in range(1, LOOKS):
        spread, root = np.sqrt(fractions[k] - fractions[k - 1]), np.sqrt(fractions[k])
        target = spent[k] - spent[k - 1]
        step = (score, mass, root, spread, target)
        bound = float(optimize.brentq(_excess, 0.1, 8.0, args=step, xtol=1e-12))
        bounds.append(bound)
        new, quadrature = _nodes(root, bound)
        density = stats.norm.pdf(new[:, None], loc=score[None, :], scale=spread) @ mass
        score, mass = new, quadrature * density
    return tuple(bounds)


def interval_level(look: int, level: float) -> float:
    """Two-sided level of the interval reported at `look` for a nominal `level` (e.g. 0.90).

    Before the first look the nominal level; at a look the repeated confidence interval, whose
    one-sided error is what the look may spend.
    """
    if look <= 0:
        return level
    bound = boundaries(round(SPENT_SHARE * (1.0 - level) / 2.0, 12))[min(look, LOOKS) - 1]
    return float(1.0 - 2.0 * stats.norm.sf(bound))
