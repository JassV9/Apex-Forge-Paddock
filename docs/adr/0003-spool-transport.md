# ADR 0003: The agent talks to core through spool files

Status: accepted (2026-11-11), amended in phase 1 (see "Amendment")

## Context

The in-game agent runs on Assetto Corsa's game thread. Any stall there is a visible stutter,
which would end the project for its only user. The agent must hand corner-pass records to a
process outside the game, and that process may not be running.

Options considered:

1. **Files.** Write JSON lines with CSP's `io.saveAsync`; core tails the directory.
2. **HTTP** to localhost with CSP's `web.post`.
3. **Shared memory or a named pipe.**

## Decision

Option 1. The agent writes one JSON line per corner pass under
`Documents/Assetto Corsa/paddock/spool/`, only when there is something new and at most 5 times
per second. Core tails the spool directory and is idempotent by (session, corner, t_start). In
the other direction core writes setup `.ini` files and `runplan.json`, which the agent reads at
most once per second while in the pits.

## Amendment (phase 1): a session is a directory of chunk files

The brief describes one file per session, `<session-id>.jsonl`, that the agent appends to.
CSP's Lua API has no append: `io.save` and `io.saveAsync` replace the whole file (checked in
the SDK that ships with CSP, `extension/internal/lua-sdk/ac_apps/lib.lua`). Rewriting a
growing session file on every pass would mean megabytes per write.

So a session is a directory, and every write is a new, immutable file:

    spool/<session-id>/000001.jsonl    session line (car, track, corner map)
    spool/<session-id>/000002.jsonl    one or more pass lines
    ...

- Each file holds every line that was waiting when the agent was next allowed to write.
- Files are written with `io.saveAsync(..., ensure = true)`: CSP writes a temporary file and
  renames it, so core never sees a half-written file.
- Core remembers which files it has ingested in its database (`spool_chunk`), so it never
  writes to, moves or deletes anything in the spool. The spool can be mounted read-only.
- The line formats are `contracts/spool_session.schema.json` and
  `contracts/spool_pass.schema.json`.

## Consequences

- Crash-safe in both directions: if core is down, files wait on disk; if the game crashes,
  everything already written is kept. At most the pass being encoded is lost.
- No sockets, retries or timeouts on the game thread. The same pattern is proven in the
  author's MusicBlender app.
- Latency is a second or two, which is fine: nothing downstream is real-time.
- Many small files per session (one per corner pass, about 50 kB each). Nothing prunes the
  spool yet; a clean-up rule is needed once sessions pile up.

## Revisit when

A feature needs sub-second feedback from core to the game. `web.post` to localhost is the
candidate, but it is unverified against localhost and must be measured first. Ghost Driver's
333 Hz control loop is a separate channel (CSP Custom AI memory-mapped files) and is not
affected by this decision.
