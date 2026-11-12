"""Front and rear slip per phase, binned by corner speed, never averaged across speeds.

This is the finding that started Paddock (docs/audit-v0.md): on the real FA26 COTA session,
mid-corner front slip is about 7.5 degrees in slow corners and about 4 degrees in fast ones,
while rear slip stays near 3 degrees. One car-wide balance number hides that completely.
"""

from collections.abc import Iterable, Mapping
from dataclasses import dataclass

from paddock_engine.features.pass_features import Metrics

MIN_CONFIDENCE = 0.5  # phase readings below this are transients or barely cornering


@dataclass(frozen=True)
class SlipBin:
    passes: int
    front_slip_deg: float
    rear_slip_deg: float

    @property
    def front_minus_rear_deg(self) -> float:
        return self.front_slip_deg - self.rear_slip_deg


def speed_binned_slip(
    passes: Iterable[tuple[str, Metrics]],
    phase: str = "mid",
    min_confidence: float = MIN_CONFIDENCE,
) -> Mapping[str, SlipBin]:
    """Mean axle slip of one phase per speed class.

    `passes` yields (speed class, metrics) of valid passes. A pass counts when it has a reading
    for the phase with at least `min_confidence`.
    """
    front: dict[str, list[float]] = {}
    rear: dict[str, list[float]] = {}
    for speed_class, metrics in passes:
        confidence = metrics.get(f"confidence_{phase}")
        front_slip = metrics.get(f"front_slip_{phase}_deg")
        rear_slip = metrics.get(f"rear_slip_{phase}_deg")
        if confidence is None or front_slip is None or rear_slip is None:
            continue
        if confidence < min_confidence:
            continue
        front.setdefault(speed_class, []).append(front_slip)
        rear.setdefault(speed_class, []).append(rear_slip)
    return {
        speed_class: SlipBin(
            passes=len(values),
            front_slip_deg=sum(values) / len(values),
            rear_slip_deg=sum(rear[speed_class]) / len(values),
        )
        for speed_class, values in front.items()
    }
