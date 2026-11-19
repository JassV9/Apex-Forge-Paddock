"""The rows the statistics work on: one value of one channel per corner pass."""

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field

import numpy as np

from paddock_engine.io.samples import FloatArray
from paddock_engine.stats.model import IntArray

MAX_VARIANCE_INFLATION = 10.0  # of any column: 1 / (1 - R^2) on the other columns of the design
# A session term: 1 on the passes of the experiment's n-th game session (n >= 2), else 0. A new
# session restarts the game, refuels and resets the track; the term absorbs that step (ADR 0029).
SESSION_PREFIX = "session_"


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
    session_confounded: bool = False  # a session term is too tangled with the arm to keep apart


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


def _untangled(
    arms: FloatArray, covariates: FloatArray, protected: list[bool]
) -> tuple[list[int], list[int], bool]:
    """Indices of the covariate columns to keep and to drop (ADR 0022), and whether a protected
    column (a session term) stays tangled with the arm.

    While any column of arm indicators and covariates has a variance inflation above the limit,
    the unprotected covariate with the highest one goes (the last of equals, so the
    alphabetically first name stays). The arm and the session terms are never dropped: an
    experiment over several sessions is judged with its session terms or not at all.
    """
    kept, dropped = list(range(covariates.shape[1])), []
    inflation = _variance_inflation(np.column_stack([arms, covariates]))
    while inflation.max() > MAX_VARIANCE_INFLATION:
        droppable = [i for i in range(len(kept)) if not protected[kept[i]]]
        if not droppable:
            break
        worst = max(droppable, key=lambda i: (inflation[arms.shape[1] + i], i))
        dropped.append(kept.pop(worst))
        inflation = _variance_inflation(np.column_stack([arms, covariates[:, kept]]))
    return kept, dropped, bool(inflation.max() > MAX_VARIANCE_INFLATION)


def _usable_covariates(
    observations: Sequence[Observation], groups: IntArray, arms: FloatArray
) -> tuple[list[str], list[str], FloatArray, bool]:
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
    protected = [name.startswith(SESSION_PREFIX) for name in candidates]
    kept, dropped, confounded = _untangled(arms, matrix, protected)
    return (
        [candidates[i] for i in kept],
        [candidates[i] for i in sorted(dropped)],
        matrix[:, kept],
        confounded,
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
    covariates, dropped, covariate_columns, confounded = _usable_covariates(
        observations, groups, centred_arms
    )
    return Design(
        y=np.array([o.value for o in observations], dtype=np.float64),
        x=np.column_stack([indicators, covariate_columns]),
        groups=groups,
        corners=corners,
        arms=arms,
        covariates=covariates,
        dropped=dropped,
        session_confounded=confounded,
    )
