# ADR 0026: Linked setup items are one setting

Status: accepted (2026-11-17). Amends the one-factor variant of [ADR 0009](0009-experiments.md),
the base setup of [ADR 0018](0018-base-setup.md), the candidates of
[ADR 0011](0011-sensitivity-and-ranking.md) and the next step of
[ADR 0020](0020-ai-training-program.md).

## Context

On the RSS Formula Hybrid 2026 two setup items, `WING_1` and `WING_9`, are both labelled "Rear
Wing" with range 0..2, and the car's scripts keep `WING_9` equal to `WING_1`. AI experiments 13
(`WING_1` -1) and 14 (`WING_9` -1) each changed one of them, so the variant (setup versions 156
and 163) cannot exist on the car: after every pit exit the agent read back both at one value
(the spool of session 20261117T153924 alternates between 0/0 and 1/1), never the plan's, and
both experiments were aborted. Every setup the agent read back from that car has the two equal
(versions 1, 18, 104, 108, 111, 116, 122, 128 and 158: seven at 1/1, two at 0/0).

The setup menu the agent reports does not say that items are linked; CSP exposes the spinners,
not the car's scripts.

## Decision

**Detection** (`core/internal/setups/links.go`, pure). On one car, two items are linked when

1. both may be changed by an experiment (not excluded),
2. they have the same label, minimum, maximum and step,
3. at least `MinRecordedVersions` (3) setups were read back from the car, every one of them has
   both items, with the same value, and
4. that value is not the same in all of those setups.

"Read back from the car" is a `setup_version` with the agent's fingerprint (`agent_hash`): the
agent sets it only when it reported that setup on the car. A variant core wrote that never got
there (versions 156 and 163) does not count, so an impossible setup cannot unlink a group.

Why each part:

- Same label and range: a car that links two spinners shows the same thing twice. Items with
  different labels or ranges are different settings even when their values happen to match.
- Equal in every recorded setup: one recorded difference proves the car does not link them, and
  the rule is reversible by data: a setup with the two apart unlinks them on the next request.
- At least three setups: two equal values can be chance; three distinct readings from the car
  are the fewest that let the next rule mean anything.
- It must have moved: items that never changed are equal in every setup and say nothing about
  each other. On the real data this rejects 29 such pairs on the FH26 (the dampers on each
  corner, all "Bump", "Rebound", ...) and 10 on the Ferrari 488 GT3.

**Declared groups.** `setups.DeclaredLinks`, keyed by the car's game id, lists groups seen in
the game: today only the FH26's `WING_1` + `WING_9`. A declared group counts without any
recorded setup (a fresh database, the demo). A member that is not on the car's menu is ignored.

**One setting everywhere.** A group goes by its first member by name, its leader.

- `setups.Step` moves every member by the same clicks and validates each. The variant's values
  are the arms' values, the run plan's and the `.ini` files' (they are all read from the
  version), so all three carry both members.
- A one-factor experiment on any member is filed under the leader (`experiment.parameter_id`).
- A base setup's changes are filed under leaders; two members asked to move differently are
  refused.
- `GET /sessions/{id}/setup-items` (the propose form, the diagnosis's items, the base setup's
  items) lists the group once, under its leader, labelled `Rear Wing (WING_1 + WING_9)`.
- The ledger labels an experiment by what its setups record: when the variant moved more than
  one item, the label names the group with every member moved. Experiments 13 and 14 moved one
  item and stay labelled "Rear Wing" as recorded; nothing in the database is rewritten.
- Ranking: the diagnosis's candidates are filed under leaders and merged, so a group is one
  candidate. Measurements on any member are filed under the leader, so testing one member counts
  as testing the group. The car's knobs keep one per group, so the group does one job: it can
  lend to and borrow from another car's rear wing, and a base setup no longer treats the two
  members as a left/right pair it may not move.
- Training's `next()` counts a step on any member as the group tested and proposes the leader,
  never a lone member.

The engine is not changed: core sends it one name per setting.

## Consequences

- On the real data the rule finds `WING_1` + `WING_9` on the FH26 (also declared) and nothing on
  the Ferrari 488 GT3, the VRC Formula Alpha 2026, the Nissan GT-R or the BMW M4 (the last three
  have one recorded setup each). Cars without linked items behave exactly as before.
- A driver who always sets two same-label items alike (say left and right bump) and changes them
  together would see them linked. An experiment then moves both by the same clicks: a setup that
  can be driven, but one change where there were two. A single recorded setup with them apart
  unlinks them.
- Detection reads the car's recorded setups on each request that needs the menu; a car has tens
  of them, not thousands.

## Not verified

The rear wing group has not been driven in game: that an experiment moving both members passes
the agent's read-back after pit exit is unverified in game.
