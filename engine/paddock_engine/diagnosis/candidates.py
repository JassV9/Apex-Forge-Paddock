"""From a row of the rule table to experiments on this car's own setup items."""

import re
from collections.abc import Sequence

from paddock_engine.contracts.diagnose_request_schema import DiagnoseItem
from paddock_engine.contracts.diagnosis_schema import Candidate, Direction, Expectation
from paddock_engine.diagnosis.rules import DIRECTED_ROLES, RULES, Knob
from paddock_engine.diagnosis.symptoms import Symptom

NO_SETUP = "This session has no setup data, so no item of the car can be named."
# One wheel of a left/right pair, in the item's name (CAMBER_LF) or its label ("Camber LF").
SIDE_IN_NAME = re.compile(r"_[LR][FR](_|$)")
SIDE_IN_LABEL = re.compile(r"(^|[\s(])[LR][FR]([\s)]|$)")
EVENT_CHANNELS = {"lock_f": "front_lockups", "lock_r": "rear_lockups", "spin": "wheelspin_s"}


def expectations(symptom: Symptom) -> list[Expectation]:
    """Which way the channels move if a change cures the symptom. Corner time comes first: a
    cure that costs time is not one."""
    if symptom.rule in EVENT_CHANNELS:
        channel = EVENT_CHANNELS[symptom.rule]
    else:
        axle = "front" if "understeer" in symptom.key else "rear"
        channel = f"{axle}_slip_{symptom.phase}_deg"
    return [
        Expectation(channel="corner_time_s", direction=Direction.decrease),
        Expectation(channel=channel, direction=Direction.decrease),
    ]


def _is_one_wheel(item: DiagnoseItem) -> bool:
    return bool(SIDE_IN_NAME.search(item.name.upper()) or SIDE_IN_LABEL.search(item.label.upper()))


def _name(item: DiagnoseItem) -> str:
    return item.label or item.name


def _candidate(item: DiagnoseItem, knob: Knob, expects: list[Expectation]) -> Candidate | str:
    """The experiment for one item, or the reason it cannot be one."""
    if _is_one_wheel(item):
        return f"{_name(item)}: a left/right pair cannot be changed in one experiment yet."
    if knob.role not in DIRECTED_ROLES or (knob.role == "wing" and not item.axle):
        return f"{_name(item)}: which way its value works is not known."
    clicks = knob.sign * (-1 if item.shown_scale < 0 else 1)
    target = item.value + clicks * item.step
    if not item.min <= target <= item.max:
        return f"{_name(item)}: already at the end of its range."
    return Candidate(
        parameter=item.name, label=item.label, clicks=clicks, why=knob.why, expects=expects
    )


def for_symptom(
    symptom: Symptom, items: Sequence[DiagnoseItem], present: frozenset[str]
) -> tuple[list[Candidate], list[str]]:
    """Candidates in the rule table's order, and the knobs that cannot be tested with the
    reason. `present` is the rule keys of every symptom found: a knob is skipped when the
    symptom that rules it out is there."""
    if not items:
        return [], [NO_SETUP]
    candidates: list[Candidate] = []
    not_testable: list[str] = []
    expects = expectations(symptom)
    for knob in RULES.get(symptom.rule, ()):
        matches = [
            i for i in items if i.role == knob.role and (not knob.axle or i.axle == knob.axle)
        ]
        if matches and knob.unless in present:
            not_testable.append(
                f"{_name(matches[0])}: ruled out by another symptom ({knob.unless})."
            )
            continue
        for item in matches:
            result = _candidate(item, knob, expects)
            if isinstance(result, Candidate):
                candidates.append(result)
            else:
                not_testable.append(result)
    return candidates, not_testable
