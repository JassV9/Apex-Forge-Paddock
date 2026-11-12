"""The rows the statistics work on: one value of one channel per corner pass."""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray

MAX_COVARIATE_CORRELATION = 0.98


@dataclass(frozen=True)
class Observation:
    corner: int
    arm: str  # which setup the pass was driven on
    value: float  # the channel being compared, e.g. corner time in seconds
    block: int = 0  # run block the pass belongs to (A-B-A-B: 0, 1, 2, 3)
    lap: int | None = None  # lap within the session; passes of one lap share their errors
    covariates: Mapping[str, float] = field(default_factory=dict)


@dataclass(frozen=True)
class Design:
    y: FloatArray
    x: FloatArray  # arm indicators first (one per non-baseline arm), then covariates
    groups: IntArray  # corner codes 0..G-1
    arms: list[str]  # the non-baseline arms, in column order
    covariates: list[str]  # covariates that were usable, in column order


def _usable_covariates(
    observations: Sequence[Observation], groups: IntArray
) -> tuple[list[str], FloatArray]:
    """Covariates present on every pass, varying within corners, and not duplicating another."""
    names = sorted(set.intersection(*(set(o.covariates) for o in observations)))
    kept: list[str] = []
    columns: list[FloatArray] = []
    for name in names:
        column = np.array([o.covariates[name] for o in observations], dtype=np.float64)
        if not np.isfinite(column).all():
            continue
        sums = np.bincount(groups, weights=column)
        centred = column - (sums / np.bincount(groups))[groups]
        if centred.std() < 1e-12:
            continue
        duplicate = any(
            abs(np.corrcoef(centred, other)[0, 1]) > MAX_COVARIATE_CORRELATION for other in columns
        )
        if not duplicate:
            kept.append(name)
            columns.append(centred)
    matrix = np.column_stack(columns) if columns else np.empty((len(observations), 0))
    return kept, matrix


def build_design(observations: Sequence[Observation], baseline_arm: str) -> Design:
    """Covariate columns are centred within corners; only their slopes matter."""
    arms = sorted({o.arm for o in observations} - {baseline_arm})
    if not arms or all(o.arm != baseline_arm for o in observations):
        raise ValueError("observations need the baseline arm and at least one other arm")
    corners = sorted({o.corner for o in observations})
    code = {corner: index for index, corner in enumerate(corners)}
    groups = np.array([code[o.corner] for o in observations], dtype=np.intp)
    indicators = np.column_stack(
        [np.array([o.arm == arm for o in observations], dtype=np.float64) for arm in arms]
    )
    covariates, covariate_columns = _usable_covariates(observations, groups)
    return Design(
        y=np.array([o.value for o in observations], dtype=np.float64),
        x=np.column_stack([indicators, covariate_columns]),
        groups=groups,
        arms=arms,
        covariates=covariates,
    )
