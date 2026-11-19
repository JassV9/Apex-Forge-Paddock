# ADR 0028: Run context and AI provenance

Status: accepted (2026-11-18). **Unverified in game**: everything here was tested against the
mocked CSP API, PostgreSQL and the end-to-end test; none of it was run in Assetto Corsa.
Implements findings C1, I8 (recording only), I9 and I10 (recording only) of the
[Ghost Driver reliability audit](../investigations/2026-11-18-ghost-driver-reliability.md).

## Context

The AI plans its corners with a share of the car's own tyre hint
([ADR 0016](0016-ai-calibration.md)). That share is learned in free laps and kept in one
`ai_tune/<car>__<track>__<layout>.json` file, which every calibration overwrites. No session,
pass or experiment recorded which share it was driven with: the four pooled rear ARB +1 runs of
the audit were driven at unknown hints, and sessions 80 to 88 pull 2.6 to 3.5 % more peak lateral g
than session 119 on the same car. An experiment at a chosen hint (Experiment C of the
investigation: the same setup change at 0.885, 0.93, 0.9409, 0.97 and 1.0) could not be run at
all: the only hint the AI drives with in a block is whatever was learned last, and free laps at a
test hint would lower it.

Two other holes: `set_fuel` at a block start is never checked (experiment 15 drove block 1 on 24 L
and blocks 2 to 8 on 48 L), and the AI's level, aggression, CSP's version and the agent's own
build were recorded nowhere (`agent_version` was the constant `0.1.0`).

## Decision

**The session line says what the session was driven under.** `spool_session.context` (optional):
`cars_count`, `csp_version` (`ac.getPatchVersion`), `csp_build` (`ac.getPatchVersionCode`) and
`ai`: the car's tyre hint (`ai.ini [TYRES] HINT`), the learned share, its aero and brake hints
when the car's `ai.ini` has them (`[PHYSICS_HINTS] AERO_HINT`, `[PEDALS] BRAKE_HINT`), and
`aiLevel` / `aiAggression` from `ac.StateCar`. An AI session adds the pinned share, the hint it
is handed (base x share) and the extra AI grip the agent sets (1). `agent_version` is
`0.2.0+<build>`: `tools/install_agent.py` writes `src/build.lua` with the first 12 hex digits of a
SHA-256 over every file it copies; a copy it did not install says `0.2.0+dev`.

**Every pass says what it was driven under.** `spool_pass.context`: the tyre hint the AI was given
(only while it drives), the largest damage zone and the reset counter when the pass ended, the
share of samples with ABS (`absInAction`), TC (`tractionControlInAction`) and DRS (`drsActive`)
acting, and the clock of its `t` column.

**New per-sample channels**, optional columns of `samples.columns` so older lines stay valid:
`nd_*` (`wheels[i].ndSlip`, past peak grip above 1), `w_*` (`wheels[i].angularSpeed`, rad/s,
for wheel speed differences), `st_*` (`wheels[i].suspensionTravel`), `vx`, `vz`
(`localVelocity`, true body slip), `x`, `z` (`position`, the line), `gear`, `rpm`. That is the
set the parameter-to-observable map (investigation section 4) needs, nothing more. `t` is CSP's
physics time (`car.timestamp`, ms) when the car has one at start-up, else the app clock as
before; a physics time that stands still for 0.5 s while the game runs hands over to the app
clock for good, and each pass names its clock.

Field names are from the CSP Lua SDK shipped with the installed CSP,
`extension/internal/lua-sdk/ac_apps/lib.lua` (read 2026-11-18): `ac.StateWheel` `angularSpeed`
(line 4834), `ndSlip` (4838), `suspensionTravel` (4845); `ac.StateCar` `maxFuel` (4886),
`timestamp` (4934, "time of last physics state record, in milliseconds"), `position` (4938),
`localVelocity` (4943), `gear` (4955), `rpm` (4958), `aiLevel` (4990), `aiAggression` (4991),
`resetCounter` (5062), `absInAction` (5100), `tractionControlInAction` (5101), `drsActive`
(5108); `ac.StateSim.carsCount` (5283); `ac.getPatchVersion` (3041), `ac.getPatchVersionCode`
(3045). The SDK gives no units for `ndSlip` and `suspensionTravel`; they are stored as CSP
reports them.

**A hint pinned per experiment.** `POST /experiments` takes `ai_hint_share` (0.5 to 1.5, only with
`driver: "ai"`); core stores it on `experiment.ai_hint_share` and writes it into every revision of
the run plan. While the plan's blocks run, the agent hands the AI that share of the car's hint
instead of the learned one, and with a pin it learns nothing, counts nothing and never writes the
`ai_tune` file. The hint counts as given only once `physics.setAITyresHint` returned; if the
hint given is not base x pinned share (the call was refused, or another value was set), the run
stops: "AI driver stopped: the AI's tyre hint is not set, the plan pins 1.0230 (0.9300 of the
car's 1.1000)." The game has no getter for the AI's hint, so this checks what the agent gave,
not what the AI uses.

**Free laps at a test hint.** The command `ai_laps_fixed` (with an optional `ai_hint_share`)
laps the car like `ai_laps` but never learns: at the given share, or at the learned one. A share
out of range is refused and nothing happens.

**Fuel read back.** One second after a block is handed to the AI the agent compares `car.fuel`
with the fuel it set; more than 0.5 L off stops the run ("the car has 24.0 L after refuelling for
block 1, the plan wants 30.0 L"). The fuel asked for is never more than `car.maxFuel`, so a
small tank does not fail every block.

*Amended 2026-11-19 (after experiment 20).* The setup's own `[FUEL] VALUE` can land after the
set, so an off read-back first sets the fuel again, up to three times a second apart, during the
out-lap; only then does the run stop ("... (set 4 times)"). And the fuel is one constant,
`BLOCK_FUEL_L` = 30 L (or a full tank if smaller), for every block of every AI experiment. It
was the car's fuel when the AI first took it, at least 30 L: experiment 20 ran all ten blocks on
30 L, but its block 9, after a relaunch, asked for 50 L. Arms inside one experiment were always
equal; experiments compared across sessions (the hint sweep) were not.

**Core stores it.** Migration 00011: `experiment.ai_hint_share`; `corner_pass.ai_hint`,
`damage_max`, `reset_counter`, `abs_share`, `tc_share`, `drs_share`, `clock`; and `run_context`,
one row per session with its session line's settings (`block_id` NULL) and one row per session
and block with the experiment's pinned share and the lowest and highest hint its passes in that
session were driven with, and how many. Ingest writes them in the chunk's transaction; a pass
read again is not counted twice. The new sample channels go into the Parquet parts as optional
columns, NULL for a pass recorded without them; the engine's DuckDB reader and core's Parquet
reader still read the old columns from a session whose parts differ (tested).

## Not done here

- **Holding** AI level, aggression and CSP's dynamic shifting thresholds fixed
  (`physics.setAILevel`, `setAIAggression`, `setAIShiftingThresholds`). Recorded only: whether
  pinning them changes how the AI drives a calibrated car must be seen in game first.
- **Deployment control** (I8): `mgukDelivery` / `mgukRecovery` are still not recorded or set; the
  pass carries the battery as [ADR 0024](0024-ers-state-per-pass.md) says.
- A training program does not pin a hint; its experiments drive on the learned share, which every
  pass and the block's `run_context` row now record.
- Core does not yet refuse a verdict on a block whose passes ran at another hint than the pinned
  one; `run_context` makes the check one query. The guard belongs with the verdict guards of the
  statistics work package.
- The experiment's API reply does not show `ai_hint_share`; the run plan and `run_context` do.

## Consequences

- A pass line is about 75 % longer: 18 more columns of ~6 bytes per sample (about 65 kB on a
  600-sample pass), encoded one column per frame as before, so a pass takes 42 frames to encode
  instead of 24; no extra file writes. The recorder copies 47 fields per frame instead of 26, into
  the same reused buffers.
- **Core must be rebuilt before the agent is installed**: an older core's embedded schema rejects
  the new `context` members and columns (`additionalProperties: false`), so it would drop every
  line of a new agent as `schema`.
- Every earlier session, pass and experiment has no run context; nothing is back-filled.

## Alternatives considered

- **A per-experiment copy of the ai_tune file.** Hides the pin in a file core does not see; the
  run plan is already the agent's instructions for an experiment.
- **Reading the hint back from the game.** CSP has a setter and no getter for the AI's hints.
- **Per-pass `run_context` rows only.** The per-pass columns are on `corner_pass`; the table holds
  what is per session and per block so a block's hint range is one row.

## How to test it in game

See [runbooks/ghost-driver.md](../runbooks/ghost-driver.md) "A pinned-hint experiment": install
the rebuilt core and agent, propose with `ai_hint_share`, and check `run_context`.
