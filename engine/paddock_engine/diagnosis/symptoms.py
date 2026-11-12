"""Symptoms: where the evidence pack shows the car, or the driver, in trouble.

A symptom is a reading beyond a threshold, never a verdict. Whether it is the car is what the
alternatives (diagnosis/alternatives.py) and then an experiment are for.
"""

from dataclasses import dataclass

from paddock_engine.diagnosis.evidence import Pack, evidence_id

# Neutral band of front-minus-rear slip: 12 % of the slip the axles run, at least 0.4 degrees
# (v0 `advisor.band`).
BAND_SHARE = 0.12
BAND_MIN_DEG = 0.4
EVENTS_PER_PASS = 0.2  # lockups or wheelspin events per pass before it is a symptom (v0)
EXIT_COUNTERSTEER_S = 0.12  # seconds of opposite lock per pass on exit (v0)
AERO_CLASS = "fast"  # corners above 150 km/h: the aero rows of the rule table

CLASS_WORDS = {"slow": "slow corners", "medium": "medium-speed corners", "fast": "fast corners"}
PHASE_WORDS = {"entry": "on entry", "mid": "mid-corner", "exit": "on exit"}


@dataclass(frozen=True)
class Symptom:
    key: str  # stable id: <what>.<corner class>
    rule: str  # row of the rule table
    phase: str
    corner_class: str
    score: float  # how far beyond the threshold, in thresholds
    statement: str
    evidence: tuple[str, ...]


def _rule_for_balance(phase: str, corner_class: str, direction: str) -> str:
    if phase != "mid":
        return f"{phase}_{direction}"
    return f"mid_{direction}_{'high' if corner_class == AERO_CLASS else 'low'}"


def _balance(pack: Pack, phase: str, corner_class: str) -> Symptom | None:
    front_id = evidence_id(f"front_slip_{phase}_deg", corner_class)
    rear_id = evidence_id(f"rear_slip_{phase}_deg", corner_class)
    if front_id not in pack or rear_id not in pack:
        return None
    front, rear = pack[front_id].value, pack[rear_id].value
    band = max(BAND_MIN_DEG, BAND_SHARE * (front + rear) / 2.0)
    excess = (front - rear) / band
    if abs(excess) < 1.0:
        return None
    understeer = excess > 0
    where = PHASE_WORDS[phase] + (
        "" if corner_class == "all" else f" in {CLASS_WORDS[corner_class]}"
    )
    more, less = ("front", "rear") if understeer else ("rear", "front")
    high, low = (front, rear) if understeer else (rear, front)
    return Symptom(
        key=f"{phase}_{'understeer' if understeer else 'oversteer'}.{corner_class}",
        rule=_rule_for_balance(phase, corner_class, "us" if understeer else "os"),
        phase=phase,
        corner_class=corner_class,
        score=abs(excess),
        statement=(
            f"{'Understeer' if understeer else 'Oversteer'} {where}: the {more} axle slips "
            f"{high:.1f} degrees against {low:.1f} at the {less}."
        ),
        evidence=(front_id, rear_id),
    )


def _event(
    pack: Pack, metric: str, threshold: float, key: str, phase: str, what: str
) -> Symptom | None:
    reading = pack.get(evidence_id(metric, "all"))
    if reading is None or reading.value <= threshold:
        return None
    return Symptom(
        key=f"{key}.all",
        rule=key,
        phase=phase,
        corner_class="all",
        score=reading.value / threshold,
        statement=f"{what}: {reading.value:.2f} per pass over {reading.passes} passes.",
        evidence=(reading.id,),
    )


def _exit_slides(pack: Pack) -> Symptom | None:
    """Caught slides on exit are oversteer evidence even when the slip angles look balanced."""
    reading = pack.get(evidence_id("countersteer_exit_s", "all"))
    if reading is None or reading.value <= EXIT_COUNTERSTEER_S:
        return None
    return Symptom(
        key="exit_oversteer.all",
        rule="exit_os",
        phase="exit",
        corner_class="all",
        score=reading.value / EXIT_COUNTERSTEER_S,
        statement=f"Oversteer on exit: {reading.value:.2f} seconds of opposite lock per pass.",
        evidence=(reading.id,),
    )


def find(pack: Pack) -> list[Symptom]:
    """Every symptom in the pack, strongest first. Entry and exit are read over all corners,
    mid-corner per speed class: mechanical grip and aero are different problems."""
    found = [_balance(pack, "mid", corner_class) for corner_class in ("slow", "medium", "fast")]
    found += [_balance(pack, phase, "all") for phase in ("entry", "exit")]
    found += [
        _event(pack, "front_lockups", EVENTS_PER_PASS, "lock_f", "brake", "Front lockups"),
        _event(pack, "rear_lockups", EVENTS_PER_PASS, "lock_r", "brake", "Rear lockups"),
        _event(pack, "wheelspin", EVENTS_PER_PASS, "spin", "exit", "Wheelspin on exit"),
    ]
    symptoms = {s.key: s for s in found if s is not None}
    slides = _exit_slides(pack)
    if slides is not None and "exit_understeer.all" not in symptoms:
        known = symptoms.get(slides.key)
        symptoms[slides.key] = slides if known is None or slides.score > known.score else known
    return sorted(symptoms.values(), key=lambda s: -s.score)
