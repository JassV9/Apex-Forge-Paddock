# datasets/demo

Recorded sessions for demo mode (`make demo`), so the dashboard can be explored without
Assetto Corsa. The folder `spool/` has the same layout the in-game agent writes; core ingests
it exactly as it ingests a live session.

## spool/demo-fa26-cota

The real session behind the v0 audit: VRC Formula Alpha 2026 at Circuit of the Americas,
26 October 2026, one setup, **136 corner passes over 17 corners** (8 per corner), recorded by
Apex Forge v0.3.0.

It is converted from `agent/tests/fixtures/real_fa26_cota_log.json` and `real_cota_map.json`
by `tools/export_demo_dataset.py`. Regenerate with:

    python tools/export_demo_dataset.py

What this dataset does **not** contain, because v0 never recorded it:

| Missing | Consequence |
| --- | --- |
| Raw samples | Each pass carries v0's per-phase averages (`legacy`) instead of `samples`; no Parquet is written for it |
| Conditions (fuel, tyre temperatures, wear, grip) | The condition columns are empty. This is finding 5 of `docs/audit-v0.md` |
| A time per pass | `t_start_ms` is synthetic: the session start plus the driving time of earlier passes. The order is real; the gaps are not |

v0's own judgements (push or not, the balance number, verdicts) are deliberately left out.
