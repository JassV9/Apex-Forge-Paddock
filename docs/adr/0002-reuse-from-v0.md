# ADR 0002: What is reused from Apex Forge v0.3.0

Status: accepted (2026-11-11)

## Context

Apex Forge v0.3.0 is a 4,400-line CSP Lua app with offline tests against real data. The audit
(`docs/audit-v0.md`) found its capture and setup-validation layers sound and its judgement
layer the cause of false verdicts. v0 stays untouched in its own folder; Paddock is a new
repository.

## Decision

**Reused.** Files are copied, never referenced, so v0 can keep working unchanged.

Copied in phase 0 (source: v0.3.0 working tree, 2026-10-26). The hash is the first 12 hex
digits of the SHA-256 of the v0 file as copied; line endings are normalised to LF in this
repository, so a hash of the checked-in file can differ.

| v0 path | Paddock path | SHA-256 | Notes |
| --- | --- | --- | --- |
| `ApexForge/src/core/corners.lua` | `agent/src/core/corners.lua` | `246854fc8875` | Verbatim |
| `ApexForge/src/core/recorder.lua` | `agent/src/core/recorder.lua` | `be25bac5b61f` | Verbatim |
| `ApexForge/src/core/catalog.lua` | `agent/src/core/catalog.lua` | `f966bae94374` | Verbatim |
| `ApexForge/src/core/reply.lua` | `agent/src/core/reply.lua` | `ac06bf12f6dc` | Verbatim; its rules are the reference for `core/internal/setups` |
| `ApexForge/src/core/versions.lua` | `agent/src/core/versions.lua` | `88a72386e354` | Verbatim; required by `game/setup.lua` (never overwrite a setup file) |
| `ApexForge/src/game/carlink.lua` | `agent/src/game/carlink.lua` | `f2a44f1061d1` | Verbatim for now; phase 1 adds conditions (fuel, tyre temps, wear, grip, wall clock) and removes the balance scaling |
| `ApexForge/src/game/setup.lua` | `agent/src/game/setup.lua` | `b1a751997b49` | Verbatim |
| `tests/mock_csp.lua` | `agent/tests/mock_csp.lua` | `fa9fd942f795` | Verbatim |
| `tests/harness.py` | `agent/tests/harness.py` | — | Adapted: paths, type hints; AI-line readers that need a local AC install left out until phase 1 |
| `tests/test_setup.py` (roles, display, reply, versions) | `agent/tests/test_catalog.py`, `agent/tests/test_reply.py` | — | Ported to pytest; same cases and expected values |
| `tests/fixtures/*` | `agent/tests/fixtures/*` | — | Byte-for-byte (`-text` in `.gitattributes`), except `openrouter_models_sample.json`, which belongs to the in-game LLM client that is not carried forward |

Still to port in phase 1, with their v0 tests: the synthetic driver (`sim.lua`, `core_loop.lua`)
and the restart / teleport / pit tests that use it.

**Not carried forward** (they produced false verdicts, see the audit):

- front-minus-rear slip balance as the sole diagnosis (`analyze.lua` balance, `advisor.lua` as
  an answer generator)
- the push filter (within 5% of the setup's own best)
- `learner.compare`, `outcome.evaluate`, `sensitivity.lua`
- the in-game LLM clients (`gemini.lua`, `openrouter.lua`, `game/llm.lua`, `prompt.lua`): the
  LLM moves to the engine behind a provider interface
- `game/app.lua` and `ui/main.lua` (god files); the agent's UI shrinks to a run-plan window

## Consequences

- The reused code keeps its tests: 39 agent tests run the real Lua under LuaJIT in CI.
- `carlink.lua` and `setup.lua` call the CSP API and cannot be exercised without the mock; they
  are syntax-checked now and get behaviour tests when phase 1 wires the agent together.
- The fixtures are real exports from the author's own game: setup files, and the setup-item
  catalog (names, labels, ranges) of several cars, including third-party mods. They contain no
  car assets or physics data. Review the mod authors' terms before using them in anything
  beyond tests.
