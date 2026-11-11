# Runbook: the agent in Assetto Corsa

**Status: unverified in game.** Everything the agent does is tested offline against a mock of
the CSP API. None of it has been run in Assetto Corsa yet. This runbook is the procedure for
the first real run and the list of things that run has to confirm.

## Install

Needs Assetto Corsa with Custom Shaders Patch (the agent declares `REQUIRED_VERSION = 3465`).

```bash
python tools/install_agent.py
```

This copies `PaddockAgent.lua`, `manifest.ini` and `src/` to
`<assettocorsa>/apps/lua/PaddockAgent/`, replacing an older copy of that folder and nothing
else. Use `--ac-root` if the game is not in the default Steam folder. Then start a practice
session and enable **Paddock Agent** in the apps bar on the right edge of the screen.

The agent writes only under `Documents/Assetto Corsa/paddock/spool/`. It does not touch
setups, and it is separate from Apex Forge, VRC Energy Mapper and MusicBlender.

## Point core at the spool

Docker (PowerShell):

```powershell
$env:PADDOCK_SPOOL_HOST = "$HOME/Documents/Assetto Corsa/paddock/spool"
make dev
```

Native core: set `PADDOCK_SPOOL_DIR` to the same folder (see `local-dev.md`).

Open http://localhost:5173. A few seconds after the first corner, the session appears in the
list and its pass count grows as you drive.

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

Things the offline tests cannot show: real frame cost (the mocked benchmark measures only the
Lua, at about 0.002 ms per frame), behaviour of `io.saveAsync` under load, and whether every
CSP field holds what its name says on every car.

## Remove

Delete `<assettocorsa>/apps/lua/PaddockAgent/`. The spool folder can be deleted at any time;
core has already copied what it ingested.
