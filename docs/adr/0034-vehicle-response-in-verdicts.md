# ADR 0034: Vehicle response in verdicts

Status: accepted (2026-11-19). Integrates [ADR 0029](0029-block-level-inference-and-finding-classes.md)
(findings, Holm, margins) with [ADR 0031](0031-corner-phases-speed-regimes-and-response.md)
(frozen phases, per-corner regimes, the response analysis), which were written in parallel.
Implements section 5's last sentence of the
[reliability audit](../investigations/2026-11-18-ghost-driver-reliability.md): "a finding on time
and a finding on vehicle response are separate". (0032 and 0033 are taken by changes written in
parallel.)

## Context

The audit's central result (section 1): with a plan-limited AI, corner time barely moves while
the car's response does (experiment 15, rear wing -1: time 0.0 ms, rear slip and yaw gain up).
After ADR 0029 and 0031 the engine could say both, but in two places that did not meet:

- the verdict's context-dependent test still classed corners by the per-pass `speed_class`,
  frozen by the baseline's majority (ADR 0029, "Not verified"), while the per-class results used
  ADR 0031's per-corner regimes: two maps of the same corners in one verdict;
- the verdict had no phase interactions (they waited for ADR 0031's frozen phases);
- `POST /response` judged 344 cells with no multiplicity control, against margins of its own
  (`response/metrics.toml`), beside ADR 0029's `config/margins.toml`; core never called it, and
  `switch_metric` ("time neutral, response moved") read only guardrails and predictions.

## Decision

### 1. One regime map per verdict

The verdict's speed regimes are per corner, frozen from the baseline (`features/regimes.py`): from
the raw samples' slowest metre when the verdict has them (the same baseline passes that freeze
the phases, `experiments/vehicle.py`), else from the compared passes' `v_min_kmh`
(`experiment_regimes`), else the corner's most common stored class. The per-class results, the
"at least 3 corners" rules of `experiments/context.py` and the response cells all use that one
map. A pass's own `speed_class` never moves its corner (test: flipping every variant pass's class
leaves the verdict identical).

### 2. Context-dependent: regimes and phases, one Holm family

`experiments/context.py` tests two families of block-level interactions (`stats/interaction.py`,
unchanged) together, Holm at 5 % across all pairs:

- **regimes**: every pair of speed regimes with at least 3 corners each, on the primary channel;
- **phases** (primary channel corner time, raw samples present): every pair of the frozen phases
  braking, entry, mid, exit, traction present in at least 3 corners, on `phase_time_s`, which
  sums to the corner time. Each (corner, phase) is a group of the random-intercept fit; per block,
  the corner-weighted mean residual of one phase less another's, compared between the arms with
  a two-sample t on the blocks. A setup that loses in braking what it gains on exit is
  context-dependent (confounder kind `phase_interaction`).

### 3. Response findings beside the time finding

`EvaluateRequest.response` (optional) carries what `/response` needs: track length, corner
geometry, each session's Parquet directory, the changed item's catalog role and axle, and the
push passes on the arm's setup from the experiment's driver, **off-track ones included** (never
compared; they may complete the previous pass's traction), with the compared passes' block, lap
codes and covariates. Core builds it in the same request (`core/internal/experiments/vehicle.go`):
no second round trip. The engine loads the samples (`experiments/vehicle.py`), freezes phases and
regimes from the baseline, and measures only the cells the verdict needs.

The cells `response/parameters.toml` marks expected for the changed parameter (direct and proxy,
its phases, its regimes plus "all") each get the effect model's block-level interval
(`stats.effects.estimate_effects`, ADR 0029) at the success test's level and a **response finding**
(`experiments/response_findings.py`). Holm runs across the expected cells at 5 %, family-wise,
two-sided; a cell has *moved* when Holm rejects "no change":

| Response finding | Rule |
| --- | --- |
| increase / decrease | moved, the whole interval beyond the margin, at least 8 blocks |
| neutral | the whole interval inside the margin |
| likely increase / decrease | moved, not by the margin (or fewer than 8 blocks) |
| insufficient | otherwise; not estimable; fewer than 3 corners in the regime |

Only a moved cell claims a direction. Cells the parameter is not expected to move are not judged
in the verdict: they stay descriptive in `POST /response` (kept for ad-hoc analysis) and never
drive a finding. The response findings never change the time finding or the ledger decision;
they steer the recommendation: `switch_metric` fires when an expected cell moved (or, as before,
a guardrail or prediction channel moved after Holm). Without a response context the verdict is
as before (fields absent); with one whose samples cannot be read, `responses` is empty and
`response_notes` says why.

### 4. One margins file

`engine/paddock_engine/config/margins.toml` is the single source of truth: `[metrics]` holds the
verdict's per-pass channels and every response metric (35 values; where an id is both,
`corner_time_s` and `v_min_kmh`, it is the same quantity in the same unit with one margin), and
`[parameters.<SETUP_ITEM>]` overrides apply to both kinds. `response/metrics.toml` no longer has
margins; `response/config.py` reads them from the margins file and refuses a metric without one
(test both ways). The values are ADR 0031's, unchanged.

### 5. Storage (migration 00013, append-only)

Each estimable response finding is a `result` row of role `response`, under its verdict with the
engine version (ADR 0030): the metric is `channel`, the frozen phase is the existing `phase`
column, the regime is `corner_class` (per corner, ADR 0031). `result` gains `response_finding`,
`expected` (direct or proxy) and `margin`, with a CHECK that a response row has all three and no
other row has any. `current_result` keeps meaning what the ledger shows and pools and leaves
response rows out (a pooling reader never mixes in per-phase cells); the view `current_response`
is the latest verdict's response rows. Down deletes response rows, drops the view and columns and
puts `current_result` back as 00012 made it (both replaced in place). The engine version is 0.3.0.

### 6. Contracts and web

`EvaluateRequest.response` (`VehicleResponseInput`), `EvaluateResponse.responses`
(`ResponseFinding`), `response_parameter`, `response_notes`, confounder kind `phase_interaction`;
`Experiment.responses` (`ExperimentResponse`). The experiment page has a "What the car did"
table, separate from the results and the verdict: each expected cell's channel, phase, regime,
direct or proxy, effect and interval, margin and finding.

### 7. Driver inputs where they happen

Experiment 20 (FH26, rear ARB +3, 8 blocks) flagged "driver adapted" on `brake_dist_m` +2.6 m
[+0.8, +4.4] while the response replay put the absolute braking point at -0.2 m [-0.3, 0.0],
neutral in every regime: the slowest point moved, not the braking (finding I6). Core now sends
the absolute `brake_point_m` and `throttle_pickup_point_m` (ADR 0031) as driver channels whenever
every compared pass carries them, and the apex-relative pair otherwise, so an experiment whose
features predate them keeps its two driver checks (`driverInputs`, `looks.go`). The AI reference
still reads the relative pair. Experiment 20 was not re-judged here.

### 8. Amendment (2026-11-19, engine 0.5.0): a driver input shifts only beyond its margin

Experiment 27, an A/A (identical arms, the AI at hint 0.97), flagged `driver_input_shifted` on the
braking point at -0.27 m [-0.43, -0.11]. The check tested each driver channel against zero (Holm
at 1 - level), so it worked at its nominal rate on identical setups (11 % for one channel,
simulated) but flagged any real wobble however small: 96.5 % of runs with a 0.3 m shift. Every
sweep step of experiment D and experiment B's -0.19 m were flagged the same way, so the flag
stopped saying anything.

Now a driver input has shifted only when Holm still rejects "no change" **and** the whole interval
lies beyond the channel's margin, the rule a response cell needs for an increase or decrease. The
margins are in `config/margins.toml` beside the others: `brake_point_m`, `throttle_pickup_point_m`
and the apex-relative `brake_dist_m`, `throttle_pickup_m` at 1 m (experiment B's prediction named
1 m for the braking point; the response metric `pickup_point_m` already used 1 m);
`steer_corrections_per_s` at 0.1 /s (as `steer_reversals_per_s`). A channel with no margin is
judged against zero as before. The driver-input result rows are classed against the margin too.

Simulated (`tests/test_driver_inputs.py`, 200 runs each, 8 blocks, interval about +-0.15 m, as in
experiment 27), braking-point shift and the share flagged, new rule (zero alone):

| Shift (m) | 0 | -0.3 | -0.5 | -1.0 | -1.5 | -2.0 |
| --- | --- | --- | --- | --- | --- | --- |
| Flagged | 0 % (11 %) | 0 % (96.5 %) | 0 % (100 %) | 5.5 % (100 %) | 100 % | 100 % |

Re-judged on the stored intervals of experiments 20 to 31 (the Holm step is unchanged, so this
is exact): 11 flagged before, 3 now. Gone: experiment 20, the A/A, every rear ARB sweep step's
braking and throttle flags, and four of the five grip-hint runs' braking point (-0.95 to -1.23 m:
the AI's earlier braking with less wing sits on the 1 m margin). Kept: experiment 22's braking
point (-1.26 m [-1.39, -1.12]) and the steering corrections of experiments 25 (+0.26 /s) and 30
(+0.21 /s). Stored verdicts are not re-judged (the ledger is append-only); the next evaluate of an
experiment applies the rule.

Rejected: a minimum-effect test (one-sided p of |shift| > margin) inside Holm. It flags at a
lower bar than "the whole 90 % interval beyond the margin" and would give driver inputs a rule
the response cells do not have. Known limit: 1 m is set by judgement, not learned; an aero change
moves the AI's braking by about that much, so for wing experiments the flag is on the boundary.

## Operating characteristics (offline, known truth)

`engine/tests/test_verdict_response.py`, `tests/verdict_data.py`; rates from 1000 simulated
experiments unless stated. Response cells: the 27 expected cells of the rear ARB row, 9 corners,
8 blocks of 3 laps, pass scatter 1 margin, block and lap shocks per cell.

| Identical setups, block shock | Any false directional finding, Holm (chosen) | Each cell at 90 % alone |
| --- | --- | --- |
| 0 | 0.5 % | 92 % |
| 0.3 margins | 1.8 % | 86 % |
| 0.6 margins | 3.8 % | 82 % |

A real effect on one cell (rear slip, exit, 300 experiments) is found (increase or likely
increase) 17 % of the time at 1 margin, 85 % at 2, 99 % at 3. Phase interactions on identical
setups (phase times only, 1000): 4.9 % flagged; braking +-2 ms against exit (one margin each, 300):
73 %, +-4 ms: 100 %. End to end on sampled synthetic passes (9 corners, `response_data.py`, 200
experiments, both families of context): 5.5 % context-dependent, 0 % with any moved response cell.
A rear-slip shift of 0.3 deg with no time effect gives a neutral or insufficient time finding,
rear slip mid "increase" with 0.3 inside its interval, yaw gain not moved and `switch_metric`.

## Replay: experiment 15 (rear wing -1, `WING_1 + WING_9`, FH26, Hungaroring)

Offline and read-only: the evaluate passes from `engine/tests/fixtures/real_experiments.csv.gz`
(ADR 0029's export), the raw samples and pass timing of session 105 from the spool cache of ADR
0031's replay; 336 push passes, judged at look 1 with no extension left, `parameter = WING_1`,
role `wing`, axle R. 0.5 s.

Time verdict: **context-dependent**, corner time -0.01 [-6.65, +6.63] ms (4 degrees of freedom);
slow +8.7 [+1.3, +16.2] ms, medium -2.3 [-20.9, +16.3], fast -9.1 [-15.1, -3.0]; interactions
fast vs slow, and braking vs entry, exit and mid, entry vs mid (the braking phase got longer).
Next: investigate context. Decision: inconclusive (as the ledger has it). ADR 0029's replay found
"fast vs medium" with the per-pass classes; the per-corner regimes put corners elsewhere.

What the car did (16 expected cells of `rear_wing`, Holm at 5 %; ms, deg, km/h; 90 % intervals):

| Metric | Phase | Regime | Kind | Effect [interval] | p | Finding |
| --- | --- | --- | --- | --- | --- | --- |
| rear slip (deg) | mid | all | direct | +0.108 [+0.068, +0.148] | 0.0045 | increase |
| rear slip | mid | fast | direct | +0.050 [+0.019, +0.080] | 0.025 | insufficient (excludes 0, not after Holm) |
| rear slip | exit | all | direct | +0.169 [+0.095, +0.243] | 0.0083 | increase |
| rear slip | exit | fast | direct | +0.121 [+0.104, +0.138] | 0.00011 | increase |
| rear slip | traction | all | direct | +0.086 [+0.071, +0.101] | 0.00028 | increase |
| rear slip | traction | fast | direct | not estimable | | insufficient (2 corners) |
| yaw gain | mid | all | direct | +0.020 [+0.013, +0.027] | 0.0032 | increase |
| yaw gain | mid | fast | direct | +0.022 [+0.016, +0.027] | 0.0011 | increase |
| yaw gain | exit | all | direct | +0.019 [+0.013, +0.024] | 0.0021 | increase |
| yaw gain | exit | fast | direct | +0.023 [+0.018, +0.027] | 0.00051 | increase |
| top-speed proxy (km/h) | traction | all | direct | +0.17 [-0.10, +0.43] | 0.25 | insufficient |
| top-speed proxy | traction | fast | direct | +0.98 [+0.67, +1.28] | 0.0024 | increase |
| slip balance F-R (deg) | mid | all | proxy | -0.173 [-0.233, -0.114] | 0.0035 | decrease |
| slip balance F-R | mid | fast | proxy | -0.175 [-0.261, -0.089] | 0.012 | decrease |
| slip balance F-R | exit | all | proxy | -0.174 [-0.283, -0.065] | 0.027 | insufficient (excludes 0, not after Holm) |
| slip balance F-R | exit | fast | proxy | -0.120 [-0.131, -0.110] | 0.00002 | decrease |

Ten of sixteen cells moved beyond their margins after Holm, all in the direction less rear wing
predicts (more rear slip and rotation, the balance towards the rear, more speed after fast
corners); none moved against it. The time stayed flat overall. With block-level intervals the
response intervals are wider than ADR 0031's (which inherited C2) and fewer cells pass: ADR 0031
had rear slip mid fast "likely up", here it does not survive Holm.

## Alternatives considered

- **Judge every cell with Holm** (344): the expected cells would lose most of their power to
  cells nobody predicted; the parameter row is the pre-registered hypothesis.
- **A second engine call from core** (`/response` after `/evaluate`): two round trips, two
  sources of regimes, and the recommendation could not use the response.
- **Store every cell**: hundreds of rows per verdict that decide nothing; `/response` gives them.
- **Response rows in `current_result`**: the sensitivity reader pools every channel of it per
  experiment and would mix per-phase cells into channel effects.
- **Phase interactions on every primary channel**: only corner time splits into phase times that
  add up; another channel's phase values are not its parts.

## Consequences

- An evaluation reads the session's Parquet parts: about 0.5 s on 336 passes after loading.
- Stored verdicts are not re-judged; they show no response table. Verdicts with a response
  context but unreadable samples say so in `response_notes`.
- The rear wing row now steers recommendations: experiment 15 would be "switch metric" were it not
  context-dependent first.

## Known limits

- The phase test is calibrated (4.9 %) on simulated phase times; on the real FH26 its many
  braking-phase hits may reflect the AI braking earlier, a real effect, but phase times are
  correlated within a pass and only the simulation checked that.
- The end-to-end context rate on sampled synthetic passes is 5.5 % (200 experiments; nominal 5).
- Margins are still ADR 0031's judgement from experiment 15's scatter.
- Not-estimable expected cells are in the verdict reply but not stored (no number to store).
- **Unverified in game.** Nothing here ran in Assetto Corsa or against the live database; the core
  path is tested against PostgreSQL with a fake engine and the engine on synthetic and recorded
  passes offline.
