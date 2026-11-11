# Changelog

One short entry per phase. Dates are when the phase was finished, not accepted.

## Phase 0: skeleton (2026-11-11)

- Repository layout: `agent/`, `core/`, `engine/`, `web/`, `contracts/`, `deploy/`, `docs/`,
  `tools/`.
- `contracts/health.schema.json` with generated Go structs, Pydantic models and TypeScript
  types; `make gen` and a drift check (`make gen-check`).
- core (Go): `GET /health`, environment configuration, PostgreSQL connection, embedded goose
  migrations, first migration (`jobs`).
- engine (Python): FastAPI app with `GET /health`, `mypy --strict`.
- web (React + Vite + TypeScript): a page that reports whether core is reachable, through a
  typed client generated from `docs/api/openapi.yaml`.
- agent: reusable v0 Lua modules, the mock CSP API and the real fixtures copied in
  (ADR 0002); 39 offline tests under LuaJIT, including a check that pure modules make no CSP
  calls.
- Docker Compose stack, Makefile, GitHub Actions CI (generated-code drift, Go, Python, Lua,
  TypeScript, dependency audits, secret scan), pre-commit.
- Docs: architecture, v0 audit, ADRs 0001 to 0006, onboarding, local-dev runbook, demo script.

`make dev` and the Docker images were verified on Docker Desktop after the first commit.
Not verified: anything in Assetto Corsa.
