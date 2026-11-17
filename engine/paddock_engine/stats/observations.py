"""The rows the statistics work on: one value of one channel per corner pass."""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray

MAX_VARIANCE_INFLATION = 10.0  # of any column: 1 / (1 - R^2) on the other columns of the design


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
    corners: list[int]  # the corner number of each code
    arms: list[str]  # the non-baseline arms, in column order
    covariates: list[str]  # covariates that were usable, in column order
    dropped: list[str] = field(default_factory=list)  # covariates too tangled with the rest


def _centre_within(values: FloatArray, groups: IntArray) -> FloatArray:
    sums = np.bincount(groups, weights=values)
    centred: FloatArray = values - (sums / np.bincount(groups))[groups]
    return centred


def _variance_inflation(columns: FloatArray) -> FloatArray:
    """Per column, 1 / (1 - R^2) of regressing it on the others; infinite when it is a copy."""
    inflation = np.ones(columns.shape[1])
    for index in range(columns.shape[1]):
        column, others = columns[:, index], np.delete(columns, index, axis=1)
        total = float(column @ column)
        if others.shape[1] == 0 or total <= 0.0:
            continue
        coefficients, *_ = np.linalg.lstsq(others, column, rcond=None)
        unexplained = float(np.sum((column - others @ coefficients) ** 2)) / total
        inflation[index] = 1.0 / unexplained if unexplained > 1e-12 else np.inf
    return inflation


def _untangled(arms: FloatArray, covariates: FloatArray) -> tuple[list[int], list[int]]:
    """Indices of the covariate columns to keep and to drop (ADR 0022).

    While any column of arm indicators and covariates has a variance inflation above the limit,
    the covariate with the highest one goes (the last of equals, so the alphabetically first
    name stays). The arm is never dropped.
    """
    kept, dropped = list(range(covariates.shape[1])), []
    while kept:
        inflation = _variance_inflation(np.column_stack([arms, covariates[:, kept]]))
        if inflation.max() <= MAX_VARIANCE_INFLATION:
            break
        worst = max(range(len(kept)), key=lambda i: (inflation[arms.shape[1] + i], i))
        dropped.append(kept.pop(worst))
    return kept, dropped


def _usable_covariates(
    observations: Sequence[Observation], groups: IntArray, arms: FloatArray
) -> tuple[list[str], list[str], FloatArray]:
    """Covariates present on every pass and varying within corners, then untangled from the arms.

    `arms` holds the arm indicators centred within corners, as the covariates are.
    """
    names = sorted(set.intersection(*(set(o.covariates) for o in observations)))
    candidates: list[str] = []
    columns: list[FloatArray] = []
    for name in names:
        column = np.array([o.covariates[name] for o in observations], dtype=np.float64)
        if not np.isfinite(column).all():
            continue
        centred = _centre_within(column, groups)
        if centred.std() >= 1e-12:
            candidates.append(name)
            columns.append(centred)
    matrix = np.column_stack(columns) if columns else np.empty((len(observations), 0))
    kept, dropped = _untangled(arms, matrix)
    return (
        [candidates[i] for i in kept],
        [candidates[i] for i in sorted(dropped)],
        matrix[:, kept],
    )


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
    centred_arms = np.column_stack(
        [_centre_within(indicators[:, i], groups) for i in range(len(arms))]
    )
    covariates, dropped, covariate_columns = _usable_covariates(observations, groups, centred_arms)
    return Design(
        y=np.array([o.value for o in observations], dtype=np.float64),
        x=np.column_stack([indicators, covariate_columns]),
        groups=groups,
        corners=corners,
        arms=arms,
        covariates=covariates,
        dropped=dropped,
    )
