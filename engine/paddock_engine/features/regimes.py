"""Speed regimes per corner, never per pass (ADR 0031, finding C4).

A pass's own minimum speed is an outcome of the setup: classing each pass by it let a corner
change class between the arms (experiment 17, corner 6: 38 % of one arm's passes slow, none of
the other's). Here the class belongs to the corner and is frozen from the baseline: the median
minimum speed of the corner's baseline passes.

The cut-offs are per car and track: the two largest gaps between the corners' log median
minimum speeds, with at least MIN_PER_CLASS corners in every class, each cut at the geometric
mean of the speeds either side. The Formula Hybrid 2026 at the Hungaroring breaks at 130 and
161 km/h (7 slow, 3 medium, 4 fast); the fixed 100/150 km/h bands gave it one slow corner. With
fewer than MIN_CORNERS_FOR_GAPS corners the fixed bands are used, on the corner medians.

The aero share is an ESTIMATE, never a measurement: a least-squares fit of peak lateral g
against v^2 over the corner medians, `lat_g = c0 + c1 v^2`, read as the share of grip that grows
with speed, `c1 v^2 / (c0 + c1 v^2)` at a corner's median minimum speed. The fit lumps
downforce with everything else that differs between corners (camber, banking, kerbs, the AI's
plan), so it is reported only when R^2 >= MIN_R2, c0 > 0 and c1 > 0, and it decides nothing.
"""

import math
from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from statistics import median

import numpy as np

CLASSES = ("slow", "medium", "fast")
MIN_PER_CLASS = 2
MIN_CORNERS_FOR_GAPS = 3 * MIN_PER_CLASS
FIXED_BREAKS_KMH = (100.0, 150.0)
MIN_R2 = 0.5


@dataclass(frozen=True)
class Regimes:
    classes: dict[int, str]  # corner -> slow, medium or fast
    medians_kmh: dict[int, float]  # the baseline's median minimum speed per corner
    breaks_kmh: tuple[float, float]
    method: str  # "gaps" (per car and track), "fixed" (too few corners) or "stored"


@dataclass(frozen=True)
class AeroFit:
    c0_g: float
    c1_g_per_ms2: float  # g per (m/s)^2
    r2: float
    corners: int

    @property
    def adequate(self) -> bool:
        return self.r2 >= MIN_R2 and self.c0_g > 0 and self.c1_g_per_ms2 > 0

    def share(self, v_kmh: float) -> float | None:
        """The ESTIMATED share of lateral grip that grows with speed, at this speed."""
        if not self.adequate:
            return None
        aero = self.c1_g_per_ms2 * (v_kmh / 3.6) ** 2
        return aero / (self.c0_g + aero)


def gap_breaks(medians_kmh: Iterable[float]) -> tuple[float, float] | None:
    """The two largest gaps in log speed leaving MIN_PER_CLASS corners per class, or None."""
    logs = sorted(math.log(v) for v in medians_kmh if v > 0)
    n = len(logs)
    if n < MIN_CORNERS_FOR_GAPS:
        return None
    gaps = [logs[i + 1] - logs[i] for i in range(n - 1)]
    best: tuple[float, int, int] | None = None
    for low in range(MIN_PER_CLASS - 1, n - 2 * MIN_PER_CLASS):
        for high in range(low + MIN_PER_CLASS, n - MIN_PER_CLASS):
            size = gaps[low] + gaps[high]
            if best is None or size > best[0]:
                best = (size, low, high)
    assert best is not None
    _, low, high = best

    def cut(i: int) -> float:
        return math.exp((logs[i] + logs[i + 1]) / 2)

    return cut(low), cut(high)


def classify(v_kmh: float, breaks: tuple[float, float]) -> str:
    if v_kmh < breaks[0]:
        return "slow"
    return "medium" if v_kmh < breaks[1] else "fast"


def _stored_mode(classes: Iterable[str | None]) -> str | None:
    counts: dict[str, int] = {}
    for value in classes:
        if value in CLASSES:
            counts[value] = counts.get(value, 0) + 1
    return max(counts, key=lambda c: (counts[c], -CLASSES.index(c))) if counts else None


def corner_regimes(baseline: Iterable[tuple[int, float | None, str | None]]) -> Regimes:
    """Classes from the baseline's (corner, minimum speed km/h, stored class) per pass.

    A corner whose passes carry no minimum speed keeps the most common stored class of its
    baseline passes (imported data); that is still one class per corner.
    """
    speeds: dict[int, list[float]] = {}
    stored: dict[int, list[str | None]] = {}
    for corner, v_min, speed_class in baseline:
        stored.setdefault(corner, []).append(speed_class)
        if v_min is not None and math.isfinite(v_min):
            speeds.setdefault(corner, []).append(v_min)
    medians = {corner: float(median(values)) for corner, values in speeds.items()}
    breaks = gap_breaks(medians.values())
    method = "gaps" if breaks is not None else "fixed"
    breaks = breaks or FIXED_BREAKS_KMH
    classes = {corner: classify(v, breaks) for corner, v in medians.items()}
    for corner, values in stored.items():
        mode = _stored_mode(values)
        if corner not in classes and mode is not None:
            classes[corner] = mode
    if not medians and classes:
        method = "stored"
    return Regimes(classes=classes, medians_kmh=medians, breaks_kmh=breaks, method=method)


def aero_fit(corner_points: Mapping[int, tuple[float, float]]) -> AeroFit | None:
    """Fit over corners' (median minimum speed km/h, median peak lateral g); None under 3."""
    if len(corner_points) < 3:
        return None
    v = np.array([point[0] for point in corner_points.values()]) / 3.6
    lat = np.abs(np.array([point[1] for point in corner_points.values()]))
    design = np.column_stack([np.ones_like(v), v**2])
    (c0, c1), *_ = np.linalg.lstsq(design, lat, rcond=None)
    total = float(((lat - lat.mean()) ** 2).sum())
    residual = float(((lat - design @ np.array([c0, c1])) ** 2).sum())
    r2 = 1.0 - residual / total if total > 0 else 0.0
    return AeroFit(c0_g=float(c0), c1_g_per_ms2=float(c1), r2=r2, corners=len(corner_points))
