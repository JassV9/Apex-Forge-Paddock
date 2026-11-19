"""Does the setup act differently in two groups of corners? (ADR 0029)

A change that gains 10 ms in slow corners and loses 10 ms in fast ones averages to nothing over
the lap, and "neutral" would hide the most useful thing the experiment found. The interaction
of the arm with a corner's group is measured at block level like the effect itself: per run
block, the corner-weighted mean residual of the primary fit in one group less that in the
other. A shock shared by the whole block cancels in that difference; what remains is compared
between the arms with a two-sample t on the blocks (degrees of freedom: blocks - 2). The
covariates were fitted once for the whole lap, with one slope for every corner, so they cancel
in the difference too.
"""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from itertools import combinations

import numpy as np

from paddock_engine.stats.block_level import arm_error, cell_means, corner_residuals, row_arms
from paddock_engine.stats.effects import block_codes, weighted_fit
from paddock_engine.stats.model import NotEstimableError
from paddock_engine.stats.observations import Observation, build_design


@dataclass(frozen=True)
class Interaction:
    first: str  # group of corners
    second: str
    estimate: float  # (arm - baseline) in `first` less (arm - baseline) in `second`
    standard_error: float
    degrees: int


def _group_means(
    observations: Sequence[Observation], baseline_arm: str, groups: Mapping[int, str]
) -> dict[tuple[int, int, str], float]:
    """Corner-weighted mean residual per (block, arm, group) of the first arm's fit."""
    design = build_design(observations, baseline_arm)
    weights, fit = weighted_fit(design)
    residuals = corner_residuals(design, fit.coefficients)
    arms, blocks = row_arms(design), block_codes(observations)
    keep = np.array([o.corner in groups for o in observations]) & (arms <= 1)
    keys = [
        (int(block), int(arm), groups[o.corner])
        for o, block, arm, kept in zip(observations, blocks, arms, keep, strict=True)
        if kept
    ]
    return cell_means(residuals[keep], weights[design.groups][keep], keys)


def _pair(
    means: Mapping[tuple[int, int, str], float], first: str, second: str
) -> Interaction | None:
    cells = sorted({(block, arm) for block, arm, _ in means})
    both = [c for c in cells if (*c, first) in means and (*c, second) in means]
    outcome = np.array([means[(*c, first)] - means[(*c, second)] for c in both])
    regressors = np.column_stack([np.ones(len(both)), [float(arm) for _, arm in both]])
    try:
        estimate, error, degrees = arm_error(outcome, regressors)
    except NotEstimableError:
        return None
    return Interaction(first, second, estimate, error, degrees)


def group_interactions(
    observations: Sequence[Observation], baseline_arm: str, groups: Mapping[int, str]
) -> list[Interaction]:
    """One interaction per pair of groups (sorted by name) that the blocks can estimate.

    `groups` maps a corner number to its group; corners not in it are left out. Raises
    NotEstimableError when the passes cannot support the primary fit.
    """
    means = _group_means(observations, baseline_arm, groups)
    names = sorted({group for _, _, group in means})
    pairs = (_pair(means, first, second) for first, second in combinations(names, 2))
    return [pair for pair in pairs if pair is not None]
