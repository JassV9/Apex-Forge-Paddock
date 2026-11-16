# ADR 0018: A base setup from the car's own measurements, judged as its own experiment

Status: accepted (2026-11-16)

## Context

The owner's flow is "get in, hotlap, get a base setup, then small changes". v0 produced a
base setup from a rule table. Paddock has a better source: the car's sensitivity profile
(ADR 0011), which records what each item measurably did on this car. What was missing is a
way to turn those one-item measurements into one starting setup without breaking the rules
the rest of the system keeps: facts from deterministic code, every change a hypothesis with a
prediction, judged A-B-A-B against the baseline.

The first real case shows why the obvious rule is too strict. On the 488 GT3 the AI drove
ARB front -1 click and measured -4.2 ms per corner [-6.8, -1.6]: faster with 90 %
confidence, but "inconclusive" against the 5 ms band of practical equivalence, because the
interval reaches inside the band. A rule that only takes changes beyond the band would take
nothing, on almost every car, for a long time.

## Decision

**Inclusion rule.** `POST /sessions/{id}/base-setup` asks the engine
(`engine/paddock_engine/optimize/base_setup.py`, `POST /base-setup`) which rows of the car's
profile go in. A row qualifies when it is this car's own pooled measurement on corner time
over all corners and its interval lies wholly on the better side of zero (`ci_high < 0`).
Never taken, each with its reason in `left_out`:

- a prior from another car (`is_prior`): a prior never decides anything on this car;
- a row flagged non-linear: nothing is extrapolated from it (ADR 0011);
- a change this car measured worse on a guardrail (the same item and clicks, its class on
  `rear_slip_peak_deg` or `countersteer_s` beyond the tolerance in the bad direction);
- a paired item (another item of the car has the same role, kind and axle, as a camber per
  wheel): one side is never moved alone (ADR 0011);
- a change that would leave the item's range, or an item not on the baseline's menu.

One change per item: when several click amounts qualify, the best estimate wins and the
others are listed with the reason.

**Why zero and not the band.** The band answers "is this change worth an experiment of its
own"; a base setup does not rest on that answer. Each change only has to be measured as
better, and then the combination is driven as one experiment of kind `base`: baseline against
base setup, four interleaved blocks, the default success test and guardrails, with the
prediction written down before a lap is driven. If the combination does not clear the band, it
is rejected and reverted like any experiment, and the ledger keeps it. The strict test is
still applied, once, to the thing that becomes the champion.

**Prediction.** The sum of the estimates, with an interval from the summed variances (each
standard error read back from its interval). The note says that the items were measured one at
a time and that how they interact was not measured. Every chosen interval is below zero, so
the combined one is too: the prediction always says "faster".

**Champion.** A base experiment starts from the champion like any other (the baseline is the
setup an experiment on the session would start from: the champion, or before the first
experiment the session's latest setup). Accepted, its variant becomes the champion and counts
towards the next re-check; reverted, the baseline stays. `champion.go` needed no change: its
rules only single out `recheck`.

**Nothing qualifies.** Core answers 409 with the same body: what the car measured, each row
with its class, and the advice to calibrate the AI on the car and let it drive one-item
experiments first.

**No language model.** The choice is a filter and a sum over measured numbers. A model would
add nothing that can be checked and would put an unmeasured opinion into a setup.

Other details: the kind `base` is added to the contract enum and to the database CHECK
(migration 00007); base experiments are not read into the sensitivity profile (only `ofat`
is), so the combination never poses as a measurement of one item; the variant's setup version
is created by "base setup, experiment N" and its outbox files are named
`paddock-e<N>-base-<code name>.ini`, both arms alike, so the files stay blind.

## Consequences

- A car with one AI-driven one-item experiment can get a base setup the same day.
- A base setup that loses its own experiment costs one experiment and stays in the ledger.
- Changes that each help a little and hurt together are caught by the experiment, not
  predicted.

## Known limits

- Interactions between items are not measured or modelled. The combination's own verdict
  is the only check, and it does not say which item to blame.
- Corner classes are not used: a change faster over all corners and slower in fast corners
  can go in.
- Guardrails are checked on the measured change only; a guardrail that was never measured for
  an item does not keep it out.
- Effects are pooled across tracks (ADR 0011), so a base setup for one track can rest on
  another track's measurement.
- Nothing here was run in Assetto Corsa: the base experiment's setups loading and the AI
  driving it are **unverified in game** (they use the same run plan path as other
  experiments, which has been seen in game).

## Rejected

- **Only changes beyond the band.** Almost never anything to take, for the reason above.
- **Priors from similar cars.** That is the v0 rule table again, with numbers.
- **Promoting the combination without driving it.** The sum is a prediction, not a result.
- **A new kind of arm or a new state machine.** The re-check already compares two whole
  setup versions; a base experiment uses the same machinery.
