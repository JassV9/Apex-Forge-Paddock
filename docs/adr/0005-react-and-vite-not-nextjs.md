# ADR 0005: React with Vite, not Next.js

Status: accepted (2026-11-11)

## Context

The dashboard runs on the same PC as its API, for one user. It shows data that core already
serves as JSON. The public demo (phase 7) is read-only and serves recorded data.

## Decision

A client-rendered React app built with Vite, in TypeScript strict mode. It talks only to core's
REST API through a typed client generated from `docs/api/openapi.yaml` (`openapi-typescript`
for the types, `openapi-fetch` as a small typed wrapper around `fetch`). The dev server proxies
`/api` to core, so the browser never needs core's address and there is no CORS configuration.

No business logic in the client: it renders what core returns.

## Consequences

- The build output is static files; any file server or CDN can host the demo.
- No Node.js server to run, secure or keep alive next to core.
- Tests: Vitest with Testing Library for components. Playwright smoke flows are added when the
  first real pages exist (phase 3); there is nothing to smoke-test in phase 0.

## Why not Next.js

Server rendering, routing conventions and server actions solve problems this app does not
have: no SEO, no slow first load over a network, no server-side secrets in the web tier. It
would add a second backend to a system that already has two.

## Revisit when

The dashboard becomes a public, content-heavy site, which is not planned.
