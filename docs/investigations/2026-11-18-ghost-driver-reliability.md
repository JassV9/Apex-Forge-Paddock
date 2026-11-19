# Ghost Driver reliability and vehicle sensitivity: audit, findings and experiment design

Date: 2026-11-18. Status: Phase 1 (audit) and Phase 2 (design) complete; implementation tracked in
the ADRs this document names. I1, I11, I12 and I13 are implemented in [ADR 0033](../adr/0033-stratified-vehicle-sensitivity-profile.md). C2, C3, C5, C6, C7, I2, I3, I4 (Holm, classes) and I5 are implemented
in [ADR 0029](../adr/0029-block-level-inference-and-finding-classes.md) and
[ADR 0030](../adr/0030-append-only-results-and-engine-version.md); the boundaries of section 7
were recomputed (z = 2.135, 1.926, 1.752) and the verdict spends 70 % of the 5 %. Four independent read-only audits (statistics, AI behaviour,
telemetry, lifecycle) were run against the code, the live database (SELECT only) and the spool; the
numbers below are theirs, reproduced by replaying the engine on the real rows. Nothing here was run in
Assetto Corsa unless it says so.

## 1. What the system is actually measuring

The AI's **speed is set by its plan**, not by the car's grip. CSP's `physics.setAITyresHint` changes
the `[TYRES] HINT` the AI plans with; `physics.setExtraAIGrip(0, 1)` sets the real physics grip to
normal (default 120 %). They are separate and the code keeps them separate (`agent/src/game/ghost.lua`
keepOut, `calibration.lua`). The calibrated share is per car and layout: **0.885 for the 488 GT3 at COTA,
0.9409 for the Formula Hybrid 2026 at the Hungaroring**. Docs that say "the AI drives at 88.5 % of its
grip" are wrong twice (wrong car in general, and it is a planning share on real grip) and are corrected.

Consequence, seen in the data (experiment 15, rear wing -1 on both linked items, 8 blocks): corner time
0.0 [-5.1, +5.1] ms and minimum speed +0.23 km/h, while entry yaw gain rose in 8 of 8 corners where it
moved significantly (~+6 %), mid-corner rear slip rose in 6 of 6 (+0.11 deg), peak lateral g fell in 6 of 6
(-0.02 g) and steering per g fell in 6 of 7 (~-3 %). **The car's response changed; its time did not.**
Rear ARB +1 and front wing +1 moved almost nothing (speeds within 0.3 km/h, braking within 1.4 m).
Whether the AI masks a real effect or the effect is ~0 cannot be told apart without a limit driver.

So: corner time with a plan-limited AI is a weak observable for balance changes. Vehicle-response
channels are the primary evidence of what a setup change does; time is one outcome among several.

## 2. Findings, prioritised

### Critical

| # | Finding | Evidence | Fix (ADR) |
|---|---|---|---|
| C1 | AI calibration is not recorded with any session, pass or experiment; the ai_tune file is overwritten. The four pooled rear ARB +1 runs were driven at unknown hints. | `spool_session` has no hint; `ai_tune/*.json` single file; sessions 80-88 run 2.6-3.5 % higher peak g than 119 | Run context per session/pass and a hint pinned per experiment (0028) |
| C2 | Inference cannot see block-level error: the bootstrap resamples laps inside blocks; df = laps - blocks - 1 (15 at 8x3) while the setup is identified from ~B-2 block contrasts | `effects.py:66-69,108`; A/A simulation with 6 ms block shocks: 77 % coverage at 8 blocks, 9 % false decisions at 4 | Block-level inference as the binding SE; >= 8 blocks for automatic decisions (0029) |
| C3 | Extending after a peek inflates false acceptance (training extends promising inconclusives; humans twice) | Simulation at the band edge: 6.5 % one look, 8.5-10.2 % training rule, up to 12 % human | Pre-registered looks with O'Brien-Fleming alpha spending (0029) |
| C4 | Speed class is per pass, from that pass's own minimum speed: an outcome of the setup. A corner changes class between arms | `pass_features.py:36-40,91`; 56 (experiment, corner) pairs have passes in more than one class (exp 17 T6: 38 % slow vs 0 %) | Class per corner, frozen from the baseline, track/car-specific breaks (0031) |
| C5 | Off-track passes are dropped silently; a setup that makes the AI run wide loses its worst passes | `pass_features.py:88-93`, `verdict.go:49` | Count them per arm as a guardrail (`offtrack_rate`); never silently (0029) |
| C6 | A manual `evaluate` can judge an unfinished plan; only training checks every block is driven | `decide.go:190` vs `training/advance.go:108-118` | Refuse unless every block is driven (0029) |
| C7 | Core never checks a pass ran on the arm's setup or that its source matches the experiment's driver; the agent's tag is trusted, and its check skips items missing from the read-back | `ingest/postgres_pass.go:15-24`, `runplan.lua:95` | Exclude and count mismatching passes; an experiment with any is invalid above a threshold (0029) |

### Important

| # | Finding | Evidence | Fix |
|---|---|---|---|
| I1 | Pooling across experiments is fixed-effect with z quantiles (no tau^2, no small-k correction); the t interval is inverted with z | `pooling.py:31-41`; rear ARB +1: -2.64 [-6.46, +1.17] fixed vs [-8.10, +2.81] HKSJ (k=4) | Random effects (Paule-Mandel/REML) + HKSJ with t(k-1), prediction interval (0033) |
| I2 | Results are overwritten on every evaluation and carry no engine version; verdicts judged by old engines (pre-0022/0023/0025, experiment 7 on a half-driven lap) are pooled with new ones | `verdict.go:111-114` DELETE+INSERT; exp 7 stored -15.75 [-53.2, +21.7] vs -1.03 [-19.7, +17.7] now | Append-only results with engine_version; pooling re-derives with the current engine (0030) |
| I3 | "Rejected" means both "worse" and "negligible"; inconclusive lumps "clearly faster, size unclear" with "no information"; one 5 ms band for every item and class; other channels have band 0 | `evaluate.py:67-96`, `rope.py:18-28`, `sensitivity/service.go:121-123` | Eight-way finding, orthogonal to ledger states; margins per parameter and metric in config (0029) |
| I4 | Multiple comparisons: ~10 intervals per experiment; "driver adapted" tests 3 channels at band 0 (~27 % false-flag rate under the null); degenerate speed classes (1 corner) stored and pooled | fired in verdicts 15, 16, 19; exp 5 fast class +-330 s | Holm across driver channels; no class with < 3 corners or block df < 3 (0029, 0031) |
| I5 | Time covariate restarts at 0 in a new session (exp 16's extension blocks look "early") | `verdict.go:97`; dropping it moves -3.79 to -2.82 ms | Time since experiment start plus a session term, or refuse to span sessions (0029) |
| I6 | Phase metrics (`brake_dist_m`, `throttle_pickup_m`) are measured from the per-pass slowest point, which jitters 20-46 m; the braking point itself moves < 4 m | `features/inputs.py:168,173`; T11 spread 46.6 m vs 0.9 m absolute | Events in absolute track metres; phases cut at frozen distances (0031) |
| I7 | Exit phase empty in 76-100 % of passes at several corners; `exit_speed_100m` null in 12 of 14 Hungaroring corners; chicane zones cut exits | `phases/segment.py:143,175-176`; `corners.lua:36-38` | Distance-frozen phases; join consecutive passes of a lap (0031) |
| I8 | Electric deployment is the largest AI noise source on the FH26 (T7 bimodal 2.41-2.57 vs 2.70-2.81 s; T7 vs start charge r=+0.69, T14 vs deployment r=-0.84). Only start charge is a covariate | audit_ai `t7.py` | Record and test deployment control; deployment as covariate only if it is not arm-dependent (0028) |
| I9 | Fuel not held: exp 15 block 1 at 24 L vs 48 L in blocks 2-8 (`set_fuel` did not take) | run_block passes | Read fuel back after set_fuel; stop if off by > 0.5 L (0028) |
| I10 | AI level, aggression and CSP's dynamic shifting are never pinned or recorded | `setAILevel`, `setAIAggression`, `setAIShiftingThresholds` never called | Record; pin where safe (0028) |
| I11 | Priors from another car get their own effect_class ("negligible" from a Ferrari lender); a consumer reading class without `is_prior` would treat it as measured | `sensitivity_effect` rows car 1/9/14 | Priors in their own table; never a class (0033) |
| I12 | The profile key ignores track, driver, hint, the baseline's absolute value and engine version; `sensitivity_effect` is rebuilt and deleted on every read | `cars.go:137-148`, `sensitivity/store.go:27-30` | Stratified, provenance-carrying profile (0033) |
| I13 | Base setup qualifies on fixed-effect one-sided intervals with no multiplicity control; prediction suffers winner's curse | `base_setup.py:50-139` | Qualify on HKSJ rows, BH-FDR across items, shrink the prediction (0033) |

### Minor

Dead `run_block.state`/`condition_window` and `corners.speedClass`; `driver_adapted` computed but not
persisted; `drivetrain` NULL for all cars (CSP `tractionType` available); sampling at frame rate
(~55 Hz, jitter 15-22 ms) with `car.timestamp` unused; `cleanLaps` counts experiment laps; retried laps
share a lap cluster; planner shares the optimistic df.

### Corrections to earlier statements in this project

- "The battery is not the problem" (session of 2026-11-17) was checked at lap level only, where charge at
  lap start is nearly constant. At pass level it is the largest noise source (I8). Corner weighting
  (ADR 0023) already reduces T7/T14's weight; deployment remains uncontrolled.
- "88.5 % grip" is the GT3 at COTA's planning share, not a property of the system.

### What was already right (keep)

GLS random-intercept fit (matches MixedLM); corner weights with floor and cap; VIF covariate guard;
lap noise in the planner; the ROPE/TOST idea (the 90 % interval inside the band is exactly TOST at 5 %);
ABAB blocks; experiment-level (not pass-level) pooling; judging a base setup as its own experiment;
excluding out-laps; the append-only ledger and verdicts; tyres reset per block (wear and core temp
identical at lap 1); the CSP channels already recorded per sample: speed, inputs, lat/long g, yaw rate,
slip angle and slip ratio per wheel (direct from CSP), tyre load, ride height front/rear, wheels outside.
A real A/A test on today's data (42 relabelled splits) gave 5 % interval exclusions and 0 decisions: the
engine is not shown to be anti-conservative now; it is fragile (C2), not demonstrably wrong.

## 3. Channel inventory (what exists, what to add)

Recorded per sample: speed, spline distance, throttle, brake, steering (wheel angle, deg), lat/long g,
yaw rate, slip angle x4, slip ratio x4, tyre load x4, ride height F/R, wheels outside. Per pass: tyre
core/middle temperature, pressure, wear, fuel, grip, air/road temperature, ERS charge start/end and mean
deployment. To add (all exposed by CSP, `lua-sdk/ac_apps/lib.lua`): normalised slip `ndSlip` x4 (direct
"past peak grip"), `localVelocity` (true body slip angle), wheel `angularSpeed` x4, `suspensionTravel` x4,
ABS/TC active, gear, rpm, `car.timestamp`, world position (line differences that spline distance hides),
per-pass AI settings. Estimates (documented as such): body slip from yaw/lat g/speed before
`localVelocity` exists; aero load share from a per-car fit of peak lat g against v^2.

## 4. Parameter to observable mapping (initial, data-driven later)

| Parameter | Primary observables (direct) | Proxies | Phases | Speed regime |
|---|---|---|---|---|
| Front ARB | steer per g, front slip, entry yaw gain | front saturation | entry, mid | all |
| Rear ARB | rear slip, yaw gain, traction slip ratio | countersteer | mid, exit | slow/medium |
| Front wing | steer per g, front slip | aero balance (slip F-R vs v^2) | mid | fast |
| Rear wing | rear slip, yaw gain, top speed | aero balance | mid, exit | fast |
| Diff preload | entry yaw gain, inner/outer wheel speed difference, throttle pickup | rear slip ratio split | entry, exit | slow |
| Brake bias | front/rear slip ratio under braking, lock events, entry yaw | brake distance (absolute) | braking, entry | all |
| Brake power | peak decel, slip ratio peaks, brake distance | lock events | braking | all |
| Springs / ride height | ride height, suspension travel, platform variance | aero share | mid | fast |
| Dampers | suspension velocity, transient yaw gain | ride height variance | entry, exit | all |
| Tyre pressure | tyre temps (core, inner/outer), slip at peak lat g | grip | mid | all |

The mapping is configuration, not code; each row lists direct measurements and proxies separately.
Informativeness per parameter is learned from discovery sweeps (effect size / uncertainty per channel).

## 5. Experiment classification (replaces the broad "inconclusive")

Finding classes, orthogonal to the existing ledger states (old rows derive one through a view):
invalid, confirmed improvement, confirmed regression, neutral (TOST), context-dependent, likely
improvement, likely regression, insufficient evidence (with an "uninformative" flag when the half-width
exceeds 2x the margin). Rules, margins and the mapping to ledger decisions are in ADR 0029. Only a
confirmed improvement with guardrails holding is promoted automatically. A finding on time and a finding
on vehicle response are separate: "neutral on time, rear slip +0.11 deg (confirmed)" is a valid and
useful result.

## 6. Controlled experiments (Phase 2 design)

All use the AI on the `_paddock` copy, one car and track per experiment, ABAB blocks of 3 push laps,
fuel set and read back each block, tyres fresh each block, AI hint **pinned and recorded**, extra grip 1,
no traffic. Statistical method for all: block-level inference (ADR 0029) on the primary metric, Holm
across secondary channels, TOST for neutrality with the stated margin, 90 % intervals. Invalidating
conditions for all: setup not on the car, fuel off > 0.5 L, hint differs from the pinned value, a session
restart without a session term, > 20 % of laps incomplete, incidents in more than one block.

| | Hypothesis | Independent | Controlled | Telemetry | Samples | Interpretation |
|---|---|---|---|---|---|---|
| A Repeatability | Same setup in A and B gives a finding of neutral/insufficient, never improvement/regression, and block-level intervals cover 0 at 90 % | none (A/A); session count | everything | all current | 3 sessions x 8 blocks | false-decision rate; session variance vs block variance; old vs new method on identical data |
| B AI compensation | A large balance change (rear ARB +3 or front ARB -3) changes response channels by more than their margin while minimum speed changes < 0.3 km/h and braking point < 1 m (plan-limited AI) | the setup change | hint pinned (calibrated), fuel 50 L | steer per g, yaw gain, slip F/R, ndSlip, lat g, min speed, braking point | 8 blocks | if response moves and speed does not, time is not a sufficient observable for this car |
| C Grip hint | Higher planning hints raise corner speeds and expose larger time effects of the same setup change; incidents rise | the hint (0.885, 0.93, 0.9409, 0.97, 1.0), each its own A/B | the setup change (rear wing -1, both items), car, track, fuel | as B plus incidents, invalid-pass rate | 8 blocks per hint; order 0.93, 0.885, 1.0, 0.97, 0.9409 | effect vs hint; usable hint = highest with incidents in at most one block. Never mix hints inside one A/B |
| D Rear ARB curve | Rear ARB response is monotonic in clicks and phase-specific (mid/exit, slow corners) | rear ARB -1, +1, +2 (+3 if in range), each vs baseline, baseline between | hint pinned | rear slip, yaw gain, traction slip, countersteer, phase times | 8 blocks each | linear / nonlinear / negligible / direction change, per phase and speed regime |
| E Aero | Front and rear wing effects concentrate in fast corners | front wing +-1, rear wing +-1 (both items) | hint pinned | slip F-R vs v^2, steer per g, top speed | 8 blocks each | per speed regime; context-dependent expected |
| F Diff and brakes | Diff preload acts on entry and exit; brake bias on braking and entry | preload -1/+1, bias -1/+1 | hint pinned | entry yaw gain, wheel speed difference, slip ratio under braking, lock events, absolute braking point | 8 blocks each | phase-specific findings, pooled time secondary |

Answerable offline (synthetic data with known truth): coverage under block shocks, alpha spending,
TOST operating characteristics, context-dependent detection, random-effects pooling coverage, base-setup
FDR, phase and speed-class stability, distance alignment. Answerable only in game: A-F themselves, what
the AI does at higher hints, whether deployment can be pinned, the block-1 fuel gap.

## 7. Sequential policy

Pre-registered looks at 8, 10 and 12 blocks with Lan-DeMets O'Brien-Fleming spending (one-sided efficacy
boundaries z = 2.138, 1.927, 1.754 for total alpha 0.05; equivalence spends the same way; futility at any
look). After a look the system recommends one of: continue (next look), test a larger perturbation (the
effect is small but directional), investigate a corner class or phase (context-dependent), switch the
primary metric (time neutral, response moved), stop (neutral), stop (invalid or unsafe).

## 8. Implementation plan

| Work package | ADR | Area |
|---|---|---|
| Run context, AI provenance, pinned hint, fuel read-back, new CSP channels | 0028 | agent, spool contract, ingest, migration |
| Block-level inference, finding classes, margins config, guards (manual evaluate, setup/source per pass, off-track guardrail), Holm, session term, alpha spending | 0029 | engine stats/experiments, core experiments, evaluate contract, migration |
| Append-only results, engine version | 0030 | core experiments, migration |
| Distance-frozen phases, absolute events, per-corner speed regimes, response analysis and the parameter-observable map | 0031 | engine features, new engine `response` package |
| Sensitivity discovery sweeps | 0032 | new core `discovery` package beside `training` |
| Stratified persistent profile, priors separate, random-effects pooling, base-setup FDR | 0033 | engine sensitivity/optimize, core sensitivity, migration |

## 9. In-game results

### Experiment B: AI compensation (experiment 20, 2026-11-19)

Formula Hybrid 2026, Hungaroring `_paddock`, rear ARB 2 -> 5 (+3 clicks), hint pinned at 0.9409 and
applied (read back per pass), 8 ABAB blocks of 3 push laps, session 129, 336 passes, none excluded.
Evaluated with engine 0.2.0 (block-level inference, look 1 of 3); the response table is an offline
replay of the same passes through the `POST /response` service (ADR 0031), which now uses the
block-level interval.

**Verdict (pre-registered test, corner time):** -7.3 ms [-13.9, -0.6] per corner, finding *likely
improvement*, decision inconclusive, recommendation continue (look 2 at 10 blocks). Confounders flagged:
time since start differs between blocks, and `brake_dist_m` +2.6 m [+0.8, +4.4].

**Response (variant minus baseline, 90 %):**

| Metric | Phase | all (14) | slow (7) | medium (3) | fast (4) |
|---|---|---|---|---|---|
| corner time (ms) | corner | -7.5 [-14.2, -0.9] down? | +8.8 [-7.4, +25.0] insuff. | -17.1 [-25.5, -8.7] down | -7.7 [-10.8, -4.7] down? |
| phase time (ms) | exit | -3.9 [-6.1, -1.6] down? | +4.0 [-2.7, +10.7] insuff. | -10.0 [-12.6, -7.4] down | -1.6 [-3.0, -0.2] down? |
| phase time (ms) | traction | -3.6 [-6.7, -0.5] down? | +2.0 [-5.3, +9.2] insuff. | -7.1 [-12.9, -1.3] down? | -3.6 [-4.5, -2.7] down |
| minimum speed (km/h) | mid | +0.2 [+0.1, +0.3] neutral | -0.2 [-0.6, +0.2] insuff. | +0.1 [-0.3, +0.5] insuff. | +0.7 [+0.5, +1.0] up |
| braking point (m) | corner | -0.2 [-0.3, +0.0] neutral | -0.1 [-0.3, +0.1] neutral | -0.5 [-1.0, +0.0] insuff. | insuff. |
| yaw gain | entry | +0.003 [+0.000, +0.006] up? | +0.001 [-0.004, +0.006] insuff. | +0.003 [+0.002, +0.004] neutral | +0.004 [+0.003, +0.006] up? |
| yaw gain | mid | +0.003 [-0.000, +0.006] insuff. | +0.000 [-0.006, +0.007] insuff. | +0.009 [+0.006, +0.011] up | +0.003 [-0.000, +0.006] insuff. |
| steer per g (deg/g) | mid | -0.12 [-0.41, +0.16] neutral | +1.25 [-0.32, +2.82] insuff. | -0.49 [-0.61, -0.36] down? | -0.05 [-0.17, +0.06] neutral |
| rear slip (deg) | mid | +0.010 [-0.014, +0.034] neutral | +0.084 [-0.036, +0.203] insuff. | +0.017 [-0.000, +0.034] neutral | +0.009 [-0.001, +0.019] neutral |
| rear slip (deg) | exit | +0.056 [+0.036, +0.075] up? | +0.072 [+0.025, +0.119] up? | +0.059 [+0.039, +0.080] up? | +0.024 [+0.016, +0.032] neutral |
| front slip (deg) | mid | -0.043 [-0.085, -0.001] down? | +0.161 [-0.043, +0.364] insuff. | -0.122 [-0.171, -0.074] down | -0.011 [-0.042, +0.020] neutral |
| slip balance F-R (deg) | mid | -0.043 [-0.092, +0.005] insuff. | +0.246 [-0.023, +0.516] insuff. | -0.145 [-0.180, -0.110] down | -0.009 [-0.043, +0.024] neutral |

**Against the prediction** (minimum speed within 0.3 km/h and braking point within 1 m, while rear slip,
yaw gain and steer per g move beyond their margins):

- *AI inputs did not compensate: held.* The absolute braking point is unchanged (-0.2 m, neutral in
  every regime) and minimum speed moved +0.2 km/h overall (+0.7 in fast corners). The verdict's
  `brake_dist_m` +2.6 m is the slowest point moving, not the braking point (finding I6): the
  "driver adapted" confounder is a false alarm from the apex-relative metric.
- *The car responds strongly: refuted for this change.* Rear slip moved +0.06 deg on exit (about a
  third of one click of rear wing on experiment 15, +0.17), yaw gain +0.003 (under 1 %, against +4 to
  +7 % for the wing), steer per g neutral overall. Only the medium corners (5, 8, 9) show a coherent
  balance shift: front slip -0.12 deg, slip balance -0.15 deg, yaw gain mid +0.009, steer per g -0.49.
- *Time:* the gain sits in exit and traction of medium and fast corners (-10 and -7 ms exit/traction
  at medium), where rear slip on exit rose only slightly. The slow corners carry no evidence either way.

Reading: on this car and AI, three clicks of rear ARB is a small response change, smaller than one
click of rear wing; it is not a case of "the response moved but the time did not". The prediction was
written for a large balance change and the change was not large on this car. Not yet known: whether
the time gain survives looks 2 and 3 (the time-since-start confounder is open), and whether the
response is monotonic in clicks (experiment D).

**Look 2 (10 blocks; blocks 9 and 10 in a second game session):** corner time -9.6 ms [-15.8, -3.5],
still *likely improvement*: the interval excludes zero but not the 5 ms margin, so the decision stays
inconclusive and the policy continues to look 3 (12 blocks). Time since start is left out of the model
(collinear with the session term, ADR 0022); the session term is flagged as between blocks. By class:
medium -14.9 ms [-19.5, -10.3], fast -7.1 [-9.8, -4.5], slow -12.0 [-26.8, +2.8].

**Fuel:** every block, both sessions, started its out-lap on 29.6 L and burned about 1.45 L a lap
(push lap 1 at 27.3 to 28.6 L in all ten blocks): the arms are equal. But not the 50 L the design
named: the agent took "the first block's fuel" from whatever the car held when the AI first took it
(at least 30 L), and block 9, after the relaunch, asked for 50 L and stopped on 26 L (the setup's own
`[FUEL] VALUE` landing after the set). Fixed in the agent: one fuel, 30 L, for every block of every
AI experiment, and an off read-back sets the fuel again during the out-lap before stopping (ADR 0028
amendment). Experiments before this change compared across sessions may differ in fuel.

**Look 3 (12 blocks, final; blocks 11 and 12 on the fixed-fuel agent, same 29.6 L out-lap as all
others), engine 0.4.0 after D1/D2/D3 were deployed:** corner time **-11.5 ms [-16.8, -6.3]**, finding
*confirmed improvement*, decision **accepted** (rear ARB 5 became the champion). Guardrails hold (rear
slip peak +0.06 deg, negligible). By class: medium -17.3 ms [-22.0, -12.7], fast -6.8 [-9.1, -4.5],
slow -16.1 [-29.5, -2.7]; a fast-vs-medium interaction and phase interactions (the gain is in exit
and traction, not braking or entry) are flagged, so the effect is context-dependent in size, not in
sign. The absolute braking point moved -0.19 m [-0.27, -0.11]: flagged as a driver-input shift
because the check has no margin, but 19 cm is far inside the prediction's 1 m.

Response findings (ADR 0034, first run through the deployed stack: core built the response context
from the real Parquet parts; 27 expected rear-ARB cells, block-level, Holm):

| Observable | Phase | all | medium | slow |
|---|---|---|---|---|
| slip balance F-R (deg, proxy) | mid | -0.081 decrease | -0.146 decrease | insufficient |
| slip balance F-R (deg, proxy) | exit | -0.073 decrease | -0.195 decrease | insufficient |
| yaw gain (direct) | mid | +0.0060 likely increase | +0.0099 increase | insufficient |
| yaw gain (direct) | exit | +0.0045 likely increase | +0.0102 increase | insufficient |
| rear slip (deg, direct) | exit | +0.057 likely increase | +0.081 increase | insufficient |
| rear slip (deg, direct) | mid | +0.009 neutral | +0.021 neutral | insufficient |
| rear traction slip ratio (direct) | exit | +0.0014 neutral | +0.0037 increase | neutral |
| steer reversals (/s, proxy) | mid, exit | insufficient | insufficient | insufficient |

**Final reading of experiment B.** The AI did not compensate through its inputs (braking point within
0.2 m, minimum speed +0.2 km/h). The car's response to rear ARB +3 is real but modest and
concentrated in the medium corners (5, 8, 9): the balance moves rearward (slip balance -0.15 to -0.2
deg), the car rotates more for its steering (yaw gain about +0.01) and the rear slips more on exit
(+0.08 deg) while traction slip rises slightly. The slow corners carry too few blocks of evidence
either way; the fast corners gain time with no response cell beyond its margin. And here the time did
show it: -11.5 ms per corner, confirmed. The prediction's "response moves beyond the margin
everywhere" was too strong; "response moves while the AI's inputs do not" held where the response
moved.

Process gaps seen: the prediction was recorded only as event text (`predictions` empty, not
machine-checked); the driver-input check now uses the absolute braking point (ADR 0034) but has no
margin, so a 0.19 m shift is flagged; and the accepted verdict promoted the change to champion,
which a hint sweep must avoid between its steps (fixed with `measure_only`, ADR 0032 amendment).

### Experiment C: grip hint (experiments 22 to 26, 2026-11-19)

Rear wing -1 (`WING_1 + WING_9`) against one baseline (setup 237: experiment 20's champion, rear ARB
5), measure-only, one experiment per pinned hint, run in the pre-registered order 0.93, 0.885, 1.0,
0.97, 0.9409 in one game session, 8 ABAB blocks each, engine 0.4.0. Driven unattended by a chain
that evaluated each step at look 1 and would have extended on `continue` (no step asked: every
recommendation was `investigate_context`).

**Conditions held.** The hint read back on every pass equals the pin (min = max). Push lap 1 fuel
27.82 to 27.89 L in every experiment. No invalid pass, no damage, off-track rate 0 in every arm
(`reset_counter` counts the agent's own teleports, not incidents). 336 passes each, except exp 24
(hint 1.0): 329, one block-3 lap short of 7 passes (not investigated).

**Pace by hint** (mean corner pass over both arms; between experiments, so time of day and track
temperature are not controlled):

| Hint | 0.885 | 0.93 | 0.9409 | 0.97 | 1.0 |
|---|---|---|---|---|---|
| Mean pass (s) | 5.734 | 5.662 | 5.656 | 5.636 | 5.645 |
| vs 0.9409, per lap (s) | +1.09 | +0.08 | 0 | -0.28 | -0.15 |

The AI gets faster with the hint up to 0.97 and no faster at 1.0.

**Corner time, rear wing -1 minus baseline (ms per corner, 90 %, block-level):**

| Hint | all | slow | medium | fast | Finding |
|---|---|---|---|---|---|
| 0.885 | +15.6 [+2.3, +29.0] | +20.9 [+5.4, +36.4] | +13.6 [+7.7, +19.5] | +17.8 [+10.6, +25.0] | context-dependent |
| 0.93 | +1.1 [-5.4, +7.6] | +7.8 [+0.3, +15.2] | +6.6 [-5.7, +18.9] | -10.0 [-14.4, -5.6] | context-dependent |
| 0.9409 | +2.9 [-4.2, +9.9] | -12.2 [-20.3, -4.1] | +11.8 [+1.1, +22.5] | -1.3 [-7.0, +4.4] | context-dependent |
| 0.97 | -1.5 [-17.6, +14.5] | +8.1 [-2.0, +18.2] | +1.3 [-21.2, +23.8] | -15.3 [-19.7, -10.9] | context-dependent |
| 1.0 | +1.7 [-4.0, +7.4] | +6.4 [-2.1, +14.9] | +8.1 [-3.1, +19.2] | -7.6 [-11.5, -3.6] | context-dependent |

**Response, rear wing -1 (all corners; Holm-adjusted findings, every one listed moved beyond its margin):**

| Hint | rear slip mid (deg) | rear slip exit (deg) | yaw gain mid | slip balance mid (deg) | top-speed proxy, fast (km/h) |
|---|---|---|---|---|---|
| 0.885 | +0.11 | +0.10 | +0.02 | -0.23 | not moved |
| 0.93 | +0.11 | +0.10 | +0.02 | -0.24 | +1.56 |
| 0.9409 | +0.11 | +0.19 | +0.03 | -0.32 | +0.72 |
| 0.97 | +0.15 | +0.15 | +0.01 | -0.31 | +1.22 |
| 1.0 | +0.20 (fast) | +0.13 (fast) | +0.03 | -0.41 | not moved |

**AI inputs:** at every hint the AI brakes about 1 m earlier with less wing (-0.95 to -1.26 m,
absolute braking point). At 0.885 it also picks up the throttle later (+0.9 m) and corrects more.

**Against the hypothesis** ("higher hints raise corner speeds and expose larger time effects of the
same change; incidents rise"):

- *Speeds rise with the hint: held up to 0.97*, flat at 1.0.
- *Larger time effects at higher hints: not supported.* The pooled time effect is near zero at every
  hint from 0.93 up. What is stable is its *shape*: less rear wing is faster in the fast corners
  (-7.6 to -15.3 ms at 0.93, 0.97, 1.0) with more top speed after them (+0.7 to +1.6 km/h), and
  slower or unchanged in slow and medium ones. That is the drag/downforce trade-off, and it is why
  every step is context-dependent: the pooled number is the wrong question for an aero change
  (experiment 15's replay found the same: slow +8.7, fast -9.1 ms). 0.9409's slow-corner gain is the
  exception; one experiment cannot say whether it is real.
- *Larger response at higher hints: partly supported.* The balance shift grows with the hint (slip
  balance mid -0.23 at 0.885 to -0.41 at 1.0; rear slip in fast corners +0.09 to +0.20 deg): the
  closer the AI drives to the limit, the more of the setup's effect on balance shows. Direction and
  rough size match experiment 15 at every hint, on a different baseline (rear ARB 5, not 2): the
  rear-wing response replicates.
- *Incidents rise: not seen.* None at any hint, so by the design rule (highest hint with incidents
  in at most one block) 1.0 is usable; it is not faster than 0.97.
- *At 0.885 the AI drives the change differently:* slower everywhere (+15.6 ms), brakes earlier,
  throttles later. A low hint leaves margin the AI's plan does not use, and its plan reacts to the
  setup's aero rather than to grip it is not near: 0.885 hides the trade-off.

**Reading.** For this car and AI, 0.97 is the most useful hint: the fastest AI, the largest balance
response, no incidents. Time is not a sufficient observable for an aero change; per-class time and
the response cells are. The AI's planner reacts to downforce (about 1 m earlier braking for one
click less wing at every hint), so "the AI does not compensate" from experiment B holds for a
mechanical change (ARB) but not for an aero one: the braking point is part of the response here.

**Limits.** One experiment per hint, so the hint trend is between experiments, at different times of
the night (time since start is flagged between blocks in all five). Look 1 only. The 1.0 run is short
one lap. The usable-hint choice should be confirmed with one A/A at 0.97 (experiment A) before 0.97
becomes the default pin.

The five reverts were recorded as "The driver kept the baseline", which is wrong for measure-only
runs; reverting a measure-only experiment now says nothing was decided (the five events stay as
written: the ledger is append-only).

### Experiment A: repeatability, A/A (experiment 27, 2026-11-19)

Setup 237 (rear ARB 5) in both arms (rear ARB 0 clicks, measure-only, ADR 0032 amendment), hint
pinned at 0.97, fixed 30 L fuel, engine 0.4.0. Pre-registered: the time finding is neutral or
insufficient, never an improvement or regression; corner time for all corners covers 0 at 90 %; no
response cell moves beyond its margin after Holm. One design deviation: one experiment in one game
session plus the extension in a second, not 3 sessions x 8 blocks, so it measures the false-decision
behaviour of one run, not session variance.

| Look | Blocks | Corner time, all (ms) | Finding | Recommendation |
|---|---|---|---|---|
| 1 | 8 | -0.04 [-5.8, +5.8] | insufficient evidence | continue |
| 2 | 10 | **+0.14 [-3.7, +3.9]** | **neutral** | stop (neutral) |

420 passes, none excluded. By class: slow +2.4 [-4.1, +9.0], medium -1.1 [-12.0, +9.8], fast -0.8
[-4.4, +2.8]. Guardrails negligible. Response: 22 cells, 18 insufficient, 4 neutral, **none moved**.

**Against the prediction: held on every count.** The time interval covers 0 at both looks and the
equivalence test closed at look 2 inside the 5 ms margin; no response cell moved. At 10 blocks an
identical pair is resolved to about +-4 ms per corner, so an effect has to be about the size of the
margin to be called at that look.

**One failure: the driver-input check.** The absolute braking point "moved" -0.27 m [-0.43, -0.11]
between identical arms and was flagged as `driver_input_shifted`. With nothing to respond to, that
is either an order or time effect inside the ABAB blocks or an interval that is too narrow; either
way the check fires on noise. It is as large as the flags on rear ARB +1 and +2 below (-0.24 m) and
experiment B's -0.19 m. Until the check has a margin and a calibrated interval, a braking-point flag
under about 0.3 m on this car says nothing.

*Fixed (engine 0.5.0, ADR 0034 amendment):* the interval was not the problem (simulated, the old
check flagged 11 % of identical runs for one channel, its nominal rate); the missing margin was (a
real 0.3 m wobble was flagged 96.5 % of the time). A driver input now shifts only when its whole
interval lies beyond its margin (1 m for braking and throttle points, 0.1 /s for steering
corrections). Re-judged on the stored intervals, experiments 20 to 31 keep 3 of their 11 flags:
experiment 22's braking point (-1.26 m, rear wing -1 at 0.93) and the steering corrections of
experiments 25 and 30. The A/A's flag is gone. Whether experiment 27's -0.27 m was chance or a
small order effect inside the ABAB blocks, one A/A cannot say.

**Reading.** At hint 0.97 the inference did not invent effects on the primary metric or the response
cells in this run, so 0.97 is fit to be the default pin for this car (a calibration change, not made
here). One A/A is one draw: it rules out a gross false-decision rate, not a 5 % one.

### Experiment D: rear ARB curve (sweep 1, experiments 28 to 31, 2026-11-19)

The first sweep through `POST /sessions/{id}/sweeps` (ADR 0032): rear ARB -1, +1, +2, +3 from setup
237 (rear ARB 5, so ARB 4, 6, 7, 8), hint 0.97, fixed fuel, the baseline back on the car between
steps, each step its own experiment under the sequential policy, curves classified by the engine at
the end. Driven unattended; no step failed, none skipped, 336 passes a look, none excluded.

**Steps (corner time, variant minus baseline, ms per corner, 90 %):**

| Step (ARB) | Exp | Looks | all | slow | medium | fast | Finding / recommendation |
|---|---|---|---|---|---|---|---|
| -1 (4) | 28 | 1 | +8.0 [-0.8, +16.9] | +4.3 [-10.4, +19.0] | +8.3 [+3.9, +12.7] | +10.3 [+4.0, +16.6] | insufficient / stop (futility) |
| +1 (6) | 29 | 3 | -2.4 [-8.1, +3.3] | -1.1 [-10.7, +8.6] | -3.4 [-11.4, +4.6] | -3.4 [-8.1, +1.3] | insufficient / larger perturbation |
| +2 (7) | 30 | 3 | -9.5 [-17.4, -1.6] | -11.9 [-22.2, -1.5] | -1.7 [-9.7, +6.3] | -10.1 [-13.0, -7.2] | likely improvement / larger perturbation |
| +3 (8) | 31 | 1 | -11.0 [-21.1, -0.9] | -6.6 [-15.5, +2.3] | -7.3 [-20.6, +6.0] | -13.1 [-15.3, -10.9] | context-dependent / investigate context |

Step -1 stopped at look 1 because its interval cannot reach an improvement (futility), although its
point estimate is 8 ms slower; "stop (neutral)" is the policy's word for that and reads oddly here.

**Curves (engine fit across the four steps; slope per click, 90 %):**

| Observable | Phase | Regime | Shape | Slope per click |
|---|---|---|---|---|
| corner time (ms) | corner | all | linear | -4.1 [-5.9, -2.4] |
| corner time (ms) | corner | fast | linear | -4.5 [-5.2, -3.9] |
| corner time (ms) | corner | slow | linear | -3.0 [-5.5, -0.6] |
| corner time (ms) | corner | medium | nonlinear (flattening) | -3.6 [-6.0, -1.3] |
| yaw gain | mid | all | nonlinear (flattening) | +0.0026 [+0.0017, +0.0035] |
| yaw gain | exit | all | linear | +0.0028 [+0.0019, +0.0037] |
| rear slip (deg) | exit | all | linear | +0.021 [+0.010, +0.032] |
| slip balance F-R (deg) | mid | all | nonlinear | -0.038 [-0.059, -0.017] |
| slip balance F-R (deg) | exit | all | nonlinear | -0.030 [-0.042, -0.017] |
| steer reversals (/s) | exit | all | linear | +0.020 [+0.004, +0.035] |
| rear traction slip ratio | exit | all | negligible | 0.0000 |
| rear slip peak (deg, guardrail) | corner | all | negligible | +0.023 (inside +-0.5) |
| countersteer (s, guardrail) | corner | all | negligible | 0.000 |
| braking point (m, AI input) | corner | all | linear | -0.15 [-0.19, -0.10] |
| throttle pickup (m, AI input) | corner | all | linear | -0.32 [-0.42, -0.22] |
| steer corrections (/s, AI input) | corner | all | linear | +0.073 [+0.048, +0.098] |

37 curves in all: 12 insufficient (6 of them with only one step's data in their cell), the rest
above or in slow-corner cells. Rear traction slip in slow corners is classed as a direction change
at +1, inside its margin.

**Against the hypothesis** ("rear ARB response is monotonic in clicks and phase-specific: mid/exit,
slow corners"):

- *Monotonic: held.* Time falls about 4 ms per corner per click from ARB 4 to ARB 8 with no
  curvature overall, and the balance channels move one way across all four steps: more yaw gain,
  more rear slip on exit, slip balance rearward. The curve has not turned by ARB 8, so the time
  optimum on this car and AI is at or beyond the top of the swept range.
- *Phase-specific (mid/exit): held.* Every balance curve that moved is in mid or exit; traction slip
  did not move.
- *Slow corners: not supported.* The fast corners carry the clearest time slope (-4.5 ms per click,
  the tightest interval); the slow corners' slope is smaller and wide. Experiment B's medium-corner
  concentration does not repeat as a curve: medium flattens.
- *Replication of experiment B per click:* B's +3 clicks (ARB 2 to 5) gave yaw gain +0.0045 to
  +0.006, rear slip exit +0.057, slip balance -0.07 to -0.08 (all corners), so about +0.002, +0.019
  and -0.025 per click; the sweep from ARB 5 gives +0.0026 to +0.0028, +0.021 and -0.030 to -0.038.
  Same direction, similar size per click, on a different stretch of the bar.

**The sweep's main lesson for the method.** At the step level almost every response cell was
*insufficient* (20 of 22 at each step): one click of rear ARB moves yaw gain by about half its 0.005
margin, so no single A/B can call it. The curve, fitting four steps against one baseline, classifies
the same channels with intervals clear of zero. For small-response parameters the sweep is the
experiment, not the individual step verdicts.

**AI inputs.** The AI does adapt to the bar: it picks the throttle up earlier (-0.32 m per click) and
corrects more (+0.07 /s per click) as the rear stiffens. The braking-point slope (-0.15 m per click)
must be read against experiment A: identical arms differed by -0.27 m, so only step +3's -0.51 m is
clearly beyond that noise. "The AI does not compensate" from experiment B needs a qualifier: its
braking does not, its throttle does, in the direction a car that rotates more allows.

**Reading.** On this car, track and AI at 0.97, stiffer rear ARB is faster up to at least ARB 8,
linearly, mostly in fast corners, through a car that rotates more and a throttle that comes in
earlier, with the rear-slip guardrails far inside their margins. Next for this parameter: extend the
sweep to the top of the range (ARB 9 and up, if the car allows), and check one step with the human
driver on the regular layout before trusting the direction there (unverified with a human driver).
The steps are measure-only and promoted nothing; the champion is still setup 237.

**Limits.** One sweep, one baseline. Steps +1 and +2 were extended into later game sessions (flagged
between blocks); -1 and +3 stopped at look 1, so the steps have unequal precision. Time since start
is flagged between blocks in all four.
