# ADR 0016: The AI drives on the car's own grip, and learns speed caps in free laps only

Status: accepted (2026-11-15).

## Context

The first AI-driven runs ([ADR 0014](0014-ghost-driver.md)) showed two things.

**The AI had more grip than the car.** CSP documents that AI cars get 120 % "extra AI grip"
by default. Over 16 corners of COTA in the Ferrari 488 GT3, the AI's 95th-percentile lateral
acceleration was a median 1.20 times the owner's in the same car (session 26 against session
21). An effect measured on that car is an effect on a different car.

**The AI does not learn.** It is a fixed set of rules. On a setup it copes with badly it makes
the same mistake every lap: on the softer front bar of experiment 2 it lost the car at turns
19 and 20 on consecutive laps, was put back on track by AC, and had to be sent back to the pits.
With real grip, and on fast cars, this gets worse.

## Decision

**Real grip.** While the AI drives, the agent sets `physics.setExtraAIGrip(0, 1)` every two
seconds with its other keep-out calls.

**Speed caps, learned in free laps.** `agent/src/core/aitune.lua` divides the lap into 200
sections. During a free run, a section where the AI went off (3 or more wheels outside), spun or
stopped (under 25 km/h on track for 2 s), was reset, or took damage is an incident. At the end
of the lap, that section and the 6 before it (its braking zone) get a top-speed cap of 93 % of
the speed the AI carried there, or an existing cap is lowered by 5 %. Caps only come down. Two
clean laps in a row mark the car calibrated. The game side applies the cap of the current
section with `physics.setAITopSpeed` and lifts every cap when the AI hands the car back.

**Frozen during experiments.** In an experiment's blocks the caps are applied but never
changed, and incidents are only counted. The driver is the same in every block; a setup the AI
copes with worse shows as incidents and slower corners, which is a result, not something to
tune away.

**Kept per car and layout** in `Documents/Assetto Corsa/paddock/ai_tune/<car>__<track>__<layout>.json`.

## Alternatives considered

- **Leave the 120 % grip.** The laps are smoother, and every measured effect is for a car that
  does not exist.
- **Tune caution, brake hint or lookahead.** CSP exposes them, but their effect on a lap is
  indirect and car-specific; a speed cap per section is a quantity a human can read and check.
- **Keep learning during experiments.** The improvement from learning would be credited to the
  setup driven at the time: the exact confound the experiment design exists to remove.
- **Our own controller (Ghost Driver level 3).** The right tool for limit laps in formula cars;
  a research project with its own gates in the design doc.

## Consequences

- With real grip the AI is slower than before and may need several free laps to calibrate.
- Caps learned on one setup are applied on another; a setup that would allow more speed in a
  capped section cannot show it there. The caps are a floor of safety, not an optimum.
- Experiment 2 ran before this change, at 120 % grip; its result is labelled as such.

## Not verified

- How many laps calibration takes, and whether caps alone keep a formula car on track.
- That 1.20 is the grip ratio and not partly a line or driving difference; it matches the
  documented default.
