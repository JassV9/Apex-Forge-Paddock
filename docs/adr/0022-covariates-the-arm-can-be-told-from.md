# ADR 0022: The model adjusts only for covariates the setup change can be told from

Status: accepted (2026-11-17). Amends the covariate rule of [ADR 0008](0008-effect-model.md). Amended by [ADR 0029](0029-block-level-inference-and-finding-classes.md): session terms (`session_<n>`) are never left out; time runs from the experiment's first pass.

## Context

AI experiment 6 (differential coast, Ferrari 488 GT3, COTA) was judged inconclusive with a
corner-time interval of [-452, +450] ms over 203 passes, and a rear-slip interval of about
+-3.9 degrees. Experiments 3 to 5 on the same car had intervals of a few to a few tens of
milliseconds, and the passes of experiment 6 were clean: the 293 passes tagged to its blocks
were all valid. The only ones more than 0.1 s from their corner's median are six out-laps
(block lap 0, at corner 303), which core already leaves out; among the 208 push passes sent to
the engine the largest deviation is 0.07 s (corner 311) and 93 % of all deviations are under
0.03 s. There was no outlier to reject. Replaying the same rows offline showed the cause is in the design matrix:

- The game was restarted twice, so the four blocks were driven over three sessions (70, 72, 74).
  A new session resets the clock (`time_in_session_s`) and refuels, so the variant blocks sat
  early in a session on a heavy tank (fuel 38-47 l) and the baseline blocks late on a light one
  (18-39 l). Within corners, the arm correlated -0.87 with time and -0.81 with fuel.
- Fuel and time burn off together inside one session, so the old rule (drop a covariate that
  correlates above 0.98 with another) removed one of them in experiments 3 to 5. Across three
  sessions their correlation is 0.94: below the threshold, both stayed, and the variance
  inflation factor was 94 for time, 153 for fuel and 42 for the arm.
- The bootstrap resamples whole laps, two of three per block. In 183 of 400 resamples the
  arm, fuel and time columns were close enough to dependent that the fitted arm coefficient
  swung by more than 0.1 s (range -1.6 to +0.02 s), so the bootstrap standard error was 0.23 s
  although the fit on all passes put the arm at 0.0000 s. The interval was the model refusing to
  tell the arm from the drift, not noise in the laps.

## Decision

**A covariate enters only while no column of the design is nearly a combination of the
others.** After the covariates that every pass has and that vary within corners are chosen,
the engine computes the variance inflation factor, 1 / (1 - R^2) of each column regressed on
the other columns, over the arm indicators and the covariates, all centred within corners as
the model uses them. While the largest is above 10 (the textbook limit), the covariate with the
largest factor is left out (among equals, the alphabetically last, so the first name stays); the
arm is never left out. This replaces the pairwise 0.98 rule, which it contains: two columns
correlated at 0.98 have a factor of at least 25.

**It is never silent.** The effect row's covariates are the ones used; the ones left out are on
`Effect.dropped_covariates`, and the verdict's reason says "Not adjusted for fuel_l: too
entangled with which setup was on the car (see ADR 0022)."

**No outlier rule.** The first reading of experiment 6 was a few corrupt passes. The data does
not support it, and a rule written for a problem the ledger does not have would be a number
nobody can explain. If extreme passes appear in a real experiment, rejecting them with a robust
scale (MAD, as `stats/noise.py` already uses) and reporting the count in the reason is the
option; it is not built.

**Replay of experiment 6 on its real rows** (offline; the ledger was not touched): the engine
leaves fuel out, keeps time and tyre temperature, and gives corner time +4.6 ms [+0.2, +9.1]
(inconclusive: not wholly outside the 5 ms band, not inside it), rear slip peak +0.05
degrees [+0.00, +0.09] (guardrail holds). With fuel kept instead of time the estimate is
-2.3 ms [-6.0, +1.4]. The sign depends on which of the two confounded drifts is adjusted for,
which is the honest summary of what a restarted A-B-A-B can say.

## Related fix in core

Passes of one lap share their errors, and the engine clusters them by lap number. Core coded
a lap as `block * 1000 + block_lap`, so a block continued in a new session had two lap 1s
merged into one cluster (block 4 of experiment 6: 15 passes from session 72 and 17 from session
74). Laps are now numbered per block in the order they were driven and told apart by session as
well as by lap (`core/internal/experiments/verdict.go`).

## Alternatives considered

- **Reject passes more than k MAD from their corner's median.** Rejected, see above: there were
  none to reject, and it would not have changed the interval.
- **Keep both covariates and widen the interval.** It was already +-450 ms; an honest answer, but
  one that wastes a clean experiment for want of a rule about two nuisance variables.
- **Drop time and fuel always, adjust for tyre temperature only.** Simpler, but it throws away
  the adjustment that matters when a session is long and the tank is light by the end.
- **Test conditioning inside the bootstrap (skip bad resamples).** Skipping resamples changes the
  distribution being estimated and hides the cause; screening the design once is explainable.

## Consequences

- The rule is deterministic and unit-free (the factor does not depend on a covariate's units);
  a hypothesis test checks the kept columns always sit under the limit and that rescaling a
  covariate never changes the choice.
- Experiments that were judged before this change are not re-judged. Experiment 6 stays
  "inconclusive, +-450 ms" in the ledger, with this ADR as the explanation.
- A covariate that really drives the result and is confounded with the arm is no longer
  adjusted for. The reason says so, and the driver can see the confound.

## Not fixed here

- **Fuel is not held equal across blocks.** The cleaner remedy is in the run plan: the agent
  refuels to the same level at the start of every block (or at least every session), so fuel does
  not track the arm. Needs a change in the agent and a run in the game; not done.
- **A lap counts as driven when its first pass arrives** (`laps_done` is the highest
  `block_lap`), so core judged experiment 6 about four corners into the last lap (203 of 208
  passes). That lap is incomplete for the design, though not what widened the interval. Fixed
  in [ADR 0025](0025-finished-laps-and-lap-noise.md): a lap counts once it is finished.
- Everything above is unverified in the game; the replay uses rows the game produced.
