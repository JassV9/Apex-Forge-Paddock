# ADR 0011: A sensitivity profile that is re-derived, priors that are flagged, and a ranking that explains itself

Status: accepted (2026-11-12)

## Context

After phase 4 the system can propose experiments and judge them, and forgets what it
learned: the fifth experiment on a car is planned as if it were the first. Phase 5 adds the
memory (`docs/design.md`, sections E and F): what each setup item does on each car, how sure
that is, and which experiment is worth the next laps.

Three things can go wrong here that the rest of the system was built to prevent. A number
borrowed from another car can be read as a measurement. An effect measured once can harden
into a fact. And a ranking can become an oracle nobody can check.

## Decision

**The profile is derived, never edited.** A row of `sensitivity_effect` is one item, a signed
number of clicks, a channel and a corner class. Core rebuilds every car's rows from the
`result` rows of its finished one-factor experiments on each `GET /sensitivity`, in one
transaction, and stores which method made them (`model_version`). There is no update in
place, so there is nothing to drift: change the method and every row is re-derived from the
same ledger. Experiments still being driven, aborted ones and champion re-checks are left
out, and so is an experiment judged at another interval level than the one the engine is
told.

**Pooling is inverse-variance weighting** (`engine/paddock_engine/sensitivity/pooling.py`).
Each experiment's interval gives a standard error; repeated experiments on the same change
are averaged by precision. The pooled interval is classed against the band of practical
equivalence the experiments are judged with (5 ms on corner time, each guardrail's tolerance
on its own channel): lower, higher, negligible, or inconclusive. "Negligible" therefore
means the whole interval is inside the band. A change that was measured badly is
inconclusive, not negligible.

**Non-linearity is flagged from measurements only.** One click up and one click down should
cancel; two clicks should do twice what one does. A row is flagged when a departure from
either is outside the band with the same confidence the effects themselves have, and the
note says which check failed. Nothing is extrapolated from a flagged item.

**A prior is always shown as a prior.** Another car's measured effect is offered for an item
that does the same job (same role, kind and axle in the agent's catalog), and only when each
car has exactly one such item: a left and a right damper share a job, and one side says
nothing certain about the other. Clicks are restated when one car's menu runs the other way.
The engine then shrinks the number towards zero, keeping half of it for a car with the same
drivetrain and a quarter otherwise, and widens its interval so that both "what the other car showed" and
"no effect at all" stay inside it. The row carries `is_prior`, the lender's name and a note,
has no experiments and no passes, and is replaced the moment the car measures the change
itself. The dashboard words it "Prior from X, not measured on this car" and draws it hollow.

**The ranking is expected gain per lap** (`engine/paddock_engine/optimize/rank.py`). Every
candidate of the session's diagnosis gets a belief about its effect on the judged channel:
the car's measurement, a prior, or, with neither, "unknown" (centred on zero, three bands
wide). Expected gain is the expected improvement beyond the band, so a wide belief is worth
more than a narrow one with the same mean. Laps are those of an A-B-A-B plan able to detect
the expected effect, at least the standard plan. Each proposal says what it rests on and
gives the interval; when the whole interval is a gain, the propose form starts its
prediction from it.

**Only the car's own measurement rules a change out.** A change this car measured as
negligible, or as worse, is not ranked; it is listed with the reason and the numbers. A prior
never rules anything out, and neither does an inconclusive measurement. This is the
acceptance rule "never proposes a knob classed negligible unless uncertainty justifies it":
classed negligible already means the uncertainty is small.

**The scorecard is arithmetic** (`GET /scorecard`): experiments, how many were judged, and
how many of the predictions written down before driving came true, by where the hypothesis
came from. It uses the newest engine verdict of each experiment.

## Consequences

- The profile is always consistent with the ledger and costs one or two engine calls per car
  per request. With a handful of cars that is nothing; with hundreds it would want a cache
  keyed on the ledger's newest verdict.
- `GET /sensitivity` writes. It is safe to repeat and two requests cannot interleave their
  rows (the table is locked for the rebuild).
- The dashboard can show where every number came from: the experiment ids are on the row.

## What is not built, and known limits

- **The surrogate model with Thompson sampling** that `docs/design.md` names for cars with 20
  or more experiments. No car has that many. The stage is reported (`cold_start`, `learning`,
  `optimizing`) and all three are ranked the same way.
- **No track or conditions dimension.** The design's `track` and `conditions_hash` columns
  are not there: effects from different tracks are pooled. Corner class is the only split.
- **Similarity is drivetrain only.** Mass, weight distribution and aero class are stored on
  the car and not used. The weights 0.5 and 0.25 are judgement, not fitted.
- **A click is not a unit.** One click of a bar is a different stiffness on every car. The
  shrinkage is what makes the prior honest about that; it does not correct it.
- **Paired items** (camber or pressure per wheel) are neither measured by an experiment nor
  lent between cars.
- **Humans rarely produce "negligible".** With 28 ms of scatter per pass, an interval inside
  a 5 ms band takes about 90 laps per setup. The acceptance test uses a repeatable driver
  (8 ms), which is what the Ghost Driver is for. With a human the honest class is
  usually "inconclusive", and such a change stays in the ranking.
- Nothing here was run in Assetto Corsa.

## Rejected

- **Updating rows as experiments finish.** A second source of truth beside the ledger, and
  a row could survive the experiment it came from.
- **A hierarchical model across cars.** The right tool once there are many cars with many
  experiments each. With two cars it would dress up two numbers as a distribution.
- **Letting a prior skip a candidate.** It would let another car's result stand in for an
  experiment on this one, which is the mistake v0 made with its rule table.
- **Ranking by expected gain alone.** It would always prefer the knob already known to work
  and never learn what the others do.
