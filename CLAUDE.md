# CLAUDE.md — Apex Forge: Paddock

Read `docs/architecture.md` and the ADRs in `docs/adr/` before changing anything. The full brief is
`docs/MASTER_PROMPT.md`.

## Ground rules
- Engineering decisions must be testable, measurable, explainable and reversible.
- Facts come from deterministic code. LLM output never enters the ledger as a number.
- A recommendation is a hypothesis with a prediction; failed experiments are kept.
- Never claim in-game behaviour works unless it was run in Assetto Corsa. Say "unverified in game".
- Do not modify `C:\Users\Jason\Desktop\ApexForge` (v0), Energy Mapper or MusicBlender.

## Layout
- `agent/` CSP Lua (pure `core/`, CSP shell `game/`), tests under lupa + mocked CSP
- `core/` Go monolith: ingest, setups, experiments, api, store
- `engine/` Python analysis: features, stats, diagnosis, sensitivity, optimize, explain
- `web/` React + Vite + TS
- `contracts/` JSON Schema, source of truth for cross-process types (codegen, never hand-edit)

## Commands
- `make dev` start Postgres + core + engine + web
- `make test` all suites; `make lint`; `make gen` regenerate contracts
- `make demo` run with `datasets/demo` and no game (added in phase 1 with the demo dataset)

## Conventions
- Files < 300 lines, functions < 40 lines, descriptive names, no dead code.
- Tests with every change; table-driven in Go, pytest + hypothesis in Python, Vitest in TS.
- Structured logs with `session_id` / `experiment_id`. Errors wrapped with context.
- Migrations reviewed, reversible, never edited after merge.
- New dependency or surprising decision: write an ADR.
- In-game agent: no `ac.*` in pure modules, `io.saveAsync`, write on change only, <= 5 Hz.

## Finishing a task
Report: what changed and why, how it was verified (commands + output), known limitations, what is
not verified. Keep docs in sync in the same change.
