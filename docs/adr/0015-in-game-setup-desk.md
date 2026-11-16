# ADR 0015: The in-game window is the setup desk; it talks to core over HTTP for control only

Status: accepted (2026-11-15).

## Context

Until now the agent's window showed three lines and a Load button, and everything else
happened in the web dashboard: proposing, starting and judging experiments. A driver had to
leave the game to start the next test. The owner asked for the split to be clear: the game is
where setups are managed, the dashboard is where telemetry is studied.

The agent hands data to core only through spool files ([ADR 0003](0003-spool-transport.md)):
crash-safe, and the game never waits for a network. That stays.

## Decision

The window becomes a setup desk with two tabs. **Run**: recording state, the block being driven
with a progress bar, the Load button, and the AI driver ([ADR 0014](0014-ghost-driver.md)).
**Setups**: the open experiments of this car with their laps, a Judge button once a plan is
driven, the last verdict, and the ranked proposals of what to test next, each startable with
"I drive it" or "AI drives it".

The desk reads and acts through core's existing REST API with CSP's `web.get` / `web.post`:
`GET /sessions` (to find the session by its spool id), `GET /experiments`,
`GET /sessions/{id}/proposals`, and on a click `POST /experiments`, `/plan`, `/start`,
`/evaluate`, or `/sessions/{id}/diagnosis`. Requests go out only while the window is drawn, at
most every 10 s, one at a time (CSP allows two), and never from a frame without a click except
that refresh. If core does not answer, the desk says so and capture is unaffected.

Turning answers into lines and request bodies is pure Lua (`agent/src/core/desk.lua`), tested
offline; the HTTP calls and drawing are in `game/desk_link.lua` and `game/window.lua`.

## Alternatives considered

- **Control through files too** (the agent writes requests into a spool, core answers into the
  outbox). Keeps one transport, but turns a click into a round trip of seconds and puts a
  request queue in two places for a few calls a session.
- **Keep control in the web dashboard only.** Simple, and what the owner found awkward: the
  decision about the next setup happens in the car.

## Consequences

- Core must be reachable at `http://127.0.0.1:8080` from the game for the Setups tab. Capture
  needs nothing but the spool folder.
- The web dashboard keeps its own propose form; both use the same endpoints.

## Not verified

- The window's look in the game: the game ran fullscreen, so it was not seen. The requests and
  buttons are tested against a fake core on the mocked CSP API.
