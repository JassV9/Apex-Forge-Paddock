# ADR 0001: Local-first modular monolith

Status: accepted (2026-11-11)

## Context

Paddock has one user, on one Windows PC, producing about 10 MB of telemetry per session. It
needs four kinds of work: in-game capture (must be CSP Lua), ingest and orchestration,
statistics, and a dashboard. The statistics ecosystem (numpy, statsmodels, DuckDB) is in
Python; a long-running service that tails files, owns a state machine and serves an API is a
good fit for Go.

## Decision

Three processes and one database, in one repository:

- `core` (Go): one binary with packages `ingest`, `setups`, `experiments`, `api`, `store`.
- `engine` (Python): one package for features, statistics, diagnosis, sensitivity, optimizer.
- `web` (React): a static dashboard that talks only to core.
- `agent` (CSP Lua): a thin capture-and-apply agent inside the game.

Packages inside `core` talk through Go interfaces, not over the network. The only network hop
between our own processes is core to engine, because it is also a language boundary.

## Consequences

- One `make dev`, one database, one log stream to follow.
- Module boundaries are enforced by package structure and the contracts, not by deployment.
- Four languages for one developer is the main cost. It is contained by keeping the agent thin
  and generating every shared type from `contracts/`.

## Revisit when

A module needs to scale or deploy independently, or a second team owns part of it. Neither is
plausible for a single-user local tool.
