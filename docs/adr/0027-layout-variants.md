# ADR 0027: A layout and its test copy are one track in the web app

Status: accepted (2026-11-17). Amends the layout paragraph of
[ADR 0021](0021-human-keeps-the-car.md) and the matching of
[ADR 0019](0019-ai-reference-driver.md). **Unverified in game and in a browser**: tested with Go
against PostgreSQL, Vitest and the agent's pytest; the grouping was run read-only on the real
rows.

## Context

The AI drives only `<layout>_paddock`, the copy that `tools/prepare_test_layout.py` makes of a
layout (a byte copy plus a marker file and app physics access in `surfaces.ini`,
[ADR 0014](0014-ghost-driver.md)). Ingest keys a track row by `ac_id, layout,
corner_map_version`, so the copy is another track row, and the reference compared a session
only with the AI on its own row. The owner drives the original layout (for example
`fn_hungaroring / layout_f1_2026`): his laps there were recorded and never compared with the
AI, although the copy is the same physical track.

He also wants the game to stay as it is on an original layout: no AI ghost, no AI times, no AI
record in the hotlap window.

## Decision

**Variant rule** (`core/internal/reference/variants.go`, pure). Two layouts of one game track
(`ac_id`) are variants when one is the other plus `_paddock` (`launcher.TestCopySuffix`); a
single-layout track (`layout` "") has none. A variant's track row is grouped with the session's
only when their corner maps match: the same corner numbers and every `apex_m` within
`ApexToleranceM` (5 m). A byte copy matches exactly; the tolerance only absorbs rounding. A map
with no corners, or a corner without a number or `apex_m`, does not match. When a variant row
does not match, the reply says why, naming the first difference (`variant_note`, e.g. "corner 2's
apex is 50.0 m from this layout's, more than 5 m").

**Read side only.** Track rows, corners and passes stay as ingest stored them; nothing is
rewritten or merged. `GET /sessions/{id}/reference` loads the AI's passes of the car on the
session's own track row and on each grouped variant row, and lists them in `ai_tracks` (layout,
name, `variant`, AI passes per row). Corners are matched by number, which the corner-map check
makes safe. The AI overlay in the telemetry reads the AI's best pass from that reply, so it
follows the grouping.

**The game is not grouped.** `GET /reference?car=&track=&layout=&setup_hash=`, which only the
hotlap window asks, keeps its exact-layout lookup: on an original layout it answers no AI
corners even when the AI has driven the copy, and the window keeps saying "No AI times: this is
the original layout". Guarded by `TestForSetupNeverGroupsVariantsForTheGame` (core) and the
hotlap test that the window asks for its own layout only (agent). The agent is unchanged.

**The web says so.** "Against the AI" labels AI passes from a variant ("AI on COTA F1 2026
(Paddock test), the same track. Compared in the web app only: the game shows AI times only on
the layout the AI drove."), the AI overlay checkbox carries the same label, and the empty state
for an original layout says laps there are compared in the web app with the AI's on its copy and
never shown in the game. The Drive panel says the same for an original layout ("no AI times in
the game. In the web app your laps are compared with the AI's on its test copy, the same
track") and marks the copy "AI drives here".

**Experiments and the ledger are unchanged.** Setup measurements are not pooled across layouts;
a block counts only passes on its own track row, as before.

## Consequences

- On the real data (read-only, 2026-11-17) COTA groups: `fn_cota / layout_f1_2026` (track 1)
  with `layout_f1_2026_paddock` (track 22), identical corner maps (17 corners). Session 21 on
  the original is now compared with 1343 AI passes on the copy, every corner on the nearest
  setup the AI drove; `GET /reference` on the original still answers no corners. The
  Hungaroring has only the copy (track 76) so far: the owner's first session on the original
  makes a new row, which groups if its corner map matches.
- A corner map rebuilt on one layout only (a new `corner_map_version`) stops the grouping for
  that row, with the reason shown, until the other is rebuilt too.
- Reversible: no data was changed, so reverting this change restores the per-row comparison.

## Known limits

- The rule trusts the `_paddock` name and the corner map, not the files: a copy edited by hand
  with the same corner positions would still be grouped.
- Corner positions are compared, not their zones or names; names come from the session's own
  map.
