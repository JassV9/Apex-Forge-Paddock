"""Per-axle views of the per-wheel channels."""

from dataclasses import dataclass

import numpy as np

from paddock_engine.io.samples import FloatArray, PassSamples


@dataclass(frozen=True)
class AxleChannels:
    front_slip: FloatArray  # load-weighted mean of the two tyres' |slip angle|, degrees
    rear_slip: FloatArray
    front_slip_max: FloatArray  # the worse tyre: an averaged axle can hide one saturated tyre
    rear_slip_max: FloatArray
    front_ratio: FloatArray  # larger |slip ratio| of the two tyres
    rear_ratio: FloatArray


def _weighted(
    angle_a: FloatArray, angle_b: FloatArray, load_a: FloatArray, load_b: FloatArray
) -> FloatArray:
    load_a, load_b = np.maximum(load_a, 0.0), np.maximum(load_b, 0.0)
    total = load_a + load_b
    loaded = total > 1.0
    weighted = (angle_a * load_a + angle_b * load_b) / np.where(loaded, total, 1.0)
    return np.where(loaded, weighted, (angle_a + angle_b) / 2)


def axle_channels(s: PassSamples) -> AxleChannels:
    angle, ratio = np.abs(s.slip_angle), np.abs(s.slip_ratio)
    return AxleChannels(
        front_slip=_weighted(angle[:, 0], angle[:, 1], s.load[:, 0], s.load[:, 1]),
        rear_slip=_weighted(angle[:, 2], angle[:, 3], s.load[:, 2], s.load[:, 3]),
        front_slip_max=np.maximum(angle[:, 0], angle[:, 1]),
        rear_slip_max=np.maximum(angle[:, 2], angle[:, 3]),
        front_ratio=np.maximum(ratio[:, 0], ratio[:, 1]),
        rear_ratio=np.maximum(ratio[:, 2], ratio[:, 3]),
    )
