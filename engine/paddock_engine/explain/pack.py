"""The evidence pack a language model is given, and the only things it may refer to.

The pack holds computed evidence and the hypotheses built on it. It holds no key, no file
path and nothing about the driver beyond the numbers of the session.
"""

import json
from collections.abc import Sequence
from dataclasses import dataclass

from paddock_engine.contracts.diagnosis_schema import Evidence, Hypothesis, Kind

SHOWN_DECIMALS = 2  # precision of the values in the prompt


@dataclass(frozen=True)
class EvidencePack:
    evidence: Sequence[Evidence]
    hypotheses: Sequence[Hypothesis]

    def evidence_ids(self) -> frozenset[str]:
        return frozenset(e.id for e in self.evidence)

    def hypothesis_keys(self) -> frozenset[str]:
        return frozenset(h.key for h in self.hypotheses)

    def numbers(self) -> list[float]:
        """Every number of the pack: evidence values, pass counts and candidate clicks."""
        values = [e.value for e in self.evidence]
        values += [float(e.passes) for e in self.evidence]
        values += [float(c.clicks) for h in self.hypotheses for c in h.candidates]
        return values

    def vocabulary(self) -> frozenset[str]:
        """Every identifier of the pack. An identifier in a reply that is not here was made up."""
        words = {kind.value for kind in Kind}
        for item in self.evidence:
            words |= {item.id, item.metric}
        for hypothesis in self.hypotheses:
            words |= {hypothesis.key, hypothesis.key.split(".")[0]}
            for candidate in hypothesis.candidates:
                words |= {candidate.parameter, *(e.channel for e in candidate.expects)}
        return frozenset(words)

    def as_json(self) -> str:
        """The pack as the model sees it. Scores are left out: the order carries them."""
        body = {
            "evidence": [
                {"id": e.id, "value": round(e.value, SHOWN_DECIMALS), "passes": e.passes}
                for e in self.evidence
            ],
            "hypotheses": [h.model_dump(mode="json", exclude={"score"}) for h in self.hypotheses],
        }
        return json.dumps(body, indent=1)
