# Runbook: the driving flow

## Start here: from Start driving to telemetry

Why it works this way: [ADR 0017](../adr/0017-host-launcher.md). **Seen in game (2026-11-16):**
Start driving opened CM, which started the game 7 s later in Practice, in the pits, on the
chosen car and test layout, clear and 100 % grip; after the game closed the launcher found the
bindings unchanged (same SHA-256), also when it was restarted in between. A restore after a
real change of bindings has not happened yet.

### Once

1. Content Manager is installed and has been opened at least once, so that `acmanager://` links
   open it (Windows registers the scheme then).
2. Your usual Quick Drive settings are saved as a CM preset (default: `NJ`). The launcher copies
   it for every start and never changes it.
3. For the AI test driver, make the test copy of the layout:
   `python tools/prepare_test_layout.py fn_cota layout_f1_2026` (see
   [ghost-driver.md](ghost-driver.md)).

### Every session

1. Start the stack: `make dev` (Docker).
2. Start the launcher on Windows, in a second terminal, and leave it open:

   ```bash
   make launcher
   # or: cd core && go run ./cmd/paddock-launcher --template NJ26
   ```

   Flags (and environment variables): `--ac-root` (`PADDOCK_AC_ROOT`, default the Steam
   folder), `--documents` (`PADDOCK_DOCUMENTS`, default your Documents folder), `--template`
   (`PADDOCK_CM_PRESET`, a preset name or a `.cmpreset` path), `--weather` (`PADDOCK_WEATHER`,
   default `3_clear`, with the Optimum track; `template` keeps the preset's), `--port`
   (default 8090).
3. Open http://localhost:5173. In the **Drive** panel at the top of the session rail, pick the
   car and the track. A layout marked "AI drives here" is a Paddock test copy. On an original
   layout the panel says the game shows no AI times there, and that in the web app your laps
   are compared with the AI's on its test copy, the same track
   ([ADR 0027](../adr/0027-layout-variants.md)); it offers **Use the test copy** (or shows the
   command that makes one), and under the pickers lists what is open on the car (below).
4. Press **Start driving**. You drive: the launcher sends no AI command. It backs up
   `controls.ini`, writes `Documents/Assetto Corsa/paddock/launch/paddock.cmpreset` and asks CM
   to start Practice in the pits, clear, on an optimum track. If CM cannot start, it says so in
   its own window. Your corners are compared with the AI's on the same setup and track: in the
   game only on the "(Paddock test)" layout, in the web app also from the original layout
   ([ADR 0019](../adr/0019-ai-reference-driver.md), [ADR 0027](../adr/0027-layout-variants.md)).
5. Drive. The agent records every corner pass; core ingests them as you go. The **Paddock
   Hotlap** window shows each corner against your best and the AI's; on the original layout it
   says there are no AI times and why.
6. Leave the game. Within a few seconds the panel says whether the wheel bindings were
   unchanged or restored.
7. Open the new session in the rail: telemetry, and **Against the AI** for the corners where
   you lose time the AI does not. A session on the original layout says it has nothing to be
   compared with and which layout to drive instead.
8. **Experiments > Build a base setup** (it proposes that you drive). Open the experiment it
   made, press Plan and Start, then in the game stop in the pits: the Paddock window names each
   block, its Load button puts the setup on the car, and you drive the laps it asks for. Press
   Evaluate when they are in.

### When an AI experiment or training program is still open on the car

The Drive panel names it before you start. A game that starts with an AI experiment open is
yours ([ADR 0021](../adr/0021-human-keeps-the-car.md)): the agent does not teleport the car,
load setups or hand it to the AI, and your laps are not counted in that experiment. The
window's Run tab says "Experiment N is for the AI to drive". To hand the car over press **Let
the AI drive this plan** there; to drop the experiment, abort it under Experiments (and press
**Stop training** if a program is open, or it will queue the next experiment, which also waits
for you). This needs the agent reinstalled (`python tools/install_agent.py`). **Unverified in
game.**

### When something is off

| Panel says | Do |
| --- | --- |
| The launcher is not running | Run `make launcher` on the Windows host, not in Docker |
| Assetto Corsa is running | Leave the game; one session at a time |
| Waiting for the last start | The game has not opened and closed since the last press; it gives up waiting for the game after 3 minutes |
| Wheel bindings restored | Nothing: they are as before the session. The file the game left is in `paddock/controls_backup/changed_*.ini` |
| Could not be restored | Copy the newest `paddock/controls_backup/before_*.ini` over `Documents/Assetto Corsa/cfg/controls.ini` by hand |
| car/layout is not installed | The car or layout folder is gone or was renamed; reload the page |

Launcher logs are JSON lines on its terminal (`asked Content Manager to start practice`,
`wheel bindings checked after the game` with `state` and `detail`).
