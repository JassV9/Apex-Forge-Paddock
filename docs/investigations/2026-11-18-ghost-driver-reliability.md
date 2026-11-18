# Ghost Driver reliability and vehicle sensitivity: audit, findings and experiment design

Date: 2026-11-18. Status: Phase 1 (audit) and Phase 2 (design) complete; implementation tracked in
the ADRs this document names. Four independent read-only audits (statistics, AI behaviour,
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
