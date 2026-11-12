"""POST /diagnose: the deterministic diagnosis, then its explanation in words."""

from paddock_engine.contracts.diagnose_request_schema import DiagnoseRequest
from paddock_engine.contracts.diagnosis_schema import Diagnosis
from paddock_engine.diagnosis.diagnose import diagnose
from paddock_engine.explain.config import Choice
from paddock_engine.explain.explain import explain
from paddock_engine.explain.pack import EvidencePack


def report(request: DiagnoseRequest, llm: Choice) -> Diagnosis:
    evidence, hypotheses = diagnose(request)
    pack = EvidencePack(evidence=evidence, hypotheses=hypotheses)
    return Diagnosis(
        evidence=evidence,
        hypotheses=hypotheses,
        explanation=explain(pack, llm.provider, llm.note),
    )
