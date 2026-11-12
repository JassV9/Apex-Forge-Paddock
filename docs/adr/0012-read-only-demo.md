# ADR 0012: A public demo that can only be read, as one disposable replica

Status: accepted (2026-11-12). The Terraform is validated and has never been applied.

## Context

Phase 7 asks for the demo mode to be public and read-only on Azure Container Apps, described
in Terraform. Until now every process assumed one trusted user on one PC: core's API has no
authentication, any caller can propose an experiment, and `POST /sessions/{id}/diagnosis`
makes the engine work. None of that may be reachable from the internet.

Two constraints shape the answer. The demo must cost close to nothing while nobody looks at
it. And this project does not get to claim a deployment that was not made: no cloud account
is used from this repository, so whatever is written has to be checkable without one.

## Decision

**Read-only is enforced at the door, not in the pages.** The public demo serves the
dashboard as static files from nginx, and nginx proxies `/api/` to core with
`limit_except GET { deny all; }`. A request that would change anything is answered 403 and
never reaches core. `/api/metrics` answers 404, and the API is rate limited, because one
read does real work: the sensitivity profile is rebuilt on every `GET /sensitivity`
([ADR 0011](0011-sensitivity-and-ranking.md)).

The dashboard is built with `VITE_READ_ONLY=1`, which hides the Propose link and the
Diagnose button and says why. That is courtesy. A visitor who edits the page gains nothing,
because the rule is in nginx. Core itself is unchanged: no "demo mode" branch exists in the
Go or Python code, so the demo cannot drift from the product.

**One replica, containers that share localhost.** The demo is one container app with five
containers: PostgreSQL, the engine, core, nginx with the dashboard, and a seeder. Only
nginx's port has ingress. An init container copies the recorded session from an image into
an empty volume, because the platform has no folder to mount. Core ingests it as it would
ingest the game's spool.

**Nothing is kept.** The database lives in the replica and dies with it. A new replica
ingests the session again and the seeder diagnoses it from inside, which takes about a
minute. There is no state to back up, migrate or leak, the maximum is one replica because
each has its own database, and the minimum can be zero, so an idle demo costs nothing.

**The layout is rehearsed locally and in CI.** `deploy/demo/docker-compose.replica.yml` is
the same replica in Compose: every service joins one network namespace, nothing waits for
anything else, and a container that exits is restarted. `make demo-replica-check`
(`deploy/demo/smoke.sh`) then asks from outside: reads 200, `POST`, `PUT`, `PATCH` and
`DELETE` 403, metrics 404. CI runs both on every push, and runs `terraform fmt -check` and
`terraform validate`.

**Images are published by hand.** `.github/workflows/publish-demo.yml` builds the four
images and pushes them to GitHub's registry only when someone starts it. A push to `main`
publishes nothing.

New dependencies: nginx (`nginxinc/nginx-unprivileged`, runs without root), Terraform with
the `azurerm` and `random` providers (versions pinned in `.terraform.lock.hcl`), `busybox`
for the dataset image and `curlimages/curl` for the seeder.

## Alternatives considered

- **A read-only flag in core.** One more code path that exists only for the demo, and one
  missed handler makes the demo writable. A proxy rule covers every route, including the
  ones written later.
- **Authentication.** The demo has no users. Adding accounts to hide a button is the kind
  of scope [ADR 0006](0006-no-kafka-no-kubernetes.md) argues against.
- **A managed PostgreSQL and a container app per process.** The usual shape, and the right
  one for data that matters. Here it would add a database that costs money all month to hold
  one recorded session, and private networking between apps to protect it.
- **A static export of the pages.** Cheapest of all, but it would show screenshots of the
  product instead of the product: the point of the demo is that the real core and engine
  answer.

## Consequences

- Core starts before PostgreSQL accepts connections, exits, and is restarted. In the local
  rehearsal that is two restarts in under a second. On Azure the restart back-off may add
  tens of seconds to a cold start; that is not measured.
- Behind the platform's proxy all visitors may arrive from one address, so the rate limit
  can throttle everyone at once. It still protects core, which is its job.
- The demo shows a recorded session and its diagnosis. The ledger and the Garage are empty
  there, because no experiment was ever driven on that car. The populated pages in the
  README come from the end-to-end test's simulated car and are labelled as such.
- Changing the limits means changing nginx's template; they are not configurable at run
  time.

## Not verified

- **Nothing was deployed.** `terraform validate` checks the configuration against the
  provider's schema. It does not prove that Azure accepts it: the size of the replica (2
  vCPU and 4 Gi in total), the `EmptyDir` volumes shared with an init container, and
  pulling `postgres` and `curl` from Docker Hub are all as documented, not as observed.
- Scale to zero and the time of a cold start on Azure.
- The publish workflow has not been run, so no image exists in the registry.
