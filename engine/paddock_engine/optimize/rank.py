"""Which experiment to run next: POST /propose.

Every candidate gets a belief about its effect on the channel experiments are judged on:
this car's own measurement, a prior from another car, or, with neither, "unknown". The list
is ordered by expected gain per lap of driving. Expected gain is the expected improvement
beyond the band of practical equivalence, so a wide belief is worth more than a narrow one
with the same mean: an unknown knob earns a look, a knob known to do nothing does not.

A change this car has measured as negligible, or as clearly worse, is not proposed. A prior
never rules a change out: what another car did is not a measurement of this one.

Not built: the surrogate model with Thompson sampling that docs/design.md names for cars
with 20 or more experiments. No car has that many; the stage is reported and ranked as
"learning" until it does.
"""

from dataclasses import dataclass

from scipy.stats import norm

from paddock_engine.contracts.proposals_schema import (
    Basis,
    Proposal,
    Proposals,
    SkippedCandidate,
    Stage,
)
from paddock_engine.contracts.propose_request_schema import (
    Better,
    EffectClass,
    ProposeCandidate,
    ProposeRequest,
    SensitivityEffect,
)
from paddock_engine.stats.mde import passes_needed

LEVEL = 0.90
# Belief about a change nobody has measured: centred on no effect, three bands wide.
UNKNOWN_SD_IN_ROPES = 3.0
# An experiment must at least be able to decide an effect of two bands.
SMALLEST_TARGET_IN_ROPES = 2.0
BLOCKS = 4  # A-B-A-B; each block starts with an out-lap
LEARNING_FROM, OPTIMIZING_FROM = 5, 20


@dataclass(frozen=True)
class Belief:
    gain: float  # expected improvement per corner pass, positive is better
    sd: float
    basis: Basis
    source: str = ""


def _stage(experiments: int) -> Stage:
    if experiments >= OPTIMIZING_FROM:
        return Stage.optimizing
    return Stage.learning if experiments >= LEARNING_FROM else Stage.cold_start


def _band(request: ProposeRequest) -> float:
    """The size that counts as an effect: the band, or the scatter when there is none."""
    return request.rope if request.rope > 0 else request.sigma


def _effect_of(candidate: ProposeCandidate, request: ProposeRequest) -> SensitivityEffect | None:
    """The profile's row for this change on the judged channel over all corners; a
    measurement before a prior."""
    rows = [
        row
        for row in request.effects
        if row.parameter == candidate.parameter
        and row.clicks == candidate.clicks
        and row.channel == request.channel
        and row.corner_class.value == "all"
    ]
    rows.sort(key=lambda row: row.is_prior)
    return rows[0] if rows else None


def _belief(effect: SensitivityEffect | None, request: ProposeRequest) -> Belief:
    if effect is None:
        return Belief(0.0, UNKNOWN_SD_IN_ROPES * _band(request), Basis.none)
    sign = -1.0 if request.better is Better.lower else 1.0
    sd = (effect.ci_high - effect.ci_low) / (2.0 * float(norm.ppf(0.5 + LEVEL / 2.0)))
    basis = Basis.prior if effect.is_prior else Basis.measured
    return Belief(sign * effect.estimate, sd, basis, effect.prior_source)


def expected_gain(belief: Belief, rope: float) -> float:
    """E[max(gain - rope, 0)] for a normal belief."""
    if belief.sd <= 0:
        return max(belief.gain - rope, 0.0)
    distance = (belief.gain - rope) / belief.sd
    return float((belief.gain - rope) * norm.cdf(distance) + belief.sd * norm.pdf(distance))


def laps_needed(belief: Belief, request: ProposeRequest) -> int:
    """Laps of an A-B-A-B experiment able to detect the expected effect: both arms, plus an
    out-lap per block. Never fewer than the standard plan."""
    target = max(abs(belief.gain), SMALLEST_TARGET_IN_ROPES * _band(request))
    per_arm = passes_needed(request.sigma, target, request.corners)
    push_laps = max(2 * per_arm, BLOCKS * request.laps_per_block)
    return push_laps + BLOCKS


def _why_skipped(effect: SensitivityEffect | None, request: ProposeRequest) -> str:
    """Only this car's own measurement can rule a change out."""
    if effect is None or effect.is_prior:
        return ""
    span = f"{effect.ci_low:+.3f} to {effect.ci_high:+.3f} over {effect.n_passes} passes"
    if effect.effect_class is EffectClass.negligible:
        return f"Measured on this car as negligible on {request.channel}: {span}."
    worse = EffectClass.positive if request.better is Better.lower else EffectClass.negative
    if effect.effect_class is worse:
        return f"Measured on this car as worse on {request.channel}: {span}."
    return ""


def _proposal(candidate: ProposeCandidate, belief: Belief, request: ProposeRequest) -> Proposal:
    half = float(norm.ppf(0.5 + LEVEL / 2.0)) * belief.sd
    gain, laps = expected_gain(belief, request.rope), laps_needed(belief, request)
    return Proposal(
        parameter=candidate.parameter,
        label=candidate.label,
        clicks=candidate.clicks,
        why=candidate.why,
        hypotheses=candidate.hypotheses,
        basis=belief.basis,
        prior_source=belief.source,
        expected_gain=gain,
        gain_low=belief.gain - half,
        gain_high=belief.gain + half,
        laps_needed=laps,
        value_per_lap=gain / laps,
    )


def propose(request: ProposeRequest) -> Proposals:
    ranked, skipped = [], []
    for candidate in request.candidates:
        effect = _effect_of(candidate, request)
        reason = _why_skipped(effect, request)
        if reason:
            skipped.append(
                SkippedCandidate(
                    parameter=candidate.parameter,
                    label=candidate.label,
                    clicks=candidate.clicks,
                    reason=reason,
                )
            )
            continue
        ranked.append(_proposal(candidate, _belief(effect, request), request))
    ranked.sort(key=lambda p: (-p.value_per_lap, p.parameter, p.clicks))
    return Proposals(stage=_stage(request.experiments), ranked=ranked, skipped=skipped)
