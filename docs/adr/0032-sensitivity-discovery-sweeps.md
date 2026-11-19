# ADR 0032: Sensitivity discovery sweeps

Status: accepted (2026-11-19). Implements section 6, Experiment D, and section 8's work package of
[the reliability investigation](../investigations/2026-11-18-ghost-driver-reliability.md). Sits
beside [ADR 0020](0020-ai-training-program.md) (training) and uses ADR 0026 (linked items), 0028
(pinned hint), 0029 (findings, looks, recommendations) and 0031 (response cells). **Unverified in
game**: nothing here ran in Assetto Corsa.

## Context

Training (ADR 0020) improves a setup: it drives the best-ranked change, keeps it when it is better
and moves on. It cannot say how the car *responds* to a setting. The investigation's section 1
found that with a plan-limited AI, corner time barely moves while the car's response does (rear
wing -1: time insufficient, rear slip +0.11 to +0.17 deg). Experiment D asks for the response curve
of one setting: rear ARB -1, +1, +2, +3, each against the baseline, the baseline back on the car
between steps, at a pinned hint, classified as linear, nonlinear, negligible or direction change
per phase and speed regime. That is a different procedure with different rules: nothing is
promoted, a step that is "only neutral" is still an answer, and every step must be measured
against the same baseline or the steps cannot be put on one curve.

## Decision

### 1. A sweep is its own program (`core/internal/discovery`)

`POST /sessions/{id}/sweeps {parameter, steps, ai_hint_share}` opens a sweep on the session's car
and track; `GET /sweeps` lists them; `POST /sweeps/{id}/stop` ends one. A background loop beside
training's (every 15 s) advances each open sweep. Like training it only calls the experiment
service's actions (`create`, `plan`, `start`, `evaluate`, `extend`, `revert`), so every rule of
the ledger holds. Training's code is unchanged.

**Refused at start** (nothing stored): no `ai_hint_share` or one outside 0.5..1.5 (422: steps
driven on whatever hint calibration learned last cannot be compared, ADR 0028); an item that is
not changeable on the car (422); no possible step (422); an experiment on the car, whoever drives
it, or an open training program (409: a sweep starts from the baseline and two programs would
fight over the car); another open sweep on the car (409, a unique index); a baseline the agent
never read back from the car (409, `setup_version.agent_hash` NULL: "verified" means the setup is
known to exist on the car as recorded).

**Steps within the item's real limits.** Each step is clicks from the baseline, clipped to the
item's min and max (`discovery/steps.go`); a step clipping makes 0 (the baseline is at that
limit) or a repeat of an earlier step is skipped, and every change is said in words
(`skipped`). Rear ARB at 3 of 0..5 with -1, +1, +2, +3 drives -1, +1, +2 and says "+3 would be 6,
beyond the maximum 5: clipped to +2, already a step; skipped". A linked group is one setting: a
sweep of `WING_9` sweeps `WING_1 + WING_9` under the leader, both members by the same clicks.

**One baseline.** The sweep records the baseline at start (`sweep.baseline_version_id`). Every
step is its own A-B-A-B experiment proposed with `CreateSweepStep` from the session; before each
step the sweep checks that the session's baseline is still that setup, and stops ("The baseline
moved from setup X to Y") when it is not.

**Experiments with purpose `sweep`.** Migration 00015 adds `experiment.purpose` (`improve`,
`sweep`, `recheck`; default `improve`; existing re-checks are marked `recheck`). The ledger
settles a sweep step differently in one place: an accepted or trade-off verdict is followed by
the baseline going back on the car in the same transaction ("A sweep step is measured, never
promoted"), as a rejection is; the champion is never touched and no re-check is counted. That is
what keeps the baseline the same for every step. The verdict itself (finding, recommendation,
results) is stored as for any experiment. The experiment's API reply now carries `purpose`.

**Per step:**

| Step's experiment | What the sweep does |
| --- | --- |
| proposed, planned | plans, starts (waits while another experiment is on the car) |
| running, laps missing | waits; stops the sweep when a block was driven at another AI hint than base x pinned share (`run_context`, tolerance 0.0005), or when nothing came from the step for 30 minutes (an AI failure: car stuck, game closed, watchdog) |
| running, every block driven | asks for the verdict (the same hint check first) |
| inconclusive, finding `invalid` or recommendation `stop_invalid` | puts the baseline back and stops the sweep, with the verdict's reason |
| inconclusive, recommendation `continue`, extension left | extends to the next pre-registered look (ADR 0029) |
| inconclusive, anything else (`stop_neutral`, `larger_perturbation`, `investigate_context`, `switch_metric`) | puts the baseline back and moves on: a sweep measures what it planned and never takes a larger step |
| aborted, discarded | stops the sweep ("the AI could not drive it or the driver stopped it") |
| reverted (after accepted, trade-off, rejected or the above) | reads the step's response cells once (`POST /response`), then starts the next step |

Reverting is recorded with the sweep's reason and `decided_by: training` (the ledger's value for
"a program decided"; a separate value would need a contract and migration change to the verdict
for no behaviour). A stopped or failed sweep leaves its open experiment as it is, for the driver
to abort or finish; the steps before it stand.

**Response cells.** Core now calls the engine's `POST /response` (ADR 0031) for each measured
step: `experiments.Service.ResponseRequest` sends the step's push-lap passes on the arm's setup
and from its driver (off-track ones as not valid), the covariates and lap codes the verdict uses,
the corner map, the sessions' Parquet directories and the item's role and axle. Only the cells the
item's parameter row expects (`expected` direct or proxy) are kept on `sweep_step.response`: 344
cells at 90 % would give about 34 "moves" by chance. A step the engine refuses (4xx, e.g. no
samples) keeps its verdict channels only; an engine that does not answer is asked again.

**When every step is measured** core sends the per-step effects to `POST /discovery/curve` and
stores the answer on `sweep.curve`; the sweep is `done`. Observables sent: every channel of each
step's verdict (corner time first, per speed class; guardrails, driver inputs, predictions; their
block-level intervals of ADR 0029) and the expected response cells per phase and regime.

### 2. The curve method (`engine/paddock_engine/discovery/curve.py`)

Each step's effect is f(c) - f(0) against the same baseline, so the baseline is the point (0, 0),
known exactly, and no intercept is fitted. Each step's standard error is its interval's half-width
over z at that interval's level: the success test's 0.90, or for the primary channel over all
corners the repeated confidence interval of the step's look (98.0, 96.4 or 94.3 %,
`stats.sequential.interval_level`). Reading a t interval with z overstates the standard error a
little: conservative. Steps at the same clicks are pooled by inverse variance; steps without a
finite positive standard error are dropped.

At one level (0.90 unless the request says otherwise), in this order:

1. **insufficient** with fewer than two distinct steps. One step is a point, not a curve: it never
   yields linear or nonlinear, however clear (a hypothesis test checks it for any value).
2. **negligible** when every step's interval lies inside +-margin: two one-sided tests at each step;
   "inside at every step" is an intersection-union test, so no correction. Margins: the verdict's
   for time, a guardrail's tolerance, a response cell's own (`response/metrics.toml`). Without a
   margin, never negligible.
3. **direction change** when a stretch between neighbouring steps (the baseline included) rises
   and another falls, each clear of zero, Bonferroni over the stretches; reported with the step at
   which it turns (the highest or lowest point). Both directions raising an observable (a V) turns
   at 0.
4. **nonlinear** when the quadratic term q of the weighted least-squares fit y = b c + q c^2
   (weights 1/se^2) is clear of zero.
5. **linear** when the slope of the weighted straight line through the baseline is clear of zero
   and q is not; the slope per click with its interval is reported.
6. **insufficient** otherwise.

Synthetic checks (`engine/tests/test_discovery_curve.py`, 400 sweeps of -1, +1, +2, +3 each): a
linear response (0.1 per click, se 0.02) is linear in at least 83 % (nonlinear at most 15 %, the
curvature test's nominal 10 %); quadratic (0.1 c + 0.05 c^2) nonlinear in at least 90 %; flat with
a margin 0.05 and se 0.01 negligible in at least 95 %, without a margin never negligible and
insufficient in at least 70 %; 0.2 c - 0.1 c^2 a direction change in at least 90 %, turning at +1.
Hypothesis checks that the shape does not depend on the sign, the order of the steps or the unit.

Each observable is classified on its own: across tens of observables some look linear by chance.
The classification describes; it decides nothing and never enters the champion or the ranking.

### 3. Web

A "Discover" card in the experiments tab: start a sweep (setting, steps "-1, 1, 2, 3", the pinned
hint share, required), each sweep's progress, steps and what clipping skipped, and when done one
plain sentence per curve with where it was measured, e.g. "rear slip rises with each click, about
0.1 deg per click, linear (exit, slow corners)".

### 4. Interactions: a staged plan (design only)

Nothing below is implemented beyond what a one-factor sweep needs.

1. **One-factor sweeps first.** One sweep per setting of interest (rear ARB, front ARB, both wings,
   diff preload, brake bias), steps inside the range, each step 8 blocks. They give each setting's
   sensitivity per observable (slope per click over its interval) and its shape.
2. **A 2^2 factorial on the two most sensitive settings,** ranked by |slope| / its standard error
   on the primary response observable, at one chosen step each (the largest step still linear, or
   the one before a direction change). Four arms against the baseline in one interleaved plan
   (baseline, A, B, A+B, repeated; at least 3 blocks per arm, so 12 to 16 blocks), effects of A, B
   and the A x B interaction as contrasts with block-level intervals, Holm over the three. It needs
   the experiment kind `factorial` (in the schema since ADR 0009, never built), a multi-arm plan in
   the engine and a run plan with more than two setups: a separate ADR.
3. **A response surface only if justified:** when the interaction is clear of zero and larger than
   the margin, or a one-factor sweep was nonlinear on the primary observable, a central composite
   design on those two settings (axial points at the sweep's steps). Not before: on the FH26's
   scatter a 20 ms effect needs the later looks (ADR 0029), and a surface multiplies the arms.

## Alternatives considered

- **Running the steps as one multi-arm experiment** (baseline and four variants interleaved). One
  baseline estimate for all steps and fewer baseline laps, but it needs the multi-arm plan of
  stage 2, five setup loads per round and a new evaluator; separate A-B-A-B experiments reuse the
  verdict, its looks and guards unchanged.
- **Letting training run sweeps.** Training's rules (rank, promote, never re-test an item) are the
  opposite of a sweep's (fixed item, never promote, every step).
- **Fitting each step's effect with an intercept** (ordinary regression on clicks). The baseline is
  known exactly: an intercept wastes a degree of freedom on something measured as zero by design.
- **Taking a larger step when the engine recommends it.** The sweep's steps are pre-registered; a
  bigger step is another sweep.
- **Classifying all 344 response cells.** About 34 would move by chance at 90 %; the item's
  parameter row says which cells it is expected to move.

## Consequences

- A four-step sweep is about 4 x 40 minutes of AI laps at 8 blocks, more when steps are extended;
  each step adds about 25 s of engine time for its response analysis.
- Sweep steps sit in the ledger like any experiment (purpose `sweep`) and feed the sensitivity
  profile as measurements, as any one-factor experiment does.
- Migrations 00013 and 00014 are written in parallel; on a fresh database 00015 applies after
  00012 as well as after them.

## Amendment: measure-only experiments (2026-11-19)

`POST /experiments` takes `measure_only: true` to file one experiment with purpose `sweep`
without a sweep around it: the same never-promote rule, so a hand-run series that is not a click
sweep (the grip-hint sweep of the reliability investigation, one hint per experiment) keeps one
baseline. Experiment 20 showed the need: accepted at look 3, it became the champion.

## Not verified

- **Unverified in game.** No sweep has been driven in Assetto Corsa. The loop is tested against a
  fake ledger and engine on PostgreSQL; the ledger's never-promote rule, the response request and
  the curve against PostgreSQL and on synthetic data.
- `POST /response` has not been called by core on a real experiment; its intervals are the
  effect model's bootstrap (ADR 0031's known limit C2) unless the response analysis has moved to
  block-level intervals.
- The 30-minute stall rule and the 0.0005 hint tolerance are judgement, not measurement.
