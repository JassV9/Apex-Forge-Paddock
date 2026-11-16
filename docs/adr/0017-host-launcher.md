# ADR 0017: Start driving from the dashboard through a host launcher and Content Manager

Status: accepted (2026-11-16). **Unverified in game and in Content Manager**: nothing in this ADR
has been run against Assetto Corsa or Content Manager yet.

## Context

The owner's flow is "get in, hotlap, get a base setup, then small changes, and view my
telemetry after the session". Getting in meant leaving the dashboard, opening Content Manager,
finding the car and the layout (for the AI, the `<layout>_paddock` test copy of
[ADR 0014](0014-ghost-driver.md)), choosing Practice and a pit start, and pressing Go.

Three constraints:

- **Docker cannot start Windows programs.** Core, the engine and the dev web server run in
  Docker ([ADR 0006](0006-no-kafka-no-kubernetes.md)); the game and Content Manager (CM) run on
  the Windows host.
- **The owner's settings are CM's.** Weather, time, track state, assists and the car's skin
  live in CM. Paddock must not keep a second copy of them or change CM's files.
- **The wheel bindings must survive.** `Documents/Assetto Corsa/cfg/controls.ini` holds them.
  When CM or the game saves controls while a device is unplugged or enumerated late, it
  rewrites `[CONTROLLERS]` (`CONn`, `PGUIDn`, `__IGUIDn`) and the `JOY=` device indices, and the
  wheel stops working until it is rebound by hand.

## Decision

**A small host program, `paddock-launcher`** (`core/cmd/paddock-launcher`, package
`core/internal/launcher`, same Go module as core). It runs on Windows outside Docker
(`make launcher`) and listens on `127.0.0.1:8090` only. The dashboard calls it directly from the
browser; it answers CORS only for the dev server's origins (`http://localhost:5173`,
`http://127.0.0.1:5173`), refuses other origins and any Host header that is not a loopback name
(DNS rebinding), and takes `POST /launch` only as `application/json`, so a page on another site
cannot start the game with a form post. API: `docs/api/launcher.yaml`; bodies in
`contracts/launcher_content`, `launch_request` and `launcher_status`.

**CM's Quick Drive protocol.** For a launch the launcher copies the owner's Quick Drive preset
(default `NJ.cmpreset` in `%LOCALAPPDATA%\AcTools Content Manager\Presets\Quick Drive`, read,
never written), sets `CarId`, `TrackId` (`track/layout`), `Mode` (Practice) and `StartType`
`PIT`, writes it to `Documents/Assetto Corsa/paddock/launch/paddock.cmpreset`, and opens
`acmanager://race/quick?presetFile=<that path>`. CM's `ProcessRaceQuick` loads the preset and
presses Go with no dialog; on failure CM shows a toast. Everything else in the preset (weather,
time, track state, assists) stays the owner's. The URL is handed over with `ShellExecuteW`
("open"), exactly as a click on an `acmanager://` link, so no shell parses it.

The launcher checks the car and layout are installed before asking: CM silently keeps its
previous selection when a preset names something it does not know. It refuses while `acs.exe`
runs (read from the process list, no subprocess), and while the previous launch is still being
watched.

**The bindings guarantee.** Before each launch the launcher copies `controls.ini` to
`paddock/controls_backup/before_<time>_<hash>.ini` (the newest ten are kept) and records its
SHA-256. A watcher polls the process list every 3 s: it waits up to 3 minutes for `acs.exe` to
appear and then for it to exit, waits 5 s more for a late save, and compares `controls.ini` with
the backup on every `[CONTROLLERS]` line and every `JOY` and `JOY_MODIFICATOR` line. If any
differ, it keeps the changed file as `changed_<time>.ini`, checks the backup still has its hash,
puts the backup back with one rename and reads it back. That rename onto `controls.ini` is the
only write the launcher makes outside `Documents/Assetto Corsa/paddock/`. The watch survives a
launcher restart: `paddock/launch/state.json` says a check is pending, and the next start
resumes it. `GET /status` reports `watching`, `unchanged`, `restored` (with the lines that
differed) or `restore_failed`.

## Consequences

- One click in the dashboard: pick a car and a layout (test copies are marked "AI can drive
  here", originals say how to make one) and press Start driving.
- A binding the owner changes on purpose inside a game started this way is undone when the game
  exits; the changed file is kept in `controls_backup`. Rebind with the game started from CM
  directly, or restart the launcher afterwards (only launches it made are watched).
- Restoring writes back the whole backed-up file, so other settings changed in that game (force
  feedback gain, for example) are undone too, but only when a binding line also changed. When
  only other lines changed, nothing is written.
- The launcher is a second process the owner must start by hand. The dashboard says how when it
  cannot reach it.
- `golang.org/x/sys` becomes a direct dependency of core's module (it was already an indirect
  one) for the process list, `ShellExecuteW` and the Documents known folder.

## What is not done

- **No skin choice.** A `.cmpreset` has no skin field; CM uses the car's last skin.
- **No change to CM's settings,** presets or the game's other config files. The template is
  read only.
- **No other modes** than Practice from the pits; no AI opponents; no online.
- **No start on the host's behalf** of core or the engine: the launcher only starts the game.
- The read-only demo build hides the Drive panel; the demo's nginx origin (`:8088`) is not
  allowed.

## Alternatives rejected

- **Launch from core in Docker.** A container cannot start a Windows program, and giving it the
  host's Docker socket or a shell would be a far larger hole than a loopback-only program.
- **Start `acs.exe` directly with a written `race.ini`.** That bypasses CM, so the owner's CM
  settings, CSP's launch-time configuration and CM's own preparation would be lost or would have
  to be rebuilt in Paddock.
- **`acmanager://race/config` with a full `race.ini`.** Same problem: Paddock would have to own
  every setting CM already keeps.
- **`cmd /c start "" <url>`** or **`rundll32 url.dll,FileProtocolHandler`.** Both work for
  simple URLs, but `cmd` parses `%` and `&` in the URL and `rundll32` re-splits its command
  line; `ShellExecuteW` takes the URL as one argument.
- **Block CM from saving controls** (a read-only `controls.ini`). It changes the game's files'
  attributes and breaks deliberate rebinding everywhere.
