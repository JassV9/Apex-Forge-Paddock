"""What else could explain a symptom: the driver, or the conditions.

v0 went straight from "the front slips more than the rear" to "soften the front bar" and
never asked whether the front was out of grip because of the car or because of what was asked
of it (docs/audit-v0.md, causes 1 and 2). Every hypothesis now carries both alternatives, and
says whether the session's own numbers speak for them.

The thresholds are engineering judgement, not measurements. They have not been checked
against a driver's own reading of the car.
"""

from paddock_engine.contracts.diagnosis_schema import Alternative, Kind
from paddock_engine.diagnosis.evidence import Pack, evidence_id
from paddock_engine.diagnosis.symptoms import AERO_CLASS, Symptom

# Share of the turn with steering still rising while lateral g does not follow.
SATURATION_SHARE = 0.25
# Front slip that grows by this much from fast to slow corners while rear slip stays within
# REAR_FLAT_DEG is the pattern of steering past the front tyre's peak, not of balance.
FRONT_GROWTH_DEG = 1.5
REAR_FLAT_DEG = 0.75

CONDITIONS = Alternative(
    kind=Kind.conditions,
    statement=(
        "Tyre temperature, fuel load and track grip were not compared: this is one session on "
        "one setup. Interleaved blocks in an experiment are what separate them."
    ),
    supported=False,
    evidence=[],
)
DRIVER_NOT_SHOWN = Alternative(
    kind=Kind.driver_input,
    statement=(
        "Driver input could cause this too; the session's data neither shows nor excludes it."
    ),
    supported=False,
    evidence=[],
)


def _saturation(pack: Pack, symptom: Symptom) -> Alternative | None:
    reading = pack.get(evidence_id("front_saturation_index", symptom.corner_class))
    if reading is None or reading.value < SATURATION_SHARE:
        return None
    return Alternative(
        kind=Kind.driver_input,
        statement=(
            f"The front tyres are past their peak: for {reading.value:.2f} of the turn the "
            "steering keeps rising and lateral g does not follow. More lock is being asked "
            "for than the front can turn into grip, which a setup change may not cure."
        ),
        supported=True,
        evidence=[reading.id],
    )


def _speed_pattern(pack: Pack, symptom: Symptom) -> Alternative | None:
    """Front slip that depends on corner speed while rear slip does not (mid-corner only)."""
    if symptom.phase != "mid" or symptom.corner_class in (AERO_CLASS, "all"):
        return None
    ids = [
        evidence_id(f"{axle}_slip_mid_deg", corner_class)
        for corner_class in (symptom.corner_class, AERO_CLASS)
        for axle in ("front", "rear")
    ]
    if any(key not in pack for key in ids):
        return None
    front_here, rear_here, front_fast, rear_fast = (pack[key].value for key in ids)
    if front_here - front_fast < FRONT_GROWTH_DEG or abs(rear_here - rear_fast) > REAR_FLAT_DEG:
        return None
    steering = evidence_id("steer_rate_mid", symptom.corner_class)
    return Alternative(
        kind=Kind.driver_input,
        statement=(
            f"Front slip is {front_here:.1f} degrees here and {front_fast:.1f} in fast corners, "
            f"while rear slip stays at {rear_here:.1f} and {rear_fast:.1f}. A balance problem "
            "would move the rear too. This is the pattern of steering past the front tyre's "
            "peak in slow corners (front saturation), and of steering geometry at large angles."
        ),
        supported=True,
        evidence=ids + ([steering] if steering in pack else []),
    )


def for_symptom(pack: Pack, symptom: Symptom) -> list[Alternative]:
    """Driver input and conditions, always both. Driver input is `supported` when the pack
    shows front saturation or the speed pattern for an understeer symptom."""
    driver = None
    if "understeer" in symptom.key:
        driver = _saturation(pack, symptom) or _speed_pattern(pack, symptom)
    return [driver or DRIVER_NOT_SHOWN, CONDITIONS]
