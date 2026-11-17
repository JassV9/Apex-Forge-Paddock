# ADR 0024: The hybrid battery is recorded on every pass and adjusted for like fuel

Status: accepted (2026-11-17). Adds a covariate to the effect model of
[ADR 0008](0008-effect-model.md), under the rule of [ADR 0022](0022-covariates-the-arm-can-be-told-from.md).

## Context

On the RSS Formula Hybrid 2026, an F1-style car with an MGU-K and a battery, AI experiment laps
swing by about +-30 ms per lap with no pattern. Fuel and tyre temperature start every block the
same, so they do not explain it. After ADR 0023 the noise shared by the passes of one lap is most
of what is left of the FH26 interval (the mean residual of a lap ranges from about -35 to
+31 ms). How much energy the car deploys depends on how much is in the battery, and the agent
recorded nothing about the battery. Whether the battery explains the swings cannot be tested
until it is recorded.

## The fields

CSP's `ac.StateCar` (CSP Lua SDK as shipped with the installed CSP,
`extension/internal/lua-sdk/ac_apps/lib.lua`, read 2026-11-17; the same names are in the public
dump [dabinn/ac-lua-sdk-api](https://github.com/dabinn/ac-lua-sdk-api), `ac_car_cphys/lib.lua`;
the official [acc-lua-sdk](https://github.com/ac-custom-shaders-patch/acc-lua-sdk) repository
does not contain the generated state structs):

| Field | Type | What it is |
| --- | --- | --- |
| `kersPresent` | boolean | The car has KERS/ERS. Physics-only (the player's car has physics). |
| `kersCharge` | number | Battery charge, 0..1. |
| `kersInput` | number | KERS/ERS deployment input, 0..1. |
| `kersCurrentKJ`, `kersMaxKJ`, `kersLoad`, `kersCharging`, `kersButtonPressed`, `mgukDelivery`, `mgukRecovery` | | Present, not recorded (see the alternatives and what is not fixed). |

The SDK gives names and types but no meanings for the `kers*` numbers. The meanings come from
AC's shared memory, which has fields of the same names (`kersCharge`, `kersInput` in the physics
page, as in [gro-ove/actools](https://github.com/gro-ove/actools)
`AcManager.Tools/SharedMemory/AcSharedPhysics.cs`): `kersCharge` is the battery's state of
charge, 0..1 (a [shared-memory reference](https://docs.google.com/document/d/1WzqMLkW2o_C0LGcvdMRelAV31ZIifux0CSHD9k6ddz0/export?format=pdf)
says so, and [Battery-Helper-AC](https://github.com/jcsilva7/Battery-Helper-AC) shows
`kersCharge * 100` as the battery percentage); `kersInput` is the KERS/ERS input, 0..1, which
this ADR takes as deployment. Neither meaning has been checked against the FH26 in the game.

## Decision

**The agent records three numbers per pass, only on a car with `kersPresent`.**
`carlink.read` copies `kersCharge` and `kersInput` into the frame sample every frame, and the
recorder keeps them in its column buffers with the other channels (`channels.summarised`): two
more numbers per frame into the same reused tables, no new allocation once the buffers have grown,
no extra file write. They are not written per frame. When the pass is encoded,
`spool_lines.lua` (pure) adds `"ers": {"charge_start", "charge_end", "deploy_mean"}`: the charge
at the first and the last sample of the zone and the mean input over its samples, 4 decimals. A
car without a battery gets no `ers` object at all, so its lines are what they were.

**Core stores them** as `corner_pass.ers_charge_start`, `ers_charge_end`, `ers_deploy_mean`
(migration 00010; NULL when absent; Down drops the columns).

**Only the charge at the start of the pass is a covariate.** `verdict.go` sends
`ers_charge_start` to the engine when the pass has it. The engine's existing rule decides: a
covariate enters only when every pass has it, it varies within corners, and no design column has
a variance inflation above 10. A car without a battery, a battery that never changes, or an
experiment in which some passes were recorded before the agent sent the charge: nothing is
adjusted, and the verdict is what it was.

Deployment and the end charge are recorded but not adjusted for. They happen during the pass and
are partly the setup's doing: a car that is slower out of the corner sits on full throttle for
less of the zone, and a setup that brakes later harvests differently. Adjusting for them would
take part of the setup's own effect out of the estimate. The charge at the start is fixed before
the corner, so it is the closest the battery comes to a condition like fuel.

**Simulated** (`engine/tests/simulation.py`, `simulate_battery_abab`): A-B-A-B, 3 laps per block,
12 corners, 12 ms pass scatter, a 10 ms effect, each lap starting on a random charge between 0.3
and 0.9, and every pass slower by 0.1 s x (0.6 - charge). Over seeds 0 to 9 the 90 % interval was
+-5.6 ms on average with the charge as a covariate and +-18.8 ms without it, and it held the true
effect in 10 of 10 against 9 of 10. When the charge follows the arm (the variant 0.3 fuller), it
is left out and named in the verdict's reason, as ADR 0022 says.

## How to test the hypothesis

Drive an AI experiment on the FH26 after installing the agent. Then, per corner, the
correlation of the start charge with the zone time over its push passes:

```sql
SELECT c.number, count(*), corr(p.ers_charge_start, p.duration_s)
FROM corner_pass p
JOIN corner c ON c.id = p.corner_id
JOIN run_block b ON b.id = p.block_id
WHERE b.experiment_id = $1 AND p.block_lap >= 1
GROUP BY c.number ORDER BY c.number;
```

and the verdict's interval against a replay of the same rows without `ers_charge_start`. If the
charge is kept and the interval narrows, the battery was part of the swing; if it does not, the
cause is elsewhere and this ADR stays as a hypothesis that failed.

## Alternatives considered

- **A per-frame spool column for charge and deployment.** Every pass of every car would carry
  two more columns (about 2 x 600 numbers), and the Parquet parts with them, for three numbers
  the model can use. Rejected; the per-frame values are kept in the recorder only.
- **Adjust for deployment as well.** Rejected, see above: deployment during the pass is downstream
  of the setup.
- **`kersCurrentKJ` (energy used this lap, in AC's shared memory).** A per-lap counter: its value
  at a corner depends on where the corner is in the lap, and the start charge already says how
  much is left. Not recorded.
- **Hold the battery equal at the start of every block.** The cleaner remedy, as for fuel
  (ADR 0022). The SDK above has no setter for the charge (`physics.setCarFuel` has no battery
  counterpart), so it is not possible here.

## Not fixed here

- **Delivery and recovery programs.** `mgukDelivery` and `mgukRecovery` are readable, and
  settable through `physics.setMGUKDelivery` / `physics.setMGUKRecovery`. If the AI or the car's
  own logic changes the program between laps, that is another lap-wide cause the agent neither
  records nor holds fixed. Recording the program per pass, or setting it at the start of every
  block, is the next step if the start charge does not explain the swings.

## Consequences

- A hybrid's pass line is about 70 bytes longer. Non-hybrid lines are unchanged byte for byte.
- Past verdicts are not re-judged, and passes recorded before this change have no battery state.
- The charge at the start of a corner is still partly the result of the earlier corners of the
  lap on the same setup. When that link is strong the variance inflation rule leaves the charge
  out; a weak link biases the estimate a little towards zero. The reason says which covariates
  were used.

## Not verified

- **Everything in the game.** That CSP reports `kersPresent`, `kersCharge` and `kersInput` for the
  FH26 (a CSP extended-physics car), that `kersInput` moves when the AI deploys rather than only
  for a player's button, and that the values stay within 0..1, are unverified in game.
- Whether the battery explains the FH26 lap swings: that is the hypothesis this makes testable.
