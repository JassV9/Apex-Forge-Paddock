# ADR 0025: A lap counts once it is finished, and plans count the noise of whole laps

Status: accepted (2026-11-17). Amends the plan of [ADR 0008](0008-effect-model.md) and the
"every block driven" trigger of [ADR 0020](0020-ai-training-program.md). (0024 is taken by a
change written in parallel.)

## Context

Two problems showed on the real rows of the AI experiments (read-only queries; the ledger was
not touched).

**The last lap was judged before it was driven.** Core counted a block's laps as the highest
`block_lap` among its passes, so a lap counted as soon as its first pass arrived. The training
program evaluates when every block has its laps, so it judged as soon as the final lap began. On
experiments 7 to 10 (Formula Hybrid 2026, Hungaroring, 14 corners) block 4's lap 3 had 4 passes
in the verdict, on experiment 5 (Ferrari 488 GT3, COTA) one, on experiment 6 six of 17. The
agent tags a pass with the lap it ends on and counts a lap at the line, after the middle of the
lap (`agent/src/core/runplan.lua`). The last corner before the line therefore ends after it: a
push lap's passes are 17, 1, 2, ..., 16 at COTA and 14, 1, ..., 13 at the Hungaroring, and a
finished lap reaches every corner of the map. After a restart the agent resumes at its saved
lap count and drives the interrupted lap again under the same `block_lap`
(experiment 6, block 4, lap 1 in sessions 72 and 74). After a retry (car stuck) it drives an
out-lap and the same lap again under a new game lap.

**Plans ignored lap noise.** The planner sized a plan from the pass-to-pass scatter alone
(`mde = 2.8 sigma sqrt(2 / (n k))`). Some laps are quicker or slower at every corner at once.
That error is shared by all corners of the lap, so it averages out over laps only. Removing
each lap's weighted mean residual from the real passes narrows the corner-time intervals as
follows (offline replay on every push pass now in the ledger, so the intervals differ slightly
from ADR 0023's, which were cut to the passes each verdict saw):

| Exp | Interval (ADR 0023 engine) | Laps' shared error removed | SD of the laps' mean residuals |
| --- | --- | --- | --- |
| 5 (Ferrari) | +-3.4 ms | +-1.8 ms | 1.6 ms |
| 6 (Ferrari) | +-2.3 ms | +-2.1 ms | 1.7 ms |
| 7 (FH26) | +-18.7 ms | +-6.9 ms | 15.3 ms |
| 8 (FH26) | +-19.5 ms | +-6.8 ms | 15.4 ms |
| 9 (FH26) | +-16.2 ms | +-6.0 ms | 7.3 ms |
| 10 (FH26) | +-26.1 ms | +-12.8 ms | 8.7 ms |

The FH26 plans promised to decide 20 to 23 ms, an implied interval of about +-11 ms, and gave
+-16 to +-32 ms. The Ferrari has almost no lap noise.

## Decision: when a lap counts (core)

`core/internal/experiments/laps.go`. A block's `laps_done` is the number of push laps the car
has **finished**. Every lap below the block's highest is finished: the agent tags lap L + 1 only
after it counted lap L at the line. The highest lap counts once any one of the following holds:

1. **Complete.** One attempt of it reached every corner of the track's map. An attempt is the
   passes of that block lap in one session and one game lap (`lap_in_session`), so an interrupted
   lap and its re-drive are not added together.
2. **Moved on.** A later block of the experiment has passes. The agent leaves a block only
   after its last line, and this covers a final lap that lost a pass.
3. **Quiet.** No pass of the experiment has ended for 15 minutes (`quietAfter`). This covers a
   car stuck three times, a closed game, or a last lap whose pass was dropped at a corner. The
   period is longer than the agent's lap watchdog (`LAP_TIMEOUT_S`, 10 minutes). After that
   watchdog the AI retries the block, and the retry's out-lap passes arrive within a couple of
   minutes, so a live run is never taken for a stopped one. The database clock is compared with
   the passes' end (`t_start + duration_s`).

The out-lap (block lap 0) still never counts. The training program's `driven` is unchanged: it
reads `laps_done`, so it now waits for the last lap's corners, normally about a minute more.

| Case | Before | Now |
| --- | --- | --- |
| Last lap 4 of 14 corners in, car driving (experiments 7 to 10) | 3 of 3, judged | 2 of 3, waits |
| Last lap complete | 3 | 3 |
| Car stuck on the last lap, the AI retries it | 3, judged on the stuck attempt | 2 until the retry finishes the lap |
| Stuck three times, or game closed, nothing more arrives | 3, judged | 2, then 3 after 15 minutes: judged on what was driven |
| Restart in the middle of lap 1 (experiment 6, block 4) | 1 during the new session's out-lap | 0, then 1 once the re-driven lap 1 is complete |
| A pass of the last lap lost, the next block started | 3 | 3 |

A stuck run is no longer a deadlock in this respect. The program judges it on the laps there
are, as it did before, only later. A run stuck before its last block still waits for the
driver, as ADR 0020 says.

## Decision: plans sized for lap noise (engine)

**Noise model.** A corner pass is `level[corner] + lap error + pass error`. With one pass per
corner and lap, an arm's mean over L laps of k corners has variance
`(sigma_pass^2 / k + sigma_lap^2) / L`. That is the pass formula with an equivalent scatter
`sigma_eff^2 = sigma_pass^2 + k sigma_lap^2` (`stats/mde.py`, `lap_inclusive_sigma`). The
measured `sigma` (median over corners of the MAD scatter, `stats/noise.py`) is taken around
each corner's level over all laps, so it already holds `sigma_lap^2` once, and
`sigma_pass^2 = sigma^2 - sigma_lap^2`.

**Lap noise estimate** (`stats/lap_noise.py`). This is the between-lap variance component of
the random-intercept model, with the lap as the group: the Swamy-Arora moment estimate of
`stats/model.py`. Each lap that reached at least half the corners gets a mean residual from its
corners' medians. The corners are weighted as the verdict weighs them (ADR 0023), so a wild
corner's pass noise is not taken for lap noise. The lap variance is the robust spread of those
means (1.4826 x MAD) squared, minus what the passes' own scatter puts into a mean. A lap
counts only once its passes come with a lap code. Core sends one per session, game lap and
block; an out-lap gets none (`plan_request.schema.json`: `lap`, `planned`).

**Which laps.** Push laps of earlier run plans on the baseline come first. Free and calibration
laps are used only when there are fewer than six of those. During calibration the agent changes
the AI's grip lap by lap, which a frozen run plan does not do. On the Ferrari's baseline all laps
together (calibration included) give 29 ms of lap noise, against 2.6 ms from its 18 planned laps
and under 1 ms inside experiment 5. Robust and not a standard deviation: the Ferrari's planned laps include
ruined laps from experiment 3 (SD 73 ms against MAD 2.5 ms), and the pass scatter beside it is
robust too. With neither (fewer than six laps that reach half the corners) the lap noise is
assumed 0 and the plan says "not measured".

**Sizing** (`experiments/plan.py`). Blocks are added in A-B pairs of the requested length
(3 laps) from four up to eight, and the plan takes the fewest whose decision effect reaches the
target:

```
decide(B) = rope + (t[3B - B - 1] + 0.84) x infl(B) x sigma_eff x sqrt(2 / (3B/2 x k))
infl(B)   = sqrt((B^2 - 1) / (B^2 - 4))     time-in-session adjustment: 1.118 at B = 4 (was 1.12)
target    = rope x max(4, ceil(decide_passes_only(4) / rope))
```

- **The target is a whole number of bands.** Four bands, which is the brief's 20 ms of corner
  time against its 5 ms band. For a car whose passes alone already need more, it is the fewest
  whole bands that cover what four blocks decide on pass scatter alone. Without lap noise every
  plan is the standard one. Lap noise worth less than the rest of a band adds no blocks: the
  first version used the standard figure itself as the target, and then 1.7 ms of lap noise
  sent experiment 9 to six blocks.
- **The cap is 8 blocks.** That is the longest run the ledger already lets an experiment drive:
  four blocks and two extensions of two blocks each (`MaxExtensions`). Blocks keep their 3 laps
  because that length has been driven in the game. Longer blocks need more fuel, and the agent
  refuels every block to the first block's level.
- **Never silent.** The plan returns `sigma_lap`, `sigma_lap_source` (`planned`, `free`,
  `none`), `mde_target` and `target_met`. Core stores them in the plan summary and writes them
  into the plan's ledger line, for example: "8 blocks of 3 laps; decides effects of 0.0328 or
  more on corner_time_s (scatter measured; whole laps scatter 0.0243, from free laps). Short of
  the 0.025 it was sized for: 8 blocks is the longest plan." The dashboard shows the same.

**Checked by simulation.** 150 simulated experiments each, 14 corners, 28 ms pass noise and
15 ms lap noise, time in session adjusted for:

| Blocks | Scatter of the estimates | Planned standard error | Mean interval half-width | Planned half-width | Coverage (90 %) |
| --- | --- | --- | --- | --- | --- |
| 4 | 9.0 ms | 10.8 ms | 20.1 ms | 20.5 ms | 95 % |
| 8 | 6.6 ms | 7.0 ms | 11.9 ms | 12.3 ms | 91 % |

The pass-only formula would have planned 4.8 ms and 3.1 ms. The planned half-widths match the
verdict's to within 4 %.

## Replay: what the planner would have planned (offline)

The baselines were rebuilt as core's `planRequest` sends them: valid passes on the baseline
setup recorded before the experiment was planned, with lap codes. The engine at this commit was
run on them. Corner time, ms. "+-" is the interval half-width the plan predicts with the
measured lap noise. "Actual" is ADR 0023's replayed interval on the passes each verdict saw (four
blocks).

| Exp | Car | sigma | Lap noise (source) | Old plan: decides, implied +- | Old plan with lap noise: +- | New plan | New: decides, +- | Target, met | Actual +- |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 5 | Ferrari 488 GT3 | 24.9 | 2.6 (planned) | 15.7, 7.4 | 8.0 | 4 x 3 | 16.5, 8.0 | 20, yes | 3.4 |
| 7 | FH26 | 38.5 | 24.3 (free) | 23.2, 12.6 | 31.3 | 8 x 3 | 32.8, 18.8 | 25, **no** | 19.6 |
| 8 | FH26 | 31.8 | 17.2 (planned) | 20.0, 10.4 | 22.8 | 8 x 3 | 25.2, 13.7 | 25, **no** | 19.1 |
| 9 | FH26 | 33.0 | 1.7 (planned) | 20.6, 10.8 | 11.0 | 4 x 3 | 20.8, 11.0 | 25, yes | 20.2 |
| 10 | FH26 | 33.6 | 9.2 (planned) | 20.8, 10.9 | 15.5 | 6 x 3 | 21.4, 11.2 | 25, yes | 32.0 |

The Ferrari plans as before. Its sigma of 24.9 ms comes from old calibration laps, and inside
its experiments a pass scatters about 3 ms. That is why its actual intervals are narrower than
planned; the same was true before this change. On the FH26 the plans grow to 6 or 8 blocks, and
two say they cannot reach 25 ms in 8. Across the four FH26 experiments the old 4-block plans
with lap noise predict +-20 ms on average, close to the +-23 ms they gave, where the old
planner implied +-11 ms. The lap noise estimate itself is noisy with 6 to 18 laps:
experiment 9's 1.7 ms is too low for the +-20 ms it gave.

## Consequences

- Contracts: `PlanPass` gains `lap` and `planned`. `PlanResponse` gains `sigma_lap`,
  `sigma_lap_source`, `mde_target` and `target_met`. `PlanSummary` gains the same four,
  optional, so plans made before stay readable. `ExperimentBlock.laps_done` keeps its type with a
  sharper meaning.
- A noisy car drives up to twice as long: 8 blocks of 3 laps plus out-laps, about 40 minutes on
  a 1:15 Hungaroring lap. An 8-block plan can still be extended. Extensions are counted, not
  capped by length, so a human can take it to 12 blocks; a training program extends once, to 10.
- Past plans and verdicts are not changed. The ledger keeps "decides 0.0232" for experiment 7.
- The ranking's `laps_needed` (`optimize/rank.py`) still uses the pass formula; it ranks by
  value per lap and is not a plan.

## Alternatives considered

- **Longer blocks instead of more blocks.** Fewer out-laps, but more fuel per block. Fuel is not
  held equal across blocks (ADR 0022), and blocks of more than 3 laps were never driven in the
  game.
- **A fixed target of four bands for every car.** Then an FH26 with no lap noise at all would get
  six blocks, and plans would change for a reason other than lap noise.
- **The standard plan's own figure as the target.** Any lap noise at all then adds blocks; see
  above.
- **The standard deviation of the lap means, as the verdict carries them.** The Ferrari's
  baseline then shows 73 ms from ruined laps, and every plan would be the longest.
- **Counting a lap at its last corner (the highest corner number).** That depends on the corner
  map's numbering; reaching every corner does not.
- **Waiting indefinitely for the last lap.** A stuck or closed game would then hold a program
  forever on the last lap.

## Not verified

- Unverified in game. Nothing ran in Assetto Corsa. The lap rule is tested against PostgreSQL
  with passes shaped like the real ones, and the planner on simulations and on the real rows
  offline.
- A retry inside one session is told apart by the game lap for counting. The verdict's lap codes
  (ADR 0022) still use session and block lap, so a retried lap's two attempts share one
  bootstrap cluster.
- The lap noise from 6 to 18 laps is uncertain by tens of percent, and calibration laps make it
  too high for a car's first experiment (experiment 7: 24 ms from free laps).
