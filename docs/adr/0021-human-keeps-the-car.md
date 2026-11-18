# ADR 0021: A person at the wheel keeps the car

Status: accepted (2026-11-17). **Unverified in game**: everything here was tested against the
mocked CSP API, Vitest and PostgreSQL; none of it was run in Assetto Corsa.

## Context

The owner's flow is "pick a car and track, Start driving, hotlap with the wheel, compare with
the AI, Build a base setup, small changes, review the session". Until now the agent treated a run
plan whose driver is `ai` as an order: the moment the plan is on disk and the track gives
physics access, it teleports the car to its pit box, repairs and refuels it, loads the block's
setup through the setup spinners and hands the car to AC's autopilot
([ADR 0014](0014-ghost-driver.md)). That is right for an unattended training run, where the game
is already open. It is wrong for a person who starts the game from the Drive panel while an AI
experiment or training program is still open on that car: the plan is read at start-up, so the
AI takes the car out of the person's hands within seconds. Seen in the agent tests, not yet in
the game: the first thing a freshly booted agent did with an AI plan on disk was to drive.

A second hole: a plan counts laps and tags passes by the car's setup, whoever drives. A person's
laps on the setup of an AI experiment's block joined the block (`corner_pass.block_id`), putting
a driver with 20 to 50 ms of scatter into a measurement the AI makes at 3 ms.

What was already right: the agent never writes a setup file (setups change only through the
setup screen's spinners, on a click or by the AI); core writes only its own outbox, never the
game's setups folder; the launcher never sends `ai_laps`, so a launch is a human run.

## Decision

**A game that starts with an AI experiment open is the driver's.** When `agent.init` has read
the car's run plan and it is for the AI (`driver: "ai"`, a block left to drive), the agent calls
`ghost.hold`. A held AI does not take the car for that plan, for the next experiment of a
training program, or for any later plan, until the person hands it over: **Let the AI drive this
plan**, **Calibrate the AI** or **Let the AI drive laps** in the window, or the `ai_laps` /
`calibrate` command. The hold is sticky on purpose: a training program starts its next
experiment as soon as the person aborts the last one, and that must not take the car mid-lap.

Not held: a plan that arrives while the game runs (an experiment started from the window or the
web is an explicit act, and unattended training depends on it), a plan for a human, and a car
that is already under AI control at start-up (the agent was reloaded mid-run; that run goes on).

**An experiment for the AI counts only the AI's laps.** While the AI does not have the car
(held, stopped by the person, or failed) no lap is counted for the plan and no pass is tagged
with its blocks. This also covers a person who presses **Stop the AI driver** and drives on.

**The window says so.** The Run tab reads "Experiment N is for the AI to drive. Your laps are
not counted in it", offers no Load button for the AI's blocks and keeps **Let the AI drive this
plan**; the AI driver section carries the same sentence, and `agent_status.json` has
`ghost_held`. With no plan it says that no experiment is waiting and every corner is recorded.

**The web says so before the game starts.** The Drive panel reads core's experiments and
training programs and, for the picked car, names an open AI experiment, an open experiment the
person drives and a training program, saying what happens and how to hand over or drop it. It
works without core. It also says "You drive" next to Start driving.

**The layout says what it costs.** The AI drives only the `<layout>_paddock` copy, and the
reference is by track row (`track.ac_id, layout, corner_map_version`), so a person on the
original layout is recorded but never compared. The Drive panel says so for an original layout
("recorded, but not compared with the AI"), offers **Use the test copy** when there is one,
the "Against the AI" tab states the same with the session's layout when the AI has no passes
there, and the hotlap window's footer says so instead of "the AI has not driven this setup".
*Amended by [ADR 0027](0027-layout-variants.md):* the web app now compares a session on the
original layout with the AI on its test copy when their corner maps match, and the Drive panel
and "Against the AI" say "compared in the web app, never shown in the game"; the game is
unchanged.

**Base setup defaults to the person who owns the session** (`I drive` in a session that is not
the AI's) and says what to do next after building: it is only proposed; Plan and Start, then the
game's pits and Load for a person, or **Let the AI drive this plan** for the AI.

## Consequences

- Starting the game with an AI experiment open leaves the car to the person and records their
  laps as an ordinary human session on the same track row as the AI's, so the reference exists
  when the layout is the test copy.
- A restarted unattended run (game closed and reopened) needs one click, or a command file, to
  resume. Someone is at the keyboard for a game start anyway.
- The agent must be reinstalled (`tools/install_agent.py`) for any of this to apply in game;
  until then the old agent still takes the car.

## Known limits

- Core does not know whether the game is open or who is driving; the web can only say what is
  open on the car. Matching is by the car's display name (the launcher's `ui_car.json` name
  against the name the agent reports): a name that differs shows no notice.
- A plan for the AI that was aborted keeps its "put the baseline back" block in the outbox; held,
  it stays pending until the AI is asked to drive, which then loads the baseline and ends. Nothing
  was loaded onto the car by a held agent, so nothing needs undoing for a person.
- A person driving a plan for a human on a different track than the experiment's still has laps
  tagged when the setup matches; plans do not carry a track.
- The launcher reports "unchanged" after its three-minute wait even if the game never opened.

## Alternatives rejected

- **Hold only the plan present at start-up.** The next experiment of a program would then take
  the car as soon as the person aborted the one in hand.
- **A "human session" flag in the launcher's preset or a file the agent reads.** The launcher
  is not the only way into the game (Content Manager is); the agent's own start-up state is.
- **Refuse to start an AI experiment or program from core while a person drives.** Core cannot
  see the game.
- **Count the person's laps as the block's and let core exclude them.** The block's identity is
  decided at the source; core would have to guess it back from the session's source.
