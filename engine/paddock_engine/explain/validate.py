"""The gate between a language model and the dashboard.

A reply is shown only when everything it refers to is in the evidence pack: the hypotheses
it ranks, the evidence it cites, every identifier in its prose and every number it writes.
A model that computes a difference, converts to a percentage or names a channel the session
never measured is rejected, and the template text is shown instead (explain/explain.py).
"""

import re

from paddock_engine.contracts.explain_reply_schema import ExplainReply
from paddock_engine.explain.pack import EvidencePack

# A number in prose. Digits inside a word (T12) are names, not numbers. The sign is dropped:
# "3.3-2.9" must not read as minus 2.9.
NUMBER = re.compile(r"(?<!\w)(?<!\d\.)\d+(?:\.\d+)?(?!\w)")
# snake_case words, with an optional corner class: metric ids, evidence ids, setup item ids.
IDENTIFIER = re.compile(
    r"\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)+(?:\.(?:all|slow|medium|fast))?\b"
)
MAX_DECIMALS = 6


def _texts(reply: ExplainReply) -> list[str]:
    return [reply.summary, *(item.why for item in reply.ranking)]


def _unknown_number(text: str, numbers: list[float]) -> str:
    """The first number in the text that is no pack number at the precision it is written."""
    for token in NUMBER.findall(text):
        decimals = len(token.partition(".")[2])
        if decimals > MAX_DECIMALS:
            return str(token)
        shown = {f"{abs(value):.{decimals}f}" for value in numbers}
        if f"{float(token):.{decimals}f}" not in shown:
            return str(token)
    return ""


def _unknown_identifier(text: str, vocabulary: frozenset[str]) -> str:
    for token in IDENTIFIER.findall(text):
        if token not in vocabulary:
            return str(token)
    return ""


def _references(reply: ExplainReply, pack: EvidencePack) -> str:
    keys, ids = pack.hypothesis_keys(), pack.evidence_ids()
    seen: set[str] = set()
    for item in reply.ranking:
        if item.hypothesis not in keys:
            return f"it ranks a hypothesis that is not in the pack: {item.hypothesis}"
        if item.hypothesis in seen:
            return f"it ranks a hypothesis twice: {item.hypothesis}"
        seen.add(item.hypothesis)
        unknown = [cite for cite in item.cites if cite not in ids]
        if unknown:
            return f"it cites evidence that is not in the pack: {unknown[0]}"
    return ""


def validate(reply: ExplainReply, pack: EvidencePack) -> str:
    """Why the reply must not be shown; empty when it may be."""
    reason = _references(reply, pack)
    if reason:
        return reason
    numbers, vocabulary = pack.numbers(), pack.vocabulary()
    for text in _texts(reply):
        identifier = _unknown_identifier(text, vocabulary)
        if identifier:
            return f"it names a metric or item that is not in the pack: {identifier}"
        number = _unknown_number(text, numbers)
        if number:
            return f"it states a number that is not in the pack: {number}"
    return ""
