# ADR 0019: The calibrated AI is the reference driver for hotlapping

Status: accepted (2026-11-16)

## Context

The owner's goal for a hotlap session is to know, corner by corner, whether to practise the
corner or to change the setup. A lap time alone cannot say which. The diagnosis
([ADR 0010](0010-diagnosis-and-explain.md)) weighs "driver input" as an alternative explanation
from the driver's own passes, but it has nothing to compare those passes with.

The AI test driver ([ADR 0014](0014-ghost-driver.md), [ADR 0016](0016-ai-calibration.md)) now
drives the player's car on a given setup, on real grip, very repeatably: within one session its
corner times on one setup scatter by 1 to 10 ms (session 57, 488 GT3, COTA), where a human's
scatter by 20 to 50 ms. Its passes are stored like a human's, as sessions with source `ai`,
each pass tagged with the agent's setup fingerprint (`corner_pass.setup_hash`).

## Decision

**The AI on the same car, track, layout and setup is the reference.** Core compares a session's
valid passes with the AI's, corner by corner, and nothing else. No new table and no migration:
everything is read from `session`, `corner_pass` and `setup_version`.

- **Which passes.** Valid passes only, never the out-lap of an experiment block (`block_lap`
  0). The driver's passes on one of the session's setups: the one asked for, else the setup of
  its latest pass. The AI's passes on that setup, from **one AI session only: the newest one
  with at least three passes of the corner on that setup**. The AI's calibration (the share of
  its tyre hint it drives with) is not stored per pass, so two AI sessions can be two different
  drivers; the newest is the AI as it drives today.
- **Fallback.** With fewer than three such AI passes, the AI passes on the setup that differs
  from the session's in the fewest items (core's setup versions; ties: more passes, then the
  lower fingerprint), said per corner with the number of items that differ. Without either:
  no reference.
- **Numbers.** Per corner: the driver's best and median zone time and pass count; the AI's
  median, best (with its session and pass, for the telemetry overlay), count, and spread as
  1.4826 times the median absolute deviation, so that one lap where the AI went off does not
  widen it.
- **The class rule.** `margin = max(AI spread, 10 ms)`. Driver's median minus the AI's median
  greater than the margin: **driver** (practise this corner). Anything else, faster included:
  **car** (the time here is what the car and setup allow, as far as the AI shows). Too few AI
  passes: **no reference**. The 10 ms floor exists because a gap of a few milliseconds is not a
  technique to practise, even when the AI repeats itself more tightly than that.
- **What differs.** Nine technique metrics of the engine's features, each with the smallest
  difference worth naming: brake point (5 m), peak brake (0.05), entry speed (2 km/h),
  trail-braking share (0.1), coasting (0.1 s), minimum speed (2 km/h), steering corrections
  (0.5 per s), throttle pickup (5 m), time to full throttle (0.2 s). The medians of driver and
  AI are compared; up to three metrics beyond their step are listed, largest in steps first,
  with both counts.
- **Routes.** `GET /sessions/{id}/reference` for the dashboard; `GET /reference?car=&track=&
  layout=&setup_hash=` for the game, the AI's median, spread and margin per corner on exactly
  that setup (newest corner map of the layout), nothing else.
- **In game.** A second, small window ("Paddock Hotlap") shows the last corner pass for six
  seconds: its time, the gap to the driver's best on this setup since the app started, the gap
  to the AI (red beyond the margin), and what the pass's own samples show with the engine's
  thresholds: front lock, rear lock, wheelspin, off track. A toggle shows the corners table
  (best, AI, delta). It asks core only while it is drawn, one request at a time, when the setup
  changes and otherwise every 30 s. It draws nothing while the AI drives.
- **Web.** An "Against the AI" view of a human session: the corners by time lost, with the
  class and what differs; in the telemetry, the AI's best pass of the corner on the same setup
  can be overlaid on the driver's.

## Limits

- **The AI drives below the limit.** Calibrated, it drives with 88.5 % of the tyre hint
  (2:15.8 against the owner's 2:12.65 on COTA). "You are faster than the AI" is expected in
  many corners and says nothing about the driver; it is classed car. The rule can only find
  corners where the driver is slower than a cautious, repeatable driver, which are the ones
  worth practising first.
- **The AI's line is not the driver's.** A different line can be faster or slower for reasons
  neither technique nor setup explains; a "driver" corner is a place to look, not a proof.
- **The AI is consistent, not optimal.** Where it copes badly with a setup, its median is slow
  and the corner reads car; that is information about the setup with this driver, not a limit.
- **Same layout only.** The AI drives on the `<layout>_paddock` copy
  (`tools/prepare_test_layout.py`), which has extended physics on. A human session on the
  original layout is a different track row and gets no reference; the owner drives the copy to
  be compared. Pairing the two is not done because extended physics may change the surface.
  The Drive panel, the "Against the AI" tab and the hotlap window say this in plain words
  ([ADR 0021](0021-human-keeps-the-car.md)).
- **The hotlap best is in memory** from the moment the app loads; it is not core's best.
- **Understeer and oversteer are not named in game.** Telling them from the driver's input needs
  the engine's front saturation and phase statistics; the display says only what one pass's
  samples show directly.

## Alternatives considered

- **The driver's own best as the reference** (v0). It says where the driver was inconsistent,
  not whether the car could go faster there.
- **Pool every AI pass on the setup.** More passes, but sessions calibrated at different grip
  mix two drivers; the median of the pool belongs to neither.
- **Sample standard deviation as the margin.** In session 57 one slow AI pass of T2 (6.521 s
  against about 6.40 s) makes it 38 ms over 9 passes; the robust spread of the same passes is
  2.4 ms.
- **An LLM verdict per corner.** Not allowed: the class is a fact and comes from the rule above.

## Not verified

- **Unverified in game:** the hotlap window has run only against the mocked CSP API; its look,
  the fade, and the HTTP request from inside Assetto Corsa were not seen.
- The class rule has not been checked against a human session on the `_paddock` layout: the
  stored human sessions of the 488 GT3 are on other layouts. It was run on real AI data only
  (AI session 53 against AI session 57, where it classes corners both ways).
- The feature steps are a judgment, not measured against the noise of each metric.
