# ADR 0003: The agent talks to core through spool files

Status: accepted (2026-11-11)

## Context

The in-game agent runs on Assetto Corsa's game thread. Any stall there is a visible stutter,
which would end the project for its only user. The agent must hand corner-pass records to a
process outside the game, and that process may not be running.

Options considered:

1. **Files.** Append JSON lines with CSP's `io.saveAsync`; core tails the directory.
2. **HTTP** to localhost with CSP's `web.post`.
3. **Shared memory or a named pipe.**

## Decision

Option 1. The agent appends one JSON line per corner pass to
`Documents/Assetto Corsa/paddock/spool/<session-id>.jsonl`, writing only on change and at most
5 times per second. Core tails the spool directory and is idempotent by
(session, corner, t_start). In the other direction core writes setup `.ini` files and
`runplan.json`, which the agent reads at most once per second while in the pits.

## Consequences

- Crash-safe in both directions: if core is down, lines wait on disk; if the game crashes,
  everything already written is kept.
- No sockets, retries or timeouts on the game thread. The same pattern is proven in the
  author's MusicBlender app.
- Latency is a second or two, which is fine: nothing downstream is real-time.
- Core must cope with partially written last lines and with replays of a file from the start.

## Revisit when

A feature needs sub-second feedback from core to the game. `web.post` to localhost is the
candidate, but it is unverified against localhost and must be measured first. Ghost Driver's
333 Hz control loop is a separate channel (CSP Custom AI memory-mapped files) and is not
affected by this decision.
