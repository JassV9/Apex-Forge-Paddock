# ADR 0031: Corner phases at fixed distances, per-corner speed regimes, vehicle-response analysis

Status: accepted (2026-11-18); amended by [ADR 0034](0034-vehicle-response-in-verdicts.md)
(margins moved to `config/margins.toml`; the verdict judges the expected cells with Holm).
Implements findings C4, I6 and I7 and sections 3 and 4 of
[the reliability investigation](../investigations/2026-11-18-ghost-driver-reliability.md).
Amends the phases of the measurement path (ADR 0008's inputs) and the speed classes of the
verdict and the diagnosis.

## Context

Three measurement faults, all seen on the real rows (investigation, section 2):

- **C4. The speed class was per pass**, from that pass's own minimum speed: an outcome of the
  setup. 56 (experiment, corner) pairs had passes in more than one class; on experiment 17,
  corner 6 was 38 % "slow" in one arm and 0 % in the other. The fixed 100/150 km/h bands also
  fit no car: they gave the Formula Hybrid 2026 at the Hungaroring one slow corner.
- **I6. Driver events were measured from the per-pass slowest point**, which jitters 20 to 46 m
  between passes while the braking point itself moves less than 4 m. On experiment 15 the
  baseline's interquartile range of the braking point is at most 4.5 m per corner, of the
  slowest point up to 17 m, of turn-in up to 46 m (corner 12).
- **I7. The exit phase was empty** in 76 to 100 % of passes at several corners, and
  `exit_speed_100m_kmh` null in 12 of 14 Hungaroring corners: phases were cut per pass in time,
  and a corner's zone ends where the next corner's begins.

Section 1 of the investigation adds the reason this matters: with a plan-limited AI, corner time
barely moves (rear wing -1: 0.0 [-5.1, +5.1] ms) while the car's response does. Response channels
need phases that are the same metres of track in both arms.

## Decision

### 1. Distance alignment (`phases/align.py`)

Each pass is put on a 1 m grid: spline distance unwrapped across the start line and expressed in
metres from the zone start, samples that do not move forward dropped (the spline position steps
back by centimetres; resampling needs increasing positions), and time, speed, inputs, g, yaw,
axle slip angle and ratio and ride height interpolated onto whole metres. The time a pass spends
between two metres is `t(x_end) - t(x_start)`, so **phase times sum to the corner time** exactly
(test). `delta_t(x)` against the baseline's median trace is a descriptive trace, reported every
10 m and never used for inference. Tested with a synthetic pass sampled at the agent's 15 to
22 ms jitter, a backward step and a wrap across the line: a known 5 % slow stretch is recovered
within 2 ms.

### 2. Events and frozen cuts (`phases/events.py`, `phases/cuts.py`, `phases/reference.py`)

Per pass, in metres from the zone start: **B** first brake > 0.08 before the apex; **TI** the
start of the run of steering at or beyond 50 % of the pass's 95th-percentile steering, in the
apex's direction, that leads to the apex; **A** the slowest metre within 30 m of the corner (on
a flat-out corner, no braking and < 4 % speed loss, the curvature peak, yaw rate / speed over
5 m); **P** gas > 0.25 and brake < 0.04 held 0.2 s, searched from B (or from the lift);
**E** steering < 20 % of its p95 **and** lateral g < 50 % of its p95, held 0.15 s, after A.
Percentiles come from the pass's own zone, never from the joined next pass.

*E uses "and", not the "or" first proposed.* With "or", a downforce car's slow corner "ended" at
its slowest point, where lateral g is lowest: on experiment 15 the exit was merged away at 4 of
14 corners (2, 4, 6, 8) and E sat at the zone end at corner 6. With "and" it is merged at 2
(corners 2 and 4, where the AI genuinely straightens within 5 m of the slowest point).

**Cuts are frozen per experiment** from the medians of the baseline arm's events, all baseline
blocks pooled, and every pass of both arms is measured over those metres:

| Phase | From | To |
| --- | --- | --- |
| braking | B | TI |
| entry | TI | A - w |
| mid | A - w | A + w |
| exit | A + w | E |
| traction | E | E + 150 m, or the baseline passes' median reach |

w is half the stretch around A where the baseline's median speed stays within 3 % of its
minimum (flat-out: +-0.3 s of travel), at least 0.15 s of travel, at most 40 % of the shorter
of A - TI and E - A, and never under 2.5 m. The corner has a braking phase only if at least
half the baseline passes brake. Boundaries are made monotonic in that order (a mid pushed by the
order keeps its width). A phase under 5 m is merged into its neighbour (braking into entry,
entry and exit into mid) and flagged with its length; the traction phase is `unavailable` when
the passes end within 5 m of E. **Each of the five phases always has a status** (`ok`,
`merged`, `absent`, `unavailable`) and a note when not `ok`: hypothesis-tested over arbitrary
(including contradictory) event sets.

**Joined passes** (`phases/join.py`): a pass is followed by the next pass of the same session
when their zones touch (within 2 m) and the next started within 1 s of this one's end; its
samples are appended with its clock shifted by the difference of `t_start`. Exit and traction
then exist in chicanes: on experiment 15 the traction phase is cut (150 m) at all 14 corners.

Driver events in the per-pass features are now also absolute: `brake_point_m` and
`throttle_pickup_point_m` (metres from the zone start). `brake_dist_m` and `throttle_pickup_m`
stay, because core's driver-adapted check (`experiments/create.go`) and the AI reference
(`reference/features.go`, the web's reference table) read them by name; switching those needs
the stored features recomputed first, or re-judging an older experiment would silently lose two
driver checks. That switch is left to a later change.

### 3. Speed regimes per corner (`features/regimes.py`)

The class belongs to the corner and is frozen from the baseline: the median minimum speed of the
corner's baseline passes. Cut-offs are per car and track: the two largest gaps between the
corners' log median minimum speeds such that every class keeps at least 2 corners, each cut at
the geometric mean of the speeds either side. Fewer than 6 corners with a speed: the fixed
100/150 km/h bands, still on corner medians. A corner whose passes carry no minimum speed keeps
its most common stored class. On the real medians (fixture `corner_medians_2026-11-18.json`):

| Car, track | Breaks (km/h) | slow | medium | fast |
| --- | --- | --- | --- | --- |
| FH26, Hungaroring | 130.2, 161.4 | 1, 2, 6, 7, 12, 13, 14 | 5, 8, 9 | 3, 4, 10, 11 |
| FH26, COTA (10 corners recorded) | 158.4, 225.9 | 1, 8, 9 | 2, 5, 6, 7, 10 | 3, 4 |
| Ferrari 488 GT3, COTA | 82.8, 150.6 | 1, 11, 12, 13, 14, 17 | 2, 5, 6, 7, 8, 9, 10, 15, 16 | 3, 4 |

Without the 2-corner rule the FH26 at COTA would make corner 1 (86 km/h) a class of its own.
`experiments/channel_effects.py` (the verdict's per-class effects) and `diagnosis/evidence.py`
use these classes; a class effect needs at least 3 corners (finding I4) and is otherwise left
out. The per-pass `speed_class` field stays in the contracts for compatibility and decides
nothing. The dead agent `corners.speedClass` is removed.

**Aero share: an ESTIMATE.** Over the corners' baseline medians, least squares
`peak lateral g = c0 + c1 v^2` (v the median minimum speed in m/s, peak lateral g the median of
the passes' 95th-percentile |lat g|); the share at a corner is `c1 v^2 / (c0 + c1 v^2)`. It is
reported only when R^2 >= 0.5, c0 > 0 and c1 > 0, and it decides nothing. The fit lumps
downforce with everything else that differs between corners (camber, banking, kerbs, the AI's
plan, the speed at which peak g is reached), so the share is an ordering of corners, not a
measurement of downforce. Real fits: FH26 Hungaroring R^2 0.73 (adequate; T4 at 202 km/h about
0.34), FH26 COTA 0.33 and 488 GT3 COTA 0.15 (not reported).

### 4. Vehicle-response analysis (`engine/paddock_engine/response/`, `POST /response`)

`response/metrics.py` computes, per pass and frozen phase, on the 1 m grid (distance-weighted
means; None, never 0, with fewer than 3 qualifying metres): phase time; speed at start, end,
minimum and maximum; mean |lat g|, mean long g, peak deceleration; steer per g and yaw gain on
cornering metres; front and rear axle slip angle (CSP's per-wheel `slipAngle`, load-weighted:
direct) and their difference (proxy); front and rear slip ratio under braking and on the
throttle; lock share (proxy for lock events); steering rate and reversals (corrections);
coasting time; brake and throttle application; ride height front and rear and their standard
deviation (platform variance, proxy). Per pass and corner: corner time (sum of the phases) and
the five events in absolute metres.

What each metric is, its unit, phases, kind (`direct` or `proxy`) and margin is configuration
(`response/metrics.toml`), as are the observables the channels cannot give, listed and never
estimated (normalised slip, body slip, wheel speed difference, suspension travel, ABS/TC,
inner/outer tyre temperature). The parameter-to-observable mapping of investigation section 4 is
`response/parameters.toml`: per row, the catalog role and axle it matches, direct observables and
proxies apart, phases, regimes, and the unavailable observables it would need. A test checks the
configuration against the code both ways. The margins are initial judgement from experiment 15's
scatter, to be replaced by ADR 0029's margins configuration.

`POST /response` (contracts `response_request`, `response_response`) takes an experiment's
passes (session, seq, arm, block, lap, `t_start_s`, duration, validity, covariates), the corner
map and each session's Parquet directory. Invalid passes are never compared but may complete the
previous pass's traction. For each configured (metric, phase) and each regime (all, slow,
medium, fast) it calls the existing `stats.effects.estimate_effects` unchanged (block bootstrap,
corner weights; ADR 0008, 0023) and reads a finding from the interval and the margin: increase,
decrease, neutral (inside the margin), likely increase / decrease (excludes 0, not the margin),
insufficient (otherwise, not on both arms, fewer than 3 corners in a regime, or not estimable).
Cells the changed item's parameter row lists are marked `expected: direct|proxy`. Core does not
call the endpoint yet.

## Replay: experiment 15 (rear wing -1, `WING_1 + WING_9`, FH26, Hungaroring)

Offline and read-only: the passes from the spool files of session 105, arm, block, lap and
validity from a SELECT on the live database; 336 push-lap passes (8 blocks x 3 laps x 14
corners), none excluded. Regimes as above (gaps at 130.8 and 161.5 km/h on this experiment's
baseline). Variant minus baseline per corner pass, 90 % intervals; "up?"/"down?" are likely
increase/decrease, "insuff." insufficient (the fast braking cell has 2 corners: too few).

| Metric | Phase | all (14) | slow (7) | medium (3) | fast (4) |
| --- | --- | --- | --- | --- | --- |
| corner time (ms) | corner | +4.6 [-2.8, +11.9] insuff. | +10.7 [-34.0, +55.5] insuff. | +10.6 [-11.7, +32.8] insuff. | -2.4 [-9.1, +4.3] insuff. |
| phase time (ms) | braking | +5.5 [+4.6, +6.4] up | +8.9 [+7.1, +10.8] up | +3.3 [+1.8, +4.8] up? | insuff. |
| phase time (ms) | entry | +1.8 [+1.0, +2.5] up? | +0.3 [-1.8, +2.3] insuff. | +2.6 [+0.9, +4.3] up? | +2.8 [+1.0, +4.5] up? |
| phase time (ms) | mid | -1.9 [-3.0, -0.8] down? | -3.8 [-6.1, -1.6] down? | +0.7 [-0.7, +2.0] neutral | -1.0 [-2.0, -0.1] neutral |
| phase time (ms) | exit | -3.5 [-8.1, +1.1] insuff. | +2.0 [-21.6, +25.6] insuff. | -1.3 [-7.4, +4.9] insuff. | -3.5 [-5.6, -1.4] down? |
| phase time (ms) | traction | -0.1 [-5.0, +4.8] insuff. | +0.9 [-9.9, +11.7] insuff. | +5.5 [-9.4, +20.5] insuff. | -3.9 [-7.7, -0.2] down? |
| minimum speed (km/h) | mid | +0.5 [+0.3, +0.8] up? | +0.8 [+0.4, +1.2] up | +0.1 [-0.4, +0.5] insuff. | +0.5 [-0.1, +1.0] insuff. |
| top-speed proxy (km/h) | traction | +0.2 [-0.3, +0.6] insuff. | -0.4 [-1.2, +0.4] insuff. | -0.1 [-1.5, +1.2] insuff. | +1.0 [+0.6, +1.3] up |
| yaw gain | entry | +0.014 [+0.010, +0.019] up | +0.007 [-0.003, +0.017] insuff. | +0.013 [+0.008, +0.018] up | +0.021 [+0.017, +0.025] up |
| yaw gain | mid | +0.020 [+0.013, +0.027] up | +0.016 [-0.005, +0.038] insuff. | +0.022 [+0.013, +0.031] up | +0.022 [+0.017, +0.027] up |
| steer per g (deg/g) | mid | -0.78 [-1.25, -0.31] down? | +0.67 [-1.32, +2.67] insuff. | -0.89 [-1.43, -0.36] down? | -1.07 [-1.28, -0.85] down |
| rear slip (deg) | mid | +0.108 [+0.040, +0.175] up? | +0.140 [-0.088, +0.367] insuff. | +0.231 [+0.166, +0.295] up | +0.050 [+0.025, +0.075] up? |
| rear slip (deg) | exit | +0.169 [+0.133, +0.204] up | +0.155 [+0.065, +0.246] up | +0.185 [+0.114, +0.257] up | +0.121 [+0.106, +0.136] up |
| front slip (deg) | entry | -0.139 [-0.205, -0.073] down | -0.035 [-0.164, +0.093] insuff. | -0.159 [-0.239, -0.079] down | -0.190 [-0.254, -0.126] down |
| front slip (deg) | mid | -0.088 [-0.185, +0.009] insuff. | +0.066 [-0.276, +0.407] insuff. | -0.117 [-0.249, +0.014] insuff. | -0.200 [-0.280, -0.120] down |
| slip balance F-R (deg) | mid | -0.173 [-0.269, -0.077] down | +0.044 [-0.737, +0.826] insuff. | -0.371 [-0.511, -0.231] down | -0.175 [-0.247, -0.103] down |
| mean lat g | mid | +0.005 [-0.004, +0.014] insuff. | +0.005 [-0.018, +0.027] insuff. | +0.008 [-0.006, +0.023] insuff. | -0.024 [-0.048, +0.000] insuff. |
| rear ride height (mm) | mid | +0.5 [+0.4, +0.7] up | +0.1 [-0.2, +0.3] insuff. | +1.1 [+0.9, +1.2] up | +0.9 [+0.7, +1.0] up |
| braking point (m) | corner | -1.3 [-1.5, -1.1] down | -1.8 [-2.0, -1.7] down | -0.8 [-1.4, -0.3] down? | insuff. |

Read with care (below), the picture is coherent with less rear wing: the rear sits higher (+0.5 to
+1.1 mm) and slips more (+0.1 to +0.2 deg mid and exit, every regime), the car rotates more for
its steering (yaw gain +4 to +7 %), needs less steering per g in medium and fast corners, the
front slips less on entry, the AI brakes about 1.3 m earlier and spends +5.5 ms more in braking,
gains it back in mid and exit, and reaches +1.0 km/h more in the 150 m after fast corners.
**Corner time stays insufficient** (+4.6 [-2.8, +11.9] ms): the response moved, the time did not
show it, as the investigation's section 1 found from the stored features. Of 344 cells, 16 were
listed by the rear-wing row; 8 of those increased and 4 decreased.

## Consequences

- An experiment's phases are the same metres in both arms; a setup that moves the slowest point
  cannot move the boundaries, and no phase is silently empty.
- The verdict's and the diagnosis's speed classes change for existing data; on the FA26 COTA
  demo the slow class now has 37 passes (was 32) and front slip 7.33 deg (was 7.67), and the
  explain tests were updated to those numbers. Stored verdicts are not re-judged.
- `POST /response` takes about 25 s on 336 passes: 319 of its 344 cells run the effect model's
  bootstrap (400 resamples each). It is an analysis call, not on the ingest path.

## Known limits

- **No multiplicity control across cells.** 344 cells at 90 % means about 34 exclude zero by
  chance. Findings read as "this moved", never as a decision; the decision stays with the
  pre-registered test (ADR 0029 adds Holm for driver channels; not applied here).
- **The intervals inherit finding C2**: the bootstrap resamples laps within blocks, so block-level
  error is not seen and the narrow response intervals above (e.g. rear ride height +-0.05 mm)
  are optimistic. ADR 0029's block-level inference replaces the interval inside
  `estimate_effects`; this package calls it unchanged and picks the fix up.
- The `delta_t` trace is unadjusted (no covariates, a median of medians): on experiment 15 its
  end values are -54 to +35 ms per corner, raw differences that the model's covariates and corner
  weights net out to +4.6 ms. It is shown, never tested.
- Margins are judgement; event thresholds (50 %, 20 %, 0.15 s, 0.2 s, 3 %) are the work package's
  and checked on one car only. TI is noisy on the AI (IQR up to 46 m at corner 12); its median is
  stable, a single pass's TI is not.
- The aero share is a cross-corner regression, not a measurement; it is never used to decide.
- Body slip, normalised slip, wheel speeds, suspension travel and ABS/TC are not recorded (ADR
  0028 adds the CSP channels); their observables are listed as unavailable.
- Per-pass features (`POST /features/session`) still cut phases per pass with `phases/segment.py`;
  `exit_speed_100m_kmh` there is still null where the zone ends. The frozen phases exist only
  in the experiment-level response analysis.
- **Unverified in game.** Everything above ran offline on recorded rows; the endpoint has not
  been called by core or from a browser.

## Alternatives considered

- **Cutting per pass, in time** (the v0 port): the faults above.
- **Events on the median trace** instead of medians of per-pass events: the median trace is not a
  pass anyone drove; per-pass events with a median keep each event an observable of real passes
  and give its scatter.
- **Speed classes per pass, frozen per experiment** (majority vote): still lets the variant vote.
- **Quantile bands per track** (thirds of the corners): cuts through clusters; the gap rule cuts
  between them.
- **E with "or"**: measured and rejected, above.
