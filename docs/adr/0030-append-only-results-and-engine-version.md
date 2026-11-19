# ADR 0030: Results are kept for every verdict, with the engine version that computed them

Status: accepted (2026-11-18). Implements I2 of the
[reliability audit](../investigations/2026-11-18-ghost-driver-reliability.md).

## Context

`core/internal/experiments/verdict.go` replaced an experiment's results on every evaluation
(`DELETE` then `INSERT`), and no row said which engine computed it. Verdicts are append-only, so
an extended experiment had two verdicts and one set of numbers, the latest. Results judged by
engines before ADR 0022, 0023 and 0025 (experiment 7 on a half-driven lap: -15.75 [-53.2, +21.7]
stored, -1.03 [-19.7, +17.7] now) sit beside new ones and are pooled as if alike.

## Decision

Migration `00012_findings_and_append_only_results.sql` (00011 is taken by a change written in
parallel; goose applies them in order on a fresh database and both together on the live one):

- `result` gains `verdict_id` (the engine verdict the numbers belong to) and `engine_version`.
  Core inserts the verdict first, then its results under its id. Nothing deletes a result.
- The view **`current_result`** is what the ledger shows and pools: the results of an
  experiment's latest verdict that has any, or, for experiments judged before this migration,
  their rows without a verdict id. `readResults` and the sensitivity reader use it.
- `verdict` gains the finding of [ADR 0029](0029-block-level-inference-and-finding-classes.md):
  `finding_class` (CHECK: the eight classes, NULL allowed), `recommendation` (CHECK),
  `flags`, `confounders` (JSONB), `driver_adapted`, `engine_version`, `passes_used`,
  `passes_excluded`, `look`, `margin`. The driver's and a program's decisions leave them NULL; the
  API shows the finding of the latest engine verdict with the latest decision.
- The view **`verdict_finding`** derives a finding for engine verdicts judged before this, from
  their decision and reason (accepted or tradeoff: confirmed improvement; "Worse on":
  confirmed regression; "No meaningful effect": neutral; "Not enough passes": invalid; otherwise
  insufficient evidence), with `derived = true`: another method judged them, and the dashboard
  says so. Past verdicts are not re-judged.
- The engine version is the engine package's version (`engine/pyproject.toml`), sent on every
  evaluate response. It is bumped with every change to how a verdict is computed: 0.2.0 is ADR
  0029.

Down deletes the results a later verdict superseded (that history exists only since this
migration), then drops the views and columns: the table means again what it meant before.

## Alternatives considered

- **A results history table beside `result`.** Two tables to keep in step; the view gives the
  same reading with one.
- **Re-judging old experiments with the current engine and storing the new numbers.** The
  ledger records what was decided and why; a replay belongs in an ADR, not over the record.
  Pooling that needs today's method re-derives from the passes (ADR 0033).

## Consequences

- An extended experiment keeps the numbers of each look; a reader can see what the first look
  said and what the last said.
- Every number is traceable to the engine that computed it.
- Old rows have no engine version; their finding is marked derived.

## Not verified

- Unverified in game. Tested against PostgreSQL (append, current view, derived findings, the
  migrations up and down) and through the end-to-end test on the mocked game.
