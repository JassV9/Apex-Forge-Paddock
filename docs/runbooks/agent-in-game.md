# Runbook: the agent in Assetto Corsa

**Status: capture verified in game once; experiments unverified in game.** Everything the
agent does is tested offline against a mock of the CSP API. Recording passes has been run in
Assetto Corsa (results below); loading setups and following a run plan have not. This runbook
is the procedure and the list of things a run has to confirm.

## First real run: 2026-11-12

Formula Hybrid 2026 (`rss_formula_hybrid_2026`) at COTA F1 2026, practice, CSP
0.3.0-preview581, started from Content Manager, driven by hand on a wheel for about six
laps. The game was fullscreen, so the agent's window text was not seen.

| Check | Result |
| --- | --- |
| The app loads | CSP's log: "Lua: <App: Paddock Agent> loaded and ready to run". The window itself was not seen |
| Passes reach the spool | 102 passes in files `000002` to `000103`: six laps of 17 corners, `seq` contiguous, one setup hash |
| Core accepts them | 0 rejected, no error or warning in core's log |
| Field names are right | Fuel fell from 49.8 to 48.4 L over the first lap; tyre core 74 to 79 C, pressures about 20 psi, grip 1 |
| Catalog and setup | 87 parameters and 78 values read from the car in the first file |
| Diagnosis on the session | The engine returned mid-corner understeer in all speed classes, with the driver-input and conditions alternatives. The template text was used (no model) |
| Sample rate | Median step 0.0221 s (45 Hz), 5th to 95th percentile 0.0198 to 0.0250 s. The recorder caps at 60 Hz, so this is the frame rate of that session |
| Slow-script warnings | None for the agent in CSP's log |

Not checked in this run: frame cost with the Lua profiler, restarting mid-corner, the setup
hash after a change in the pits, the experiment checks below. One session on one car and
one track says nothing about other cars.

## Install

Needs Assetto Corsa with Custom Shaders Patch (the agent declares `REQUIRED_VERSION = 3465`).

```bash
python tools/install_agent.py
```

This copies `PaddockAgent.lua`, `manifest.ini` and `src/` to
`<assettocorsa>/apps/lua/PaddockAgent/`, replacing an older copy of that folder and nothing
else. Use `--ac-root` if the game is not in the default Steam folder. Then start a practice
session and enable **Paddock Agent** in the apps bar on the right edge of the screen.

The agent writes only under `Documents/Assetto Corsa/paddock/` (the spool and
`agent_state.json`). It never writes a setup file; during an experiment it changes setup
values on the car through the setup screen's spinners, and only when you click "Load". It is
separate from Apex Forge, VRC Energy Mapper and MusicBlender.

## Point core at the spool and the outbox

Docker (PowerShell):

```powershell
$env:PADDOCK_SPOOL_HOST = "$HOME/Documents/Assetto Corsa/paddock/spool"
$env:PADDOCK_OUTBOX_HOST = "$HOME/Documents/Assetto Corsa/paddock/outbox"
make dev
```

Native core: set `PADDOCK_SPOOL_DIR` and `PADDOCK_OUTBOX_DIR` to the same folders (see
`local-dev.md`). Core reads the spool and never writes to it; it writes run plans and setup
files to the outbox and nowhere else in the game's folders.

Open http://localhost:5173. A few seconds after the first corner, the session appears in the
list and its pass count grows as you drive.

## Run an experiment

1. Drive a few laps so a session with its setup exists, then open **Propose** in the
   dashboard: pick the session, one setup item, a number of clicks, and write the hypothesis.
2. On the experiment's page: **Plan** (shows what four blocks can detect), then **Start**.
   Core writes `outbox/runplan.json` and one setup file per arm under `outbox/setups/<car>/`.
3. In the game, drive into the pits. Within two seconds the agent's window names the block:
   "Experiment 4, block 1 of 4: pit and load Setup K".
4. Open the setup screen and click **Load Setup K** in the agent's window. The agent sets
   every value that differs, reads each one back, and undoes all of them if one did not take.
5. Drive the out-lap and the push laps. The window counts them. When the block is done it
   names the next one; pit and repeat.
6. After the last block, click **Evaluate** on the experiment's page. A rejected or reverted
   experiment adds one more block with no laps: pit and load it to get the baseline back.

Which code name is the baseline is not shown anywhere until the verdict. The setup files in
the outbox are a record; they can also be copied into the game's setups folder and loaded by
hand if the spinners do not work on a car.

## What the first real run must confirm

| Check | How | If it fails |
| --- | --- | --- |
| The app loads | The window shows "recording N corners" | CSP's Lua debug app shows the error; `ac.log` lines start with "Paddock agent" |
| Passes reach the spool | New `000NNN.jsonl` files appear in `spool/<session-id>/` while driving | Check that the app is enabled and the track has an AI line |
| Core accepts them | `curl http://localhost:8080/metrics`: `paddock_ingest_rejects_total` stays absent or 0 | Core's log names the line and the rule it broke |
| No stutter | CSP's Lua profiler on the app; drive a lap with and without it | Lower the sample rate or channel count (`agent/src/core/channels.lua`) |
| Field names are right | Compare a pass's `conditions` with the in-game tyre and fuel apps | Fix the field in `carlink.lua`; names were taken from the SDK shipped with CSP |
| Restarts drop passes | Restart mid-corner; no pass with an impossible duration appears | Compare with `agent/tests/test_agent.py` |
| Setup hash changes with the setup | Change one value in the pits; `setup_hash` differs on the next passes | See `game/setup.lua` (verbatim from v0, where it worked) |

For experiments, additionally:

| Check | How | If it fails |
| --- | --- | --- |
| The run plan is seen | In the pits, the window names the block within a few seconds of **Start** | Check `PADDOCK_OUTBOX_HOST`; the window shows "Run plan ignored: ..." for a file it cannot read |
| Loading works | After "Load Setup X" the setup screen shows the new values, and they are still there after leaving the pits | The window says which item did not take and that every change was undone; load the outbox `.ini` by hand |
| Laps are counted once | The window's push-lap count goes up by one per lap, not on a spin across the line | `agent/src/core/runplan.lua`, `position` |
| Passes carry their block | The experiment page's block table fills while driving | A pass is only tagged while the car's values equal the block's; check the setup hash line in the spool |
| The poll is unnoticeable | Sit in the pits with CSP's Lua profiler open | Raise `POLL_S` in `agent/src/game/plan.lua` |

Things the offline tests cannot show: real frame cost (the mocked benchmark measures only the
Lua, at about 0.002 ms per frame), behaviour of `io.saveAsync` under load, and whether every
CSP field holds what its name says on every car.

## Remove

Delete `<assettocorsa>/apps/lua/PaddockAgent/`. The spool folder can be deleted at any time;
core has already copied what it ingested.
