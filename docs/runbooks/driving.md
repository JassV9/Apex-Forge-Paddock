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
   car and the track. A layout marked "AI can drive" is a Paddock test copy; on an original the
   panel shows the command that makes one.
4. Press **Start driving**. The launcher backs up `controls.ini`, writes
   `Documents/Assetto Corsa/paddock/launch/paddock.cmpreset` and asks CM to start Practice in
   the pits, clear, on an optimum track. If CM cannot start, it says so in its own window.
   Drive the "(Paddock test)" layout: your corners are compared with the AI's only on the
   same layout and setup ([ADR 0019](../adr/0019-ai-reference-driver.md)).
5. Drive. The agent records every corner pass; core ingests them as you go.
6. Leave the game. Within a few seconds the panel says whether the wheel bindings were
   unchanged or restored.
7. Open the new session in the rail for its telemetry.

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
