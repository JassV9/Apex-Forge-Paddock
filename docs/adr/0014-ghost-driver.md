# ADR 0014: Ghost Driver level 2 — AC's own AI drives the experiments, on a copied layout

Status: accepted (2026-11-15). Seen working in Assetto Corsa; results in
[docs/runbooks/ghost-driver.md](../runbooks/ghost-driver.md).

## Context

An experiment needs laps on two setups, interleaved A-B-A-B. A human's corner times scatter by
about 28 ms ([ADR 0008](0008-effect-model.md)), so a plan that can see a one-click effect
needs many laps, and a tired or adapting driver adds drift the model has to fight. The design
doc ([design.md section D](../design.md)) names three ways for a machine to drive. Level 2,
handing the player's car to AC's own AI, was rated "medium: works per BotMate, needs physics
access". Phase 6 is the research track that tries it.

CSP's Lua API has what level 2 needs on the player's car: `physics.setCarAutopilot`,
`physics.teleportCarTo(0, ac.SpawnSet.Pits)`, `physics.resetCarState`, `physics.setCarFuel`,
`physics.setAILapsToComplete` and `physics.setAIPitStopRequest`. They work only when the track
grants Lua apps physics access: `[_SCRIPTING_PHYSICS] ALLOW_APPS=1` with extended physics
(`WAV_PITCH=extended-0` on `SURFACE_0`) in the layout's `surfaces.ini`. No installed track does.

## Decision

**A copied layout, never an edited one.** `tools/prepare_test_layout.py` copies a layout to
`<layout>_paddock` (its folder, models ini, VAO patch and UI folder), turns on extended physics
and app physics access in the copy's `surfaces.ini` only, renames it "(Paddock test)" and
leaves a marker file. `--remove` deletes only what has the marker. The original layout, its
files and its online checksum are untouched.

**The agent decides, the run plan asks.** Core stores who drives an experiment (`driver`:
`human` or `ai`, migration 00006) and writes it into the run plan. The agent's state machine
(`agent/src/core/ghost.lua`, pure Lua) runs each block: teleport to the pit box, repair, set
the fuel the first block started with, load the block's setup with the same read-back as a
human load, then hand the car to the AI and keep it from making its own pit stops. The run
plan counts the laps and tags the passes exactly as for a human. A car that stands still for
20 s, on track or parked by the AI, or that takes more than 5 units of damage, is sent back to
retry its block; after three retries the run stops and says why. The driver can stop it at any
moment from the window.

**AI laps are their own sessions.** When the AI takes the car the agent closes the spool
session and opens one with `source: "ai"`, and back again when it hands the car over. A human's
laps and the AI's never mix, in the ledger or in the noise model.

**A free run without a plan.** The AI can lap the car on its current setup until stopped (a
window button, or `outbox/agent_command.json` with a timestamp that expires after 60 s). Those
passes are the AI's baseline on the car: the session that experiments are proposed from.

**Run plans per car.** Core writes `outbox/runplans/<car>.json` and allows one running
experiment per car instead of one in the world, so an AI experiment on one car does not have
to abort a human's experiment waiting on another. The agent keeps its position per experiment
(`paddock/state/e<id>.json`, reading the older `agent_state.json` once). Core re-publishes
running plans at startup.

## What the AI measures, and what it does not

The AI is a fixed, heuristic policy. It is far more repeatable than a human, which is the point:
a plan detects much smaller effects with the same laps. But its lap time responds to a setup the
way the AI copes with it, not the way the car's potential changes, and it cannot adapt the way a
driver does. Effects measured with the AI are the car's response under one consistent driver.
They are a strong prior for a human's setup, never a substitute for it, and the ledger keeps
them apart by session source.

## Alternatives considered

- **Editing the original layout's surfaces.ini.** One line, but it changes a third-party file,
  the layout's checksum for online play, and the physics of every session on it.
- **CSP Custom AI over memory-mapped files (level 3).** The right tool for proving-ground
  manoeuvres and own control laws, and still the plan for those; it needs a controller,
  a vehicle model and its own safety envelope. Level 2 answers "same driver, two setups" today.
- **A virtual joystick (vJoy) driven by an external process.** Works for research, but needs
  the game's controls remapped to the virtual device and gives nothing level 2 does not.

## Consequences

- The AI drives only on a prepared layout copy, offline. On any other track the agent says so
  and the human drives.
- Extended physics may change track behaviour slightly against the original layout; effects
  are compared within the copy, never across.
- The car is repaired and refuelled between blocks, which a human's stint is not.
- Passes driven before a damage retry stay tagged with their block; the retry adds laps.

## Not verified

- Whether AI-measured effects predict a human's on the same car (needs paired experiments).
- AI behaviour on every car: it depends on the car's AI data and the layout's AI line.
- The setup is loaded with the car parked in its box without the setup screen open; this
  worked on the Ferrari 488 GT3, other cars are untested.
