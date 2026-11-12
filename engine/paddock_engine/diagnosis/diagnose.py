"""Hypotheses from a session: the work behind POST /diagnose, before any words are added.

Everything here is deterministic. The same passes give the same evidence, the same
hypotheses and the same order.
"""

from paddock_engine.contracts.diagnose_request_schema import DiagnoseRequest
from paddock_engine.contracts.diagnosis_schema import CornerClass, Evidence, Hypothesis, Phase
from paddock_engine.diagnosis import alternatives, candidates, evidence, symptoms

# A symptom the driver's inputs explain is still worth testing, after the ones they do not.
DRIVER_DISCOUNT = 0.5


def diagnose(request: DiagnoseRequest) -> tuple[list[Evidence], list[Hypothesis]]:
    """The evidence pack and the hypotheses it supports, strongest first."""
    pack = evidence.build(request.passes)
    found = symptoms.find(pack)
    present = frozenset(s.rule for s in found)
    hypotheses = []
    for symptom in found:
        others = alternatives.for_symptom(pack, symptom)
        tests, not_testable = candidates.for_symptom(symptom, request.items, present)
        explained_by_driver = any(a.supported for a in others)
        hypotheses.append(
            Hypothesis(
                key=symptom.key,
                statement=symptom.statement,
                phase=Phase(symptom.phase),
                corner_class=CornerClass(symptom.corner_class),
                score=symptom.score * (DRIVER_DISCOUNT if explained_by_driver else 1.0),
                evidence=list(symptom.evidence),
                alternatives=others,
                candidates=tests,
                not_testable=not_testable,
            )
        )
    hypotheses.sort(key=lambda h: -h.score)
    return list(pack.values()), hypotheses
