"""The explanation that is shown: a model's reply when it passes the gate, the template text
when there is no model, the model fails, or its reply is rejected."""

import logging

from paddock_engine.contracts.diagnosis_schema import Explanation, Provider, RankedHypothesis
from paddock_engine.contracts.explain_reply_schema import ExplainReply
from paddock_engine.explain.pack import EvidencePack
from paddock_engine.explain.provider import LLMProvider, NullProvider, ProviderError
from paddock_engine.explain.validate import validate

log = logging.getLogger(__name__)


def _shown(provider: Provider, reply: ExplainReply, rejected: str) -> Explanation:
    return Explanation(
        provider=provider,
        summary=reply.summary,
        ranking=[
            RankedHypothesis(hypothesis=item.hypothesis, why=item.why, cites=item.cites)
            for item in reply.ranking
        ],
        rejected=rejected,
    )


def explain(pack: EvidencePack, provider: LLMProvider, note: str = "") -> Explanation:
    """`note` is why a requested provider is not in use (explain/config.py)."""
    template = NullProvider().complete(pack)
    if provider.name is Provider.none:
        return _shown(Provider.none, template, note)
    try:
        reply = provider.complete(pack)
    except ProviderError as error:
        log.warning("explain: provider failed", extra={"provider": provider.name.value})
        return _shown(Provider.none, template, f"{provider.name.value} gave no reply: {error}")
    reason = validate(reply, pack)
    if reason:
        log.warning("explain: reply rejected", extra={"provider": provider.name.value})
        return _shown(Provider.none, template, f"The reply was rejected: {reason}")
    return _shown(provider.name, reply, "")
