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

Without the window (for scripted runs), the same steps go through core's API, and a free run
is started with a file the agent reads every two seconds:

```bash
echo "{\"id\": \"$(date +%s)\", \"at\": $(date +%s), \"command\": \"ai_laps\"}" \
  > "$HOME/Documents/Assetto Corsa/paddock/outbox/agent_command.json"
```

`"command": "stop"` stops it. A command older than 60 seconds is ignored.

## What the agent does with the car, and when it gives up

| Situation | What happens |
| --- | --- |
| Start of every block | Teleport to the pit box, repair, fuel as at the first block, load the block's setup and read it back, hand to the AI |
| The AI wants its own pit stop | Prevented every 2 s (`setAILapsToComplete`, `setAIPitStopRequest`) |
| Under 15 km/h for 20 s (on track or parked in the pit lane) | Back to the pits, the block is retried |
| More than 5 units of damage in any zone | Back to the pits, repaired, the block is retried |
| Three retries in one block | The run stops and the window says why |
| A setup value does not take | Every change is undone and the run stops |
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
| An AI-driven experiment | See the next section |

## Not verified

- Other cars and tracks. The AI's quality depends on the car's AI data and the layout's AI line.
- Whether an effect measured with the AI predicts the effect for a human driver.
- The window's look in the game was seen once, from a screenshot taken inside the game; its
  buttons were not clicked in the game.
