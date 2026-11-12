"""The evidence pack: every number a hypothesis or an explanation may cite.

One entry is the mean of a metric over the valid passes of a corner class (slow, medium, fast,
or all). Its id is `<metric>.<class>`. Nothing downstream states a number that is not here.
"""

from collections.abc import Iterable, Sequence

from paddock_engine.contracts.diagnose_request_schema import DiagnosePass
from paddock_engine.contracts.diagnosis_schema import CornerClass, Evidence

MIN_PASSES = 3  # fewer passes than this is an anecdote, not evidence
MIN_CONFIDENCE = 0.5  # phase readings below this are transients or barely cornering

# Metrics read per phase carry that phase's confidence; a reading below MIN_CONFIDENCE is left
# out, as in features/speed_bins.py.
PHASES = ("entry", "mid", "exit")
PHASE_METRICS = {
    f"{name}_{phase}{unit}": phase
    for phase in PHASES
    for name, unit in (("front_slip", "_deg"), ("rear_slip", "_deg"), ("steer_rate", ""))
}
PASS_METRICS = (
    "front_saturation_index",
    "steer_per_g_mid",
    "rear_slip_peak_deg",
    "countersteer_s",
    "countersteer_exit_s",
    "front_lockups",
    "rear_lockups",
    "wheelspin",
    "wheelspin_s",
    "trail_share",
)
CLASSES = (CornerClass.slow, CornerClass.medium, CornerClass.fast, CornerClass.all)

Pack = dict[str, Evidence]


def evidence_id(metric: str, corner_class: str) -> str:
    return f"{metric}.{corner_class}"


def _readings(passes: Iterable[DiagnosePass], metric: str) -> list[float]:
    phase = PHASE_METRICS.get(metric)
    values = []
    for item in passes:
        value = item.metrics.get(metric)
        if value is None:
            continue
        if phase is not None and (item.metrics.get(f"confidence_{phase}") or 0.0) < MIN_CONFIDENCE:
            continue
        values.append(value)
    return values


def build(passes: Sequence[DiagnosePass]) -> Pack:
    """Means per metric and corner class, wherever at least MIN_PASSES passes have a reading."""
    pack: Pack = {}
    for corner_class in CLASSES:
        members = [
            p
            for p in passes
            if corner_class is CornerClass.all or p.speed_class.value == corner_class.value
        ]
        for metric in (*PHASE_METRICS, *PASS_METRICS):
            values = _readings(members, metric)
            if len(values) < MIN_PASSES:
                continue
            key = evidence_id(metric, corner_class.value)
            pack[key] = Evidence(
                id=key,
                metric=metric,
                corner_class=corner_class,
                value=sum(values) / len(values),
                passes=len(values),
            )
    return pack
