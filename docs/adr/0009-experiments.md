# ADR 0009: How an experiment runs

Status: accepted (2026-11-12)

## Context

Phase 3 turns a setup change into an experiment: propose, plan, drive interleaved blocks,
judge, decide, and put the baseline back when the change did not earn its place
(`docs/design.md` section F). Three processes take part and one of them runs inside a game
that cannot be called. This records who decides what, how core reaches the agent, and the
rules that are not obvious from the code.

## Decision

**The state machine lives in core; the engine only answers questions.** Core owns the
experiment's state, its run plan, the setup versions and the ledger
(`core/internal/experiments`). The engine has two stateless endpoints: `POST /plan` says what
a plan can detect, `POST /evaluate` judges the passes core sends it. Every move is one
database transaction that also writes an `experiment_event` row with the reason, so the
history of an experiment is the list of its events and nothing else.

**The test is frozen at the proposal.** The success test (corner time, 5 ms band, 90 %), the
guardrails and the predictions are stored on the experiment when it is proposed. Nothing
changes them afterwards; evaluation reads them back from the row.

**Core reaches the agent through files (the outbox).** Core writes
`Documents/Assetto Corsa/paddock/outbox/runplan.json` (temp file and rename; a `revision`
number goes up whenever it changes) and one setup file per arm under
`outbox/setups/<car>/`. The agent reads the run plan every two seconds, only while the car is
in the pits. This is the spool of [ADR 0003](0003-spool-transport.md) in the other direction,
for the same reason: nothing can block the game thread, and either side can be restarted.
Core never writes into the game's own setups folder and never overwrites a setup file.

**Setups go onto the car through the setup spinners, not by loading a file.** The run plan
carries each block's complete setup as raw values. The agent sets every value that differs,
reads each one back, and undoes all of them if one did not take. The `.ini` files in the
outbox are a record and a fallback for loading by hand.

**Blocks are blind.** The two setups get code names ("Setup K", "Setup R") drawn at random
from letters that suggest no order; either can be the baseline. The run plan lists blocks by
code name and never says which is which. The dashboard shows which setup was the baseline
only after the verdict.

**A pass counts only when the block's setup is on the car.** The agent tags a pass with its
block and lap only while the car's values equal the block's. The first lap after the pits is
lap 0 (out-lap) and is never judged. A pit visit or restart makes the next lap an out-lap
again. Whether a pass is usable is the engine's call; the agent only says what it was driven
on.

**Every experiment starts from the champion.** The champion is the setup trusted for a car
on a track. The first experiment that starts makes its baseline the champion and the
"original"; an accepted variant becomes the champion; a rollback returns it to that
experiment's baseline. A new proposal takes the champion as its baseline, and a proposal
whose baseline is no longer the champion cannot start (409).

An earlier version took the baseline from the session's latest pass. The end-to-end test
showed why that is wrong: after a rejected experiment the baseline goes back on the car
without a lap being driven, so the latest pass is still on the rejected variant and the next
experiment would have started from it.

**Ties go to the baseline, automatically.** Rejected moves to reverted in the same
transaction. Inconclusive can be extended by two blocks, at most twice; a third inconclusive
verdict reverts. Reverting appends a block with zero laps on the baseline arm, which is how
the agent is told to put the baseline back.

**Re-check every three accepted experiments.** Each accepted step was measured against the
step before it. After three, core proposes a re-check: the champion against the original. If
it fails, the original is the champion again. This is the check that v0 lacked when its setups
drifted from v2 to v9.

**The driver decides trade-offs.** When the variant is faster but a guardrail moved, the
engine does not choose. The experiment waits in `tradeoff` for the driver's call, and the
verdict records that the driver made it.

## Consequences

- A lost outbox file costs nothing: core rewrites it on the next state change, and the agent
  keeps its position in `paddock/agent_state.json`.
- One experiment at a time per installation can be on the car. A second one cannot start
  until the first is settled.
- Starting a fresh line of development from a different setup on the same car and track is
  not possible yet: there is no way to replace the champion except through experiments.
- The engine can be replaced or run elsewhere without touching the ledger; it holds no state.

## Rejected

- **A socket or HTTP server inside the game.** CSP can open sockets, but a blocked or failed
  call sits on the game thread, and the agent would need retry logic that files do not.
- **Loading the setup file through the game.** There is no documented CSP call that loads a
  setup file while in the pits; setting spinner values is what v0 did and tested.
- **Letting the engine own the state.** It would need the database, and a verdict could then
  change state without core's transaction and event.
- **Baseline from the latest pass.** See above; found by the end-to-end test.

## Not verified

Everything on the game side: that the spinners accept the values on every car, that a value
set this way holds when the car leaves the pits, and that the two-second poll is unnoticeable.
See `docs/runbooks/agent-in-game.md`.
