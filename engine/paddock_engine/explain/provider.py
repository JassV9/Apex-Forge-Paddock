"""Who writes the explanation: the provider interface and the provider that needs no model."""

from typing import Protocol

from paddock_engine.contracts.diagnosis_schema import Hypothesis, Provider
from paddock_engine.contracts.explain_reply_schema import ExplainReply, RankedReply
from paddock_engine.explain.pack import EvidencePack

NOTHING_FOUND = (
    "No symptom is beyond its threshold in this session's valid passes. That is not proof "
    "the car is balanced: corners with too few passes are not judged."
)
NOT_A_VERDICT = (
    "None of this is a verdict. Each candidate is an experiment to run, and the session's "
    "conditions were not compared."
)
NO_ALTERNATIVE_SHOWN = "The data shows no other explanation, and does not exclude one."


class ProviderError(Exception):
    """The provider gave no usable reply. The message never contains a key."""


class LLMProvider(Protocol):
    name: Provider

    def complete(self, pack: EvidencePack) -> ExplainReply:
        """The pack in words. Raises ProviderError when there is no usable reply."""
        ...


def _supported(hypothesis: Hypothesis) -> tuple[str, list[str]]:
    """Statement and evidence of the alternatives the data speaks for."""
    shown = [a for a in hypothesis.alternatives if a.supported]
    cites = [cite for a in shown for cite in a.evidence]
    return " ".join(a.statement for a in shown), cites


def _ranked(hypothesis: Hypothesis) -> RankedReply:
    alternative, cites = _supported(hypothesis)
    why = f"{hypothesis.statement} {alternative or NO_ALTERNATIVE_SHOWN}"
    return RankedReply(
        hypothesis=hypothesis.key,
        why=why,
        cites=list(dict.fromkeys([*hypothesis.evidence, *cites])),
    )


class NullProvider:
    """Template text from the hypotheses themselves. No model, no network, no key."""

    name = Provider.none

    def complete(self, pack: EvidencePack) -> ExplainReply:
        if not pack.hypotheses:
            return ExplainReply(summary=NOTHING_FOUND, ranking=[])
        top = pack.hypotheses[0]
        alternative, _ = _supported(top)
        parts = [f"Strongest reading: {top.statement}", alternative, NOT_A_VERDICT]
        return ExplainReply(
            summary=" ".join(part for part in parts if part),
            ranking=[_ranked(hypothesis) for hypothesis in pack.hypotheses],
        )
