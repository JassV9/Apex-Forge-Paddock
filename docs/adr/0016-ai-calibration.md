# ADR 0016: The AI drives on the car's own grip, and learns speed caps in free laps only

Status: accepted (2026-11-15); amended 2026-11-16, speed caps replaced by a learned tyre hint
(see the amendment at the end).

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

## Amendment (2026-11-16): the AI learns the grip it assumes, not speed caps

**What the game showed.** On real grip the 488 GT3 went off at the same two places every
lap: the esses (section 51) and turn 19 (section 184), all four wheels out. After one lap
sections 178 to 183 were capped at 171 down to 106 km/h, and the next lap the AI still arrived
at section 184 at 119 km/h and went off again. `setAITopSpeed` limits how fast the AI
accelerates; it does not make it brake earlier for a corner it misjudges, so the caps only
shrank lap after lap.

What the AI plans its corners with is its tyre hint (`ai.ini [TYRES] HINT`, 1.0 for this car):
the grip it believes the car has. With the hint at 0.90 the next lap had no wheel off the
track at all, turn 19 included.

**Decision.** `aitune.lua` (version 2) learns one number per car and layout: the share of the
car's own tyre hint the AI drives with. It starts at 1. A free lap where the AI went off,
spun, stopped, was reset or took damage lowers it by 3 %, down to 70 %; two clean laps in a
row mark the car calibrated. The game side applies it with `physics.setAITyresHint` while the
AI is on track and puts the car's own hint back when the AI hands the car over. Frozen during
experiments, kept in the same file; a version 1 file (speed caps) is dropped.

A run of incident sections counts once: after going off, the car crawls through the next
sections, and those are the result of the mistake, not new ones. The file records where the
last lap went wrong.

**Alternatives.** Per-section hints would slow the AI only where it goes wrong, but a hint
changed in a braking zone acts on the AI's lookahead in ways not yet measured. One number is
what the game showed to work; per section is the next step if a car needs it. Lowering
`setAIBrakeHint` (0.97 for this car) was not tried.

**Consequences.** The AI is slower everywhere, not only where it went wrong: on this car it
drives below the limit in corners it already handled. Effects that only appear at the very
limit of grip can be understated; the experiment's statistics still compare like with like.

*Amended by [ADR 0028](0028-run-context-and-ai-provenance.md) (2026-11-18):* an experiment can pin
the share for all its blocks (`ai_hint_share`); a pinned run neither learns nor writes the
calibration file, and every session and pass records the share and hint it was driven with.
