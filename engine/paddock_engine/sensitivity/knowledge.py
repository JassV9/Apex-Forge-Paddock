"""What a stratum's experiments establish, explicitly (ADR 0033).

Five kinds of knowledge are kept apart, and only the first three are measurements of this car:

- **confirmed**: at least 3 experiments and the pooled HKSJ interval lies wholly beyond the
  margin (an improvement or a regression, by the sign);
- **neutral**: at least 3 experiments and the pooled interval lies wholly inside the margin: two
  one-sided tests at the interval's level, on the pooled effect;
- **preliminary**: the interval excludes zero (fewer than 3 experiments, or not beyond the
  margin), the direction is likely;
- otherwise **insufficient**, and always for a stale row (the current engine could not re-derive
  its experiments, so the numbers are the ledger's, typically from an engine before ADR 0029);
- a prior from another car is not a level at all: it lives in its own table and type
  (sensitivity/priors.py) and has no class;
- general vehicle-dynamics hypotheses (the parameter-to-observable map,
  response/parameters.toml) say what a change should move; they are never stored as findings.

Two or fewer experiments never make a pooled confirmation or neutrality: with k < 3 the
between-experiment spread is not estimable, so a claim about "this car" would rest on one or two
days of driving.
"""

from collections.abc import Sequence

from paddock_engine.contracts.sensitivity_response_schema import KnowledgeLevel
from paddock_engine.sensitivity.half_normal import half_normal
from paddock_engine.sensitivity.pooling import HKSJ_FROM, Estimate, Pooled, hksj, single


def combine(estimates: Sequence[Estimate], margin: float | None, level: float) -> Pooled:
    """One experiment as it is; two with a half-normal prior on tau scaled to the margin (the
    largest standard error when the channel has none); three or more by HKSJ."""
    if not estimates:
        raise ValueError("nothing to pool")
    if len(estimates) == 1:
        return single(estimates[0], level)
    if len(estimates) < HKSJ_FROM:
        scale = margin if margin else max(e.se for e in estimates)
        return half_normal(estimates, scale, level)
    return hksj(estimates, level)


def knowledge(pooled: Pooled, margin: float | None, stale: bool) -> KnowledgeLevel:
    if stale:
        return KnowledgeLevel.insufficient
    if pooled.experiments >= HKSJ_FROM and margin is not None:
        if pooled.low > margin or pooled.high < -margin:
            return KnowledgeLevel.confirmed
        if -margin < pooled.low and pooled.high < margin:
            return KnowledgeLevel.neutral
    if pooled.low > 0.0 or pooled.high < 0.0:
        return KnowledgeLevel.preliminary
    return KnowledgeLevel.insufficient
