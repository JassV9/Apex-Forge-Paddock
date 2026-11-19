# ADR 0029: Block-level inference, pre-registered looks and finding classes

Status: accepted (2026-11-18). Amends [ADR 0008](0008-effect-model.md) (the interval and the
judgement), [ADR 0020](0020-ai-training-program.md) (when a program extends),
[ADR 0022](0022-covariates-the-arm-can-be-told-from.md) (session terms are never left out) and
[ADR 0025](0025-finished-laps-and-lap-noise.md) (every plan is eight blocks). Implements C2, C3,
C5, C6, C7, I3, I4 and I5 of the
[reliability audit](../investigations/2026-11-18-ghost-driver-reliability.md). (0028 is taken by
a change written in parallel.)

## Context

The audit found the verdict fragile, not demonstrably wrong:

- **C2.** The setup changes only between run blocks, so it is told apart from about B - 2 block
  contrasts. The interval resampled laps inside blocks and used laps - blocks - 1 degrees of
  freedom (15 at 8 blocks of 3 laps). An error shared by a whole block (a refuel, a restart, the
  AI planning differently) was invisible to it: with 6 ms block shocks its 90 % interval covered
  77 % at 8 blocks.
- **C3.** Training extends a promising inconclusive experiment, a human can extend twice. Judging
  each look with the same 90 % interval accepted a change at the band edge 8.5 to 12 % of the
  time instead of 5 %.
- **C5, C6, C7.** Off-track passes vanished silently; a manual `evaluate` could judge an unfinished
  plan; core never checked a pass ran on its arm's setup or came from the experiment's driver.
- **I3, I4.** "Rejected" meant both worse and negligible; "inconclusive" lumped "clearly better,
  size unclear" with "no information". Three driver inputs were each tested at 10 %, about 27 %
  false "driver adapted" under the null. Speed classes of one corner were stored (experiment 5's
  fast class: +-330 s).
- **I5.** Time in session restarted at 0 after a game restart (experiment 16's extension blocks
  looked early).

## Decision

### 1. The interval is block-level

The point estimate and covariate slopes stay those of the weighted random-intercept fit (ADR 0008,
0023). The standard error comes from the blocks (`engine/paddock_engine/stats/block_level.py`):

1. the fit's residuals (arm, covariates and corner levels removed), corner-weighted, averaged per
   run block;
2. those block means regressed on the arm and on the block means of the **block-varying
   covariates**: a covariate is block-varying when more than half of its variation (within
   corners, the arm removed) lies between blocks. Time since the start always is; a session term
   is; fuel is when a block was driven on another tank (experiment 15, block 1);
3. the arm's standard error from that regression, with **B - 2 - p_b degrees of freedom** (p_b
   block-varying covariates). With no block-varying covariate this is the two-sample t on the
   block means. Fewer than 1 degree of freedom: not estimable.

The interval is estimate +- t x that standard error. The lap bootstrap is gone: the task asked for
the larger of the two standard errors, and that rule was measured and rejected (table below). Both
standard errors estimate the same thing when there are no block shocks, and the larger of two
noisy estimates is biased up: it covered 95 to 100 % where the block-level one alone covers 88 to
92 %. Nothing left uses the bootstrap, so it was removed rather than kept as dead code.

A/A, 1000 simulated experiments per cell (FH26 pass scatter of experiment 17's corners, 5.5 to
147 ms, 4 ms lap noise, fuel refilled per block, time since start; `tests/block_simulation.py`).
Coverage of the 90 % interval and confirmed findings:

| Blocks | Block shock | Block-level (chosen) | Confirmed | Laps as independent units (ADR 0008's view) |
| --- | --- | --- | --- | --- |
| 4 | 0 ms | 90.4 % | 0 | 82.9 % |
| 4 | 3 ms | 90.6 % | 0 | 79.4 % |
| 4 | 6 ms | 88.5 % | 0 | 66.7 % |
| 8 | 0 ms | 88.6 % | 0.5 % | 87.1 % |
| 8 | 3 ms | 91.0 % | 0.9 % | 85.1 % |
| 8 | 6 ms | 90.4 % | 2.0 % | 77.5 % |
| 12 | 0 ms | 87.1 % | 0.1 % | 87.2 % |
| 12 | 3 ms | 89.6 % | 0.9 % | 84.0 % |
| 12 | 6 ms | 90.6 % | 0.7 % | 78.4 % |

A second seed gave 89.0 to 90.4 %. With the old engine itself (its lap bootstrap, 300 per cell):
89, 88 and 77 % at 8 blocks for 0, 3 and 6 ms, 93, 88 and 81 % at 4. The standard error rules
on the same experiments (400 per cell):

| Blocks, shock | max(bootstrap, block), block df | block only, block df | block only, time not charged |
| --- | --- | --- | --- |
| 4, 0 ms | 100.0 % (46 ms) | 87.8 % (27 ms) | 68.2 % |
| 8, 0 ms | 96.2 % (8.4 ms) | 91.8 % (7.0 ms) | 88.2 % |
| 8, 6 ms | 90.8 % (10.9 ms) | 88.2 % (10.5 ms) | 84.0 % |
| 12, 0 ms | 95.2 % (6.0 ms) | 90.8 % (5.2 ms) | 88.5 % |

(Half-widths in brackets.) Charging the time covariate a degree of freedom is necessary: without it
4 blocks covered 68 %.

**Real A/A** (`engine/tests/test_real_experiments.py`): inside one arm of experiments 12 and 15 to
19, half the blocks relabelled as the variant, 42 splits of four blocks. 40 are estimable (two of
experiment 15 have fuel and time between blocks, 0 degrees of freedom); 10 % of their 90 %
intervals exclude zero, median half-width 22.5 ms (1 or 2 degrees of freedom).

### 2. Time and sessions

Core sends `time_since_start_s` (from the experiment's first compared pass) and, for each game
session after the first, `session_<n>` = 1 on its passes. A session term is never left out by the
variance-inflation rule of ADR 0022; when it cannot be told from the arm (every variant block in a
session of its own) the effect is not estimable and the finding is invalid ("the game sessions
cannot be told apart from the setups"). Simulated restart before block 4 with every pass of the new
session 15 ms slower (200 experiments): with the term, mean estimate under 2 ms and coverage 86 to
95 %; with the clock restarting at 0 and no term, +3.7 ms of the step lands on the arm.

### 3. Margins

`engine/paddock_engine/config/margins.toml`: `[metrics]` defaults (corner time 5 ms, rear slip peak
0.5 deg, countersteer 0.05 s, the guardrails' tolerances; off-track share 0.05) and
`[parameters.<SETUP_ITEM>]` overrides. A channel's margin is the item's own, else what the
experiment froze (the success test's band, a guardrail's tolerance) when above 0, else the
metric's default, else none: a margin of 0 is refused when the file is read and never used as a
band. The evaluate request carries the setup item (`parameter`); the verdict stores the margin.

### 4. The eight findings

`engine/paddock_engine/experiments/finding.py`. [L, U] is the primary interval at the look's level
(section 5), turned so that a gain is positive; D the margin.

| Finding | Rule |
| --- | --- |
| invalid | not estimable; more than 5 % of push passes on another setup or from another driver; session terms tangled with the arm; no margin |
| confirmed improvement | L > D and at least 8 blocks |
| confirmed regression | U < -D and at least 8 blocks |
| context-dependent | not confirmed, and the arm acts differently between speed classes (below) |
| neutral | -D < L and U < D (two one-sided tests at the look's level) |
| likely improvement | L > 0 |
| likely regression | U < 0 |
| insufficient evidence | otherwise; flagged `uninformative` when (U - L) / 2 > 2 D |

Fewer than 8 blocks is flagged `few_blocks` and is never confirmed: an interval beyond the margin
is then likely. The finding is orthogonal to the ledger decision, which keeps its meaning for the
champion and for training:

| Finding | Decision |
| --- | --- |
| confirmed improvement | accepted; tradeoff when a guardrail moved or broke |
| confirmed regression, neutral, likely regression | rejected (the baseline stays) |
| likely improvement, context-dependent, insufficient evidence | inconclusive; rejected when a guardrail broke |
| invalid | inconclusive: nothing is decided on numbers that cannot be trusted |

**Context-dependent** (`stats/interaction.py`, `experiments/context.py`): per run block, the
corner-weighted mean residual of the primary fit in one speed class less that in another; block
shocks cancel in the difference. Compared between the arms with a two-sample t on the blocks
(blocks - 2 degrees of freedom), for every pair of classes with at least 3 corners each, Holm
across the pairs at 5 %. A corner's class is the one most of its baseline passes have.
Simulated (+10 ms slow, -10 ms fast, 0 medium, 8 blocks, 200 experiments): found in at least 80 %
on a car with half the FH26's pass scatter (93 % in the calibration run), at most 5 % without an
interaction (2 %). With the FH26's full scatter the same interaction is found only 37 % of the
time at 8 blocks and 53 % at 12: a limit, not a bug.

**Speed classes stored.** A class result with fewer than 3 corners, or fewer than 3 block degrees
of freedom, is not stored.

**Driver adapted** is Holm across the driver-input channels at 1 - level (10 %), each tested
against 0 with its block-level t; simulated on identical setups with three correlated channels,
5 % of 200 experiments flagged (the test allows 12 %; each at 10 % alone: about 27 %). **Response moved** (for `switch_metric`) is the same
over the guardrail and predicted channels.

**Off-track passes** are a guardrail: core counts each block's push passes and how many went off
track, the engine compares the shares variant minus baseline by block (`experiments/offtrack.py`)
against the experiment's own `offtrack_rate` guardrail or the configured margin (0.05). Off-track
passes are never compared, never silently dropped.

### 5. Pre-registered looks

Looks at 8, 10 and 12 blocks (`stats/sequential.py`). The Lan-DeMets O'Brien-Fleming spending
function alpha(t) = 2 - 2 Phi(z_{1 - alpha/2} / sqrt(t)), t = blocks / 12, gives for a total
one-sided 5 % the boundaries **z = 2.135, 1.926, 1.752**, computed by recursive integration on
Gauss-Legendre nodes and checked by Monte Carlo (10^6 correlated looks: 5.0 %). The audit's 2.138,
1.927, 1.754 are 0.001 to 0.003 high.

The verdict's statistics are block-level t on 5 to 9 degrees of freedom, refitted at every look,
not z. With the full 5 % spent, a change exactly at the band edge was accepted under the extension
rules more often than 5 %:

| Spent of the 5 % | One look | Training rule | Always extend | Nominal 90 % at every look (before) |
| --- | --- | --- | --- | --- |
| 100 % (seed 1, 2000) | 2.4 % | 5.9 % | 5.9 % | 8.8 % |
| 100 % (seed 3, 3000) | 2.1 % | 6.5 % | 6.5 % | 10.0 % |
| 75 % (seed 3, 3000) | 1.5 % | 5.1 % | 5.1 % | 10.0 % |
| **70 %** (seed 4, 3000) | 0.9 % | **3.8 %** | 3.8 % | 8.7 % |
| **70 %** (seed 5, 3 ms shocks, 3000) | 0.7 % | **3.8 %** | 3.8 % | 8.9 % |

An idealised process (normal block means, regression on arm and time) spends 5.5 % with the full
5 %; the rest of the excess comes from the refitted corner weights. The verdict spends 70 %
(`SPENT_SHARE`): z = 2.333, 2.096, 1.906, intervals of 98.0, 96.4 and 94.3 %. Equivalence spends
the same way (the same interval). The look is part of the evaluate request: core computes it from
the planned blocks (0 below 8, then 1, 2, 3), with the extensions left. A plan's `mde_decide` is at
the first look with blocks - 3 degrees of freedom.

**Every plan is 8 blocks** (`experiments/plan.py`), the first look: fewer can never confirm, and
the two extensions are the later looks. The planner still says what the first look decides and
whether the target is met. The Ferrari's 4-block plans become 8.

**Recommendation** at every look: `continue` (likely improvement or insufficient evidence whose
U >= D, with an extension left), `larger_perturbation` (no extension left, small but
directional), `investigate_context`, `switch_metric` (neutral or futile while a response channel
moved), `stop_neutral` (neutral, or futile: U < D rules out an improvement worth having), and
`stop_invalid`. None for a confirmed finding. Futility is non-binding: it only stops spending.

**Training** extends exactly when the engine recommends `continue` and an extension is left (up to
two: the looks at 10 and 12). A human may extend any inconclusive experiment; the look the blocks
make still sets the boundary.

### 6. Guards in core

`core/internal/experiments` (`comparison.go`, `looks.go`):

- `evaluate` refuses (409) unless every block of the plan has its laps, for a person as for a
  program (the same `AllDriven` as training);
- a push pass whose `setup_version_id` is not its block's arm, or whose session source is not the
  experiment's driver (human: `human`, `demo`; AI: `ai`, `ghost`), is left out and counted
  (`excluded`); above 5 % of the experiment's push passes the finding is invalid;
- off-track passes are counted per block (`offtrack`).

Stored on the verdict (migration 00012, [ADR 0030](0030-append-only-results-and-engine-version.md)):
finding class, recommendation, flags, confounders, driver adapted, engine version, passes used and
excluded, look, margin.

## Replay of the real experiments (offline; the ledger was not touched, past verdicts stand)

`engine/tests/fixtures/real_experiments.csv.gz` (exported read-only), judged as core now sends
them, at the look their blocks make and no extension left. Corner time, ms:

| Exp | Setting | Blocks | Ledger (engine of the day) | New: estimate [interval], df | New finding | Next |
| --- | --- | --- | --- | --- | --- | --- |
| 11 | rear ARB +1 | 4 | -4.67 [-17.90, +8.56] inconclusive | -4.67 [-22.03, +12.69], 1 | insufficient (few blocks, uninformative) | switch metric |
| 12 | front wing +1 | 8 | -3.09 [-9.89, +3.72] inconclusive | -3.09 [-13.37, +7.20], 5 | insufficient (uninformative) | larger step |
| 15 | rear wing -1 | 8 | -0.01 [-5.10, +5.08] inconclusive | -0.01 [-6.65, +6.63], 4 | context-dependent (fast vs medium) | investigate class |
| 16 | rear ARB +1 | 10 | -3.79 [-8.56, +0.97] inconclusive | -3.65 [-10.38, +3.07], 6 | insufficient | larger step |
| 17 | item 11 -1 | 8 | +1.27 [-3.10, +5.65] inconclusive | +1.27 [-7.36, +9.91], 5 | insufficient | larger step |
| 18 | front bias -1 | 8 | +4.51 [-0.87, +9.89] inconclusive | +4.51 [-6.11, +15.13], 5 | insufficient (uninformative) | larger step |
| 19 | rear ARB +1 | 8 | +1.29 [-6.11, +8.69] inconclusive | +1.29 [-11.51, +14.09], 5 | context-dependent (fast vs medium) | investigate class |

Every decision stays inconclusive; none would have been accepted or rejected. The estimates are
the ledger's to 0.01 ms where one session was driven (16: the session term and time since start
move it 0.14 ms). The half-widths grow by 30 to 97 %: the price of block-level degrees of freedom
and of the first look's 98 % interval. Experiment 15's rear slip peak: +0.158 deg
[+0.094, +0.221], clear of zero (the response moved; the time did not). Experiments 12 and 16 flag
the driver inputs (Holm). Experiment 15's fuel is block-varying (block 1 at half a tank, I9).

## Alternatives considered

- **max(bootstrap, block) standard error** (as asked): measured above, over-covers to 95-100 % and
  widens 8-block intervals by a fifth for nothing.
- **CR2 / wild cluster bootstrap by block.** Equivalent in aim; the block regression is exact
  under its model, explainable in one sentence, and covers nominally in every cell.
- **Spending the full 5 %.** Measured above: 5.9 to 6.5 %.
- **Keeping 4-block plans.** They can never be confirmed, and would spend both extensions reaching
  the first look.
- **An omnibus F test for the speed classes instead of Holm over pairs.** One test, slightly more
  power, but it does not say which classes differ, and the reader needs the pair.

## Consequences

- An experiment drives 8 blocks before its first verdict: about 40 minutes on a 1:15 lap, twice
  the Ferrari's old plan. Training may then extend twice.
- Intervals are wider and honest; a decision needs a larger effect. On the FH26's scatter a 20 ms
  effect is decided only at the later looks.
- No bootstrap: an evaluation fits the model twice per channel instead of about 400 times.
- Contracts: `EvaluateRequest` gains `parameter`, `look`, `extensions_left`, `excluded`,
  `offtrack`; `EvaluateResponse` and `VerdictSummary` gain the finding fields, all optional, so an
  old engine's answer still validates.

## Not verified

- **Unverified in game.** Nothing here ran in Assetto Corsa. The method is tested on simulations
  with known truth and on the real rows offline.
- Phase-level interactions (entry, mid, exit) wait for distance-frozen phases (ADR 0031); the
  speed classes used here are the per-pass ones core stores, frozen by the baseline's majority,
  until that change gives per-corner classes.
- The 70 % share is calibrated on simulations of 8 to 12 blocks with time adjusted for; other
  designs (more covariates between blocks) were not simulated.
