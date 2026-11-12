# Demo script: phase 7 (public read-only demo, case study)

What phase 7 proves: the demo can be put on the internet without becoming writable, the
layout it would have on Azure runs and is tested on any machine, and the story of why this
project exists is written down with its evidence. About five minutes. Needs Docker; part 3
needs Terraform. No game, no cloud account, no API key.

**Not part of this demo: a live URL.** The Terraform has never been applied.

## 1. The replica, on this machine

```bash
make demo-replica
make demo-replica-check
```

```console
waiting for the seeded diagnosis at http://127.0.0.1:5173
ok   GET / -> 200
ok   GET /api/sessions -> 200
ok   GET /api/sessions/1/diagnosis -> 200
ok   GET /api/sensitivity -> 200
ok   GET /api/scorecard -> 200
ok   POST /api/sessions/1/diagnosis -> 403
ok   POST /api/experiments -> 403
ok   PUT /api/sessions/1 -> 403
ok   PATCH /api/sessions/1 -> 403
ok   DELETE /api/sessions/1 -> 403
ok   GET /api/metrics -> 404
the demo answers reads and refuses writes
```

Six containers took part: one copied the recorded COTA session into a volume and exited,
PostgreSQL, the engine, core and nginx share one localhost, and the seeder asked core for
the diagnosis from inside, because nobody outside can.

## 2. What a visitor sees

Open <http://localhost:5173>.

- A note under the title: "Read-only demo on a recorded session."
- No Propose link. On the session's diagnosis page, no Diagnose button.
- The diagnosis itself is the real one: understeer in slow corners, with the driver-input
  alternative marked as supported by the data.

Then try to get past the page:

```bash
curl -i -X POST http://localhost:5173/api/experiments -d '{}'
```

`403 Forbidden`, answered by nginx. Core's log has no line for it.

```bash
docker compose -f deploy/demo/docker-compose.replica.yml logs core | grep -c experiments
make demo-replica-down
```

## 3. The same layout as Terraform

```bash
cd infra/azure
terraform init -backend=false
terraform validate
```

```console
Success! The configuration is valid.
```

`main.tf` and `deploy/demo/docker-compose.replica.yml` describe the same five containers
with the same environment. Read them side by side. How to apply it, and what that would
cost: [docs/runbooks/deploy-demo.md](../runbooks/deploy-demo.md).

## 4. The case study

[docs/case-study-v0-regressions.md](../case-study-v0-regressions.md): how nine setup
versions made the car slower, the three findings with their numbers, what each became in
the rebuild and where it is tested, and what none of this shows.

```bash
python tools/verdict_noise_sim.py
```

## What was checked, and what was not

| Claim | Checked by |
| --- | --- |
| Writes are refused before they reach core | `make demo-replica-check`, locally and in CI on every push |
| The replica layout works: shared localhost, no start order, restarts | The same check; core is restarted twice before PostgreSQL is up |
| The pages hide what a visitor cannot do | `web/src/App.test.tsx`, `DiagnosisPage.test.tsx` |
| The Terraform is well formed | `terraform fmt -check`, `terraform validate`, in CI |
| It runs on Azure | **Not checked.** Never applied |
| The publish workflow pushes working images | **Not checked.** Never started |
| Anything in Assetto Corsa | **Not checked** |
