# ADR 0020: The AI trains on a car by itself: a queue of experiments judged as they finish

Status: accepted (2026-11-16)

## Context

The owner's flow is "get in, hotlap, get a base setup, then small changes". A base setup is
only as good as what the car has measured ([ADR 0018](0018-base-setup.md)), and a human
measures slowly: about 14 ms of scatter per corner against the AI's 2.6 ms
([runbook](../runbooks/ghost-driver.md)). Experiment 3 showed that the calibrated AI can drive a
whole A-B-A-B experiment on its own. What was missing is the loop around it: judge it when the
laps are in, decide what to do with the verdict, start the next one, without anyone at the
keyboard, so a car can be trained overnight.

## Decision

**A training program is a queue on one car.** `POST /sessions/{id}/training {experiments: N}`
opens it from a session (normally the AI's calibrated free laps), diagnoses that session afresh
and starts the best-ranked experiment with the AI driving. Core's background loop
(`core/internal/training`, every 15 s) looks at the program's newest experiment:

| Newest experiment | What the program does |
| --- | --- |
| running, laps still missing | waits; its message says block and lap |
| running, every block driven | asks the engine for the verdict (`evaluate`); a lap counts once it is finished ([ADR 0025](0025-finished-laps-and-lap-noise.md)) |
| inconclusive, whole interval on the better side, not extended yet | extends it once |
| inconclusive otherwise | keeps the baseline (`revert`) |
| rejected | puts the baseline back (`revert`) |
| trade-off | waits for the driver: a trade-off is their call |
| accepted, reverted, aborted, discarded | starts the next experiment |

The next experiment is the best-ranked proposal of the session's diagnosis on an item the
program has not tested yet. The ranking itself already drops what this car measured as
negligible or worse ([ADR 0011](0011-sensitivity-and-ranking.md)) and is rebuilt from the
ledger on every call, so each verdict changes what comes next. The program ends at its target
or when nothing is left to test. Linked items are one setting: a step on any member tested the
group, and the group is proposed under its leader, never a lone member
([ADR 0026](0026-linked-setup-items.md)).

**The ledger says who decided.** A baseline the program keeps is recorded with the program's
reason and `decided_by: training` (migration 00009), never as the driver's call. The first
program in the game wrote "The driver kept the baseline." on experiment 3 before this was
fixed; the ledger is append-only, so that line stays.

**Only the ledger's own actions.** The program calls the experiment service's `create`, `plan`,
`start`, `evaluate`, `extend` and `revert`, so every rule holds as for a human: one experiment
on a car at a time, the champion as the baseline, the success test fixed before driving.
Nothing is skipped and failed experiments stay in the ledger. Starting a program takes over an
open AI experiment on the car as its first step and refuses when a human's experiment is on it.
One open program per car (a unique index).

**Calibration first, in the game.** "Calibrate the AI on this car" in the window (or the
`calibrate` command) drives free laps that stop by themselves when calibration has two fresh
clean laps, parking the car in its pit box. Training does not calibrate on its own: a program
started on an uncalibrated car will have its laps rejected by the agent's watchdogs.

## Alternatives considered

- **Auto-evaluate every AI experiment, program or not.** Simpler, but a driver who started one
  experiment from the window may want to look before it is judged; inside a program the owner
  asked for exactly that.
- **Extend every inconclusive experiment twice.** Each extension is about 20 minutes of AI laps;
  an effect whose interval still straddles zero after one extension is better left for a
  re-test than chased.
- **Re-test items already measured.** The ranking would often pick the same knob again (an
  inconclusive one stays ranked); a program explores instead. A human can still propose it.

## Consequences

- A car can be trained unattended: about 40 minutes per experiment on a 2:15 lap.
- Programs only run while core runs; a program whose experiment the agent cannot drive (game
  closed, car stuck three times) waits on that experiment until the driver aborts it. Amended by
  [ADR 0025](0025-finished-laps-and-lap-noise.md): when that happens on the last lap of the last
  block, the lap counts after 15 minutes without a pass and the program judges what was driven.

## Not verified

- A program driving more than one experiment in Assetto Corsa. The loop is tested against a
  fake ledger; experiment 3 was driven by the AI but judged by hand.
