"""Priors from other cars: shrunk, widened and never passed off as measurements.

A car that has not measured a change may borrow what a similar car measured for the item
with the same role and axle. The borrowed effect is pulled towards zero by how alike the
cars are, and its uncertainty grows by how far it was pulled: the interval of a prior always
admits both "about what the other car showed" and "much less". The car's own data replaces a
prior the moment it exists.

The similarity weights are judgement. The only thing known about a car today is its
drivetrain; aero level, weight distribution and tyre model (docs/design.md) are not recorded.
"""

import math
from collections.abc import Iterable

from paddock_engine.contracts.sensitivity_request_schema import Transfer
from paddock_engine.sensitivity.pooling import Estimate, from_interval

SAME_DRIVETRAIN = 0.5
OTHER_DRIVETRAIN = 0.25

Key = tuple[str, int, str, str]  # parameter, clicks, channel, corner class


def similarity(transfer: Transfer) -> float:
    return SAME_DRIVETRAIN if transfer.same_drivetrain else OTHER_DRIVETRAIN


def shrink(source: Estimate, weight: float) -> Estimate:
    """The source effect times `weight`, with the distance it was moved added to its error."""
    moved = (1.0 - weight) * source.mean
    return Estimate(mean=weight * source.mean, se=math.hypot(source.se, moved))


def key_of(transfer: Transfer) -> Key:
    return (transfer.parameter, transfer.clicks, transfer.channel, transfer.corner_class.value)


def best_sources(transfers: Iterable[Transfer]) -> dict[Key, Transfer]:
    """One source per change: the most similar car, then the one with the most passes."""
    chosen: dict[Key, Transfer] = {}
    for transfer in transfers:
        if transfer.clicks == 0:
            continue
        current = chosen.get(key_of(transfer))
        rank = (similarity(transfer), transfer.n_passes)
        if current is None or rank > (similarity(current), current.n_passes):
            chosen[key_of(transfer)] = transfer
    return chosen


def prior(transfer: Transfer, level: float) -> Estimate:
    source = from_interval(transfer.estimate, transfer.ci_low, transfer.ci_high, level)
    return shrink(source, similarity(transfer))
