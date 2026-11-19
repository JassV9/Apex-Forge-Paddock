# Runbook: the AI test driver (Ghost Driver level 2)

Why it works this way: [ADR 0014](../adr/0014-ghost-driver.md). The in-game window that starts
it: [ADR 0015](../adr/0015-in-game-setup-desk.md).

AC's own AI drives your car through an experiment's blocks while you watch, or while you are
away. It needs a layout that gives Lua apps physics access; the tool below makes a copy of a
layout with that access and leaves the original alone.

## Once per track

```bash
python tools/prepare_test_layout.py fn_cota layout_f1_2026
```

This adds the layout "COTA F1 2026 (Paddock test)" (`layout_f1_2026_paddock`). To remove it:

```bash
python tools/prepare_test_layout.py fn_cota layout_f1_2026 --remove
```

Pass `--ac-root` if the game is not in the default Steam folder.

## A run

1. Start the stack with core pointed at the game's folders (as in
   [agent-in-game.md](agent-in-game.md)) and install the agent (`python tools/install_agent.py`).
2. In Content Manager pick the car, the "(Paddock test)" layout, **Practice**, start from
   **Pits**, and drive (or let the AI drive) out.
3. Open the Paddock window from the apps bar. In **Run**, press **Let the AI drive laps**: the
   AI laps the car on its current setup and every pass goes into a session marked AI. Two or
   three laps are enough for a baseline.
4. In **Setups**, press **Diagnose this session**, then **AI drives it** on a proposal. Core
   plans and starts the experiment; the agent stops the free run, takes the car to its pit
   box, repairs and refuels it, loads the first block's setup and hands it back to the AI.
5. It drives every block on its own. The window counts the laps; the dashboard's experiment
   page fills block by block.
6. When all blocks are driven, press **Judge** in **Setups** (or Evaluate in the dashboard).

**Stop the AI driver** in the window gives you the car back at any moment.

**Starting the game yourself with an AI experiment open** leaves the car to you: the agent holds
the AI back ([ADR 0021](../adr/0021-human-keeps-the-car.md)), the Run tab says "Experiment N is
for the AI to drive", your laps are not counted in it, and **Let the AI drive this plan** hands
the car over. The same hold applies after **Stop the AI driver**: laps you drive are never
counted in an experiment for the AI. A plan that arrives while the game is already open, from
the window or the web, still starts the AI at once.

Without the window (for scripted runs), the same steps go through core's API, and a free run
is started with a file the agent reads every two seconds:

```bash
echo "{\"id\": \"$(date +%s)\", \"at\": $(date +%s), \"command\": \"ai_laps\"}" \
  > "$HOME/Documents/Assetto Corsa/paddock/outbox/agent_command.json"
```

`"command": "stop"` stops it. A command more than 60 seconds from the game's clock is ignored.
`at` is Unix time in UTC: in Windows PowerShell 5.1 use
`[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()`, not `Get-Date -UFormat %s` (local time, so the
command is silently ignored).

## Calibration

Before an experiment, press **Calibrate the AI on this car** in the window's Run tab (or send
`"command": "calibrate"`): the AI drives free laps until the window (or `agent_status.json`)
says **calibrated**, two clean laps in a row, then parks the car in its pit box. Every lap where it went off, spun, stopped,
was reset or took damage lowers the grip it assumes by 3 %
([ADR 0016](../adr/0016-ai-calibration.md)). The result is kept per car and layout in
`Documents/Assetto Corsa/paddock/ai_tune/`; `last_incidents` there says in which of the 200
sections of the lap it last went wrong. Delete the file to start over.

## Training: the AI tests the car by itself

Once calibrated, **Setups › Train the AI on this car › Start training** (or
`POST /sessions/{id}/training {"experiments": 4}` with the calibrated AI session) runs the
session's best-ranked experiments one after another with the AI driving. Core judges each one
when every block is driven, extends an inconclusive one when the engine recommends the next
look (8, then 10 and 12 blocks), puts the baseline back after a loss, and starts the next
untested item ([ADR 0020](../adr/0020-ai-training-program.md),
[ADR 0029](../adr/0029-block-level-inference-and-finding-classes.md)). A
trade-off waits for you. Leave the game running; about 40 minutes per experiment on a 2:15 lap.
`GET /training` and the window show where it is; **Stop training** ends it.

## A pinned-hint experiment (Experiment C of the reliability investigation)

Why: [ADR 0028](../adr/0028-run-context-and-ai-provenance.md). An experiment can fix the share of
the car's AI tyre hint the AI drives with in all its blocks, instead of the share calibration
learned. One A/B per hint, never two hints inside one A/B.

1. **Rebuild core first, then install the agent** (`python tools/install_agent.py`). An older
   core rejects the new agent's lines (`schema` rejects in its log). The session line's
   `agent_version` then reads `0.2.0+<12 hex digits>`; `0.2.0+dev` means the copy was not
   installed by the tool.
2. Calibrate as usual (above), so the learned share stays the reference.
3. Optionally look at the car at a test hint without changing the calibration: free laps that
   learn nothing,

   ```bash
   echo "{\"id\": \"$(date +%s)\", \"at\": $(date +%s), \"command\": \"ai_laps_fixed\", \"ai_hint_share\": 0.97}" \
     > "$HOME/Documents/Assetto Corsa/paddock/outbox/agent_command.json"
   ```

   (without `ai_hint_share` it drives at the learned share). A share outside 0.5 to 1.5 is
   ignored.
4. Propose each A/B with its hint, the AI driving, from the calibrated AI session:

   ```bash
   curl -s -X POST localhost:8080/experiments -H 'Content-Type: application/json' -d '{
     "session_id": <AI session>, "parameter": "WING_1", "clicks": -1, "driver": "ai",
     "ai_hint_share": 0.93, "statement": "Rear wing -1 at hint 0.93", "predictions": []}'
   ```

   then Plan and Start as for any experiment. Core refuses a pin on an experiment the human
   drives (422) and one outside 0.5 to 1.5; nothing is stored. The order of the investigation
   is 0.93, 0.885, 1.0, 0.97, 0.9409.
5. While it runs, `agent_status.json` shows `ai_hint` (what the AI was given) and
   `ai_hint_pinned`. The run stops by itself, before a lap counts, when the hint given is not
   base x pinned share, or when the fuel read back after the refuel is more than 0.5 L off.
6. Afterwards check what was driven:

   ```sql
   SELECT b."order", c.ai_pinned_share, c.ai_hint_min, c.ai_hint_max, c.passes
   FROM run_context c JOIN run_block b ON b.id = c.block_id
   WHERE b.experiment_id = $1 ORDER BY b."order";
   ```

   Every block should show the same min and max, equal to the session row's
   `ai_tyre_hint_base x ai_pinned_share`; the `ai_tune` file is unchanged by the whole run.

Not seen in game yet: whether CSP takes `setAITyresHint` above the learned share without the AI
leaving the track (that is what the experiment measures), and whether `car.timestamp` advances
for the player's car (if not, passes say `"clock": "app"`).

## A sensitivity sweep (Experiment D of the reliability investigation)

Why: [ADR 0032](../adr/0032-sensitivity-discovery-sweeps.md). A sweep learns how the car responds
to one setting; it never changes the setup. Experiment D: rear ARB -1, +1, +2, +3 on the FH26 at
the Hungaroring (`_paddock` test layout), pinned hint.

1. Rebuild core and the engine (migration 00015 and `POST /discovery/curve` come with them), and
   install the agent as for a pinned-hint experiment (above).
2. Calibrate the AI on the car (above) and leave the car on the setup you want as the baseline.
   The baseline must be a setup the agent read back from the car: drive at least one lap on it,
   or core refuses the sweep (409, "never read back from the car").
3. Make sure nothing else is on the car: no open experiment (finish or abort it), no training
   program (stop it). Core refuses otherwise (409).
4. Start it from the calibrated AI session, in the dashboard (**Experiments › Discover**: setting
   Rear ARB, steps `-1, 1, 2, 3`, pinned hint share e.g. `0.93`) or:

   ```bash
   curl -s -X POST localhost:8080/sessions/<AI session>/sweeps -H 'Content-Type: application/json' \
     -d '{"parameter": "ARB_REAR", "steps": [-1, 1, 2, 3], "ai_hint_share": 0.93}'
   ```

   The reply's `planned` and `skipped` say what the item's range allows: with the bar at 3 of 0..5,
   +3 is clipped to +2 and skipped as a repeat. A hint share is required (422 without one).
5. Leave the game running. Each step is an 8-block A-B-A-B experiment (about 40 minutes on a 1:15
   lap; up to two extensions when the engine recommends the next look), then the baseline goes back
   on the car and the next step starts. `GET /sweeps` and the card show the step, its block and
   lap, and each finished step's finding.
6. The sweep stops by itself, and says why in `message` (state `failed`), when a step is invalid,
   aborted, driven at another hint than base x pinned share (`run_context`), when nothing has come
   from the AI for 30 minutes, or when the baseline moved (someone accepted an experiment on the
   car). Abort the step's experiment if it is still open; the steps before it stand. Start a new
   sweep for the rest. **Stop sweep** ends it by hand and leaves the open experiment to you.
7. When every step is in, the card shows one sentence per observable, e.g. "rear slip rises with
   each click, about 0.1 deg per click, linear (exit, slow corners)". Check the hint afterwards with
   the `run_context` query above for each step's experiment.

Not seen in game yet: anything in this section.

## What the agent does with the car, and when it gives up

| Situation | What happens |
| --- | --- |
| Start of every block | Teleport to the pit box, repair, fuel as at the first block, load the block's setup and read it back, hand to the AI |
| The AI wants its own pit stop | Prevented every 2 s (`setAILapsToComplete`, `setAIPitStopRequest`) |
| Under 15 km/h for 20 s (on track or parked in the pit lane) | Back to the pits, the block is retried |
| More than 5 units of damage in any zone on track | Back to the pits, repaired, the block is retried |
| Any damage in the pit lane on the way out (brushing the pit wall) | Repaired on the spot, the AI drives on |
| Reset by the game, or no lap counted for 10 minutes | Back to the pits, the block is retried |
| Less than 30 L at the start | Every block starts with 30 L (or the first block's fuel, if more), never more than the tank holds |
| The fuel read back 1 s after the refuel is more than 0.5 L off | The run stops: "the car has 24.0 L after refuelling for block 1, the plan wants 30.0 L" |
| A plan pins the AI's tyre hint and the hint given is another (or CSP refused it) | The run stops, naming both hints; the `ai_tune` file is never written during a pinned run |
| Free laps below 4 L | Back to the pits to refuel; not a retry |
| Free laps while a plan for a human is open on the car | The AI laps the car's own setup (block 0) and the laps count for no experiment; the plan's blocks stay for the driver |
| Three retries in one block (in free laps: without a lap in between) | The run stops and the window says why |
| A setup value does not take | Every change is undone and the run stops |
| The block's setup is not on the car when read back afresh after the load, or on leaving the pits (the car moved or put back a value) | The run stops before a lap is driven: "AI driver stopped: Setup S is not on the car after leaving the pits: WING_1 reads 1, the plan wants 0." Every differing value is named with what the car has and what the plan wants. No reload: a value the car does not hold would not hold on the next try either |
| The track gives no physics access | Nothing happens to the car; the window says so |

`Documents/Assetto Corsa/paddock/agent_status.json` always says what the agent is doing (phase,
block, message), so a run can be followed without looking at the screen.

## Results seen in Assetto Corsa (2026-11-15)

Ferrari 488 GT3 at COTA F1 2026 (Paddock test), practice, CSP 0.3.0-preview581.

| Check | Result |
| --- | --- |
| Physics access on the copied layout | `physics.allowed()` true; the original layout untouched |
| The AI takes the player's car | Yes, after `ac.tryToStart` leaves the pit menu; drove out of the pits and lapped |
| Its pace | 2:13.6 on its first clean lap; the owner's best in this car on this track is 2:12.65 |
| Setup editable in the pit box | Yes, without opening the setup screen |
| The AI's own pit stops | In practice it pits after a stint (and refuelled once); blocked as above |
| Damage | One run picked up 24 to 26 units and the AI pitted itself; this is why blocks now retry on damage |
| An AI-driven experiment | Experiment 2 (120 % grip) stuck at turns 19 and 20 on the softer bar and was aborted; experiment 3 below |

## Results seen in Assetto Corsa (2026-11-16): calibration and a full AI experiment

Same car, track and CSP.

| Check | Result |
| --- | --- |
| Leaving the pits on real grip | The AI brushes the pit wall at about 9 km/h on every exit; repaired in place since |
| Speed caps (method 1) | Did not work: section 184 capped to 106 km/h on the approach, the AI still arrived at 119 and went off |
| Tyre hint (method 2) | 1.00, 0.97, 0.94, 0.91: off at the esses and turn 19 every lap; 0.885: clean |
| Fuel | Free-lap restarts kept the car's fuel; at 2 L the AI headed for the pits at turn 20 and AC counted a reset, taken for a mistake twice. Runs now start with at least 30 L |
| Calibrated | Two clean laps at 88.5 % of the car's tyre hint; first clean flying lap 2:15.8 (owner's best 2:12.65) |
| Experiment 3: ARB front 3 against 2, AI-driven, 4 blocks of 3 laps | Driven to the end on its own: 4 setups loaded and read back, 16 laps, 18 clean laps in a row, no retry |
| Its verdict | Inconclusive: the softer bar is 4.2 ms per corner faster (90 % interval -6.8 to -1.6 ms, 204 passes), straddling the 5 ms that counts as worth having; guardrails unchanged |
| Precision | ±2.6 ms per corner from 12 laps |

## Not verified

- Other cars and tracks. The AI's quality depends on the car's AI data and the layout's AI line.
- Whether an effect measured with the AI predicts the effect for a human driver: the AI drives
  planning with 88.5 % of its tyre hint (this car and layout; real grip 100 %), below the limit where a human finds the last tenths.
- The window's look in the game was seen once, from a screenshot taken inside the game; its
  buttons were not clicked in the game.
