# ADR 0006: No Kafka, no Kubernetes, no microservices

Status: accepted (2026-11-11)

## Context

This is a portfolio project, and it is tempting to add infrastructure because it looks good on
a CV. The actual load is one user, one PC, a few corner passes per second at most, and a
handful of analysis jobs per session.

## Decision

- **No message broker.** Core hands long-running work to the engine through a `jobs` table in
  PostgreSQL. Workers claim rows with `SELECT ... FOR UPDATE SKIP LOCKED`, which gives
  at-least-once delivery, retries (`attempts`) and a visible queue with the tools already here.
- **No Kubernetes at runtime.** Local development is Docker Compose. The public demo is one
  stateless, read-only deployment on Azure Container Apps through Terraform (phase 7).
- **No microservices.** See [ADR 0001](0001-modular-monolith.md).
- **Rust** is allowed only in `ghost/`, and only after the Go controller loop is measured to
  exceed its jitter budget.
- **No cloud dependency** for local use.

## Consequences

- Nothing to operate besides PostgreSQL.
- Job throughput is limited by polling one table, which is far above what one driver produces.
- The queue is transactional with the data it refers to: a job and the experiment state change
  that created it commit together.

## When each would be justified

| Technology | Would be justified by |
| --- | --- |
| Kafka or another broker | Many producers and independent consumers, replay of an ordered event log, or throughput beyond what one PostgreSQL table handles (thousands of jobs per second) |
| Kubernetes | Several services that scale independently, multi-node scheduling, or a team that already operates a cluster |
| Microservices | Separate teams that need to deploy on their own schedules |

None of these applies to a single-user local tool.
