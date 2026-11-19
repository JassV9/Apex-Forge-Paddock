"""Real AI experiments 7, 11, 12 and 15 to 19 (Formula Hybrid 2026, Hungaroring), as core sends
them.

fixtures/real_experiments.csv.gz holds every push-lap pass of 11, 12 and 15 to 19, exported
read-only from the ledger on 2026-11-18; fixtures/rear_arb_7.csv.gz the same columns for
experiment 7 (rear ARB +1, four blocks, the first of the four pooled rear ARB runs, ADR 0033).
`requests` builds the evaluate request the way core/internal/experiments/verdict.go does since
ADR 0029: valid passes on the arm's setup, laps
coded per block in the order they were driven, time since the experiment's first push pass, a
session term per game session after the first, and off-track passes counted per block.
"""

import csv
import gzip
from pathlib import Path
from typing import Any

FIXTURES = [
    Path(__file__).parent / "fixtures" / name
    for name in ("real_experiments.csv.gz", "rear_arb_7.csv.gz")
]
EXPERIMENTS = (11, 12, 15, 16, 17, 18, 19)
METRICS = (
    "corner_time_s",
    "rear_slip_peak_deg",
    "countersteer_s",
    "brake_dist_m",
    "throttle_pickup_m",
    "steer_corrections_per_s",
)
TEST = {"channel": "corner_time_s", "rope": 0.005, "level": 0.9, "better": "lower"}
GUARDRAILS = [
    {"channel": "rear_slip_peak_deg", "tolerance": 0.5, "worse": "higher"},
    {"channel": "countersteer_s", "tolerance": 0.05, "worse": "higher"},
]
DRIVER_CHANNELS = ["brake_dist_m", "throttle_pickup_m", "steer_corrections_per_s"]


def rows(experiment: int) -> list[dict[str, str]]:
    out = []
    for fixture in FIXTURES:
        with gzip.open(fixture, "rt", encoding="utf-8", newline="") as handle:
            out += [row for row in csv.DictReader(handle) if int(row["experiment"]) == experiment]
    return out


def _number(text: str) -> float | None:
    return float(text) if text else None


def _covariates(row: dict[str, str], start: float, sessions: list[str]) -> dict[str, float]:
    covariates = {"time_since_start_s": float(row["t_start"]) - start}
    for name in ("fuel_l", "tyre_core_c", "ers_charge_start"):
        if row[name]:
            covariates[name] = float(row[name])
    for index, session in enumerate(sessions[1:], start=2):
        covariates[f"session_{index}"] = float(row["session"] == session)
    return covariates


def passes(experiment: int) -> list[dict[str, Any]]:
    """The valid passes on the arm's setup, as EvaluatePass dicts."""
    driven = [r for r in rows(experiment) if r["valid"] == "t" and r["on_arm_setup"] == "t"]
    sessions = list(dict.fromkeys(r["session"] for r in driven))
    start = min(float(r["t_start"]) for r in driven)
    codes: dict[tuple[str, str, str], int] = {}
    out = []
    for row in driven:
        key = (row["block"], row["session"], row["block_lap"])
        block = int(row["block"])
        codes.setdefault(key, block * 1000 + 1 + sum(k[0] == row["block"] for k in codes))
        out.append(
            {
                "corner": int(row["corner"]),
                "arm": "baseline" if row["baseline"] == "t" else "variant",
                "block": block,
                "lap": codes[key],
                "speed_class": row["speed_class"] or None,
                "metrics": {name: _number(row[name]) for name in METRICS},
                "covariates": _covariates(row, start, sessions),
            }
        )
    return out


def offtrack(experiment: int) -> list[dict[str, Any]]:
    counts: dict[int, dict[str, Any]] = {}
    for row in rows(experiment):
        arm = "baseline" if row["baseline"] == "t" else "variant"
        block = counts.setdefault(
            int(row["block"]), {"block": int(row["block"]), "arm": arm, "passes": 0, "offtrack": 0}
        )
        block["passes"] += 1
        block["offtrack"] += row["valid"] != "t"
    return [counts[block] for block in sorted(counts)]


def request(experiment: int, **overrides: Any) -> dict[str, Any]:
    """The evaluate request of a real experiment, judged at the look its blocks make."""
    return {
        "success_test": TEST,
        "guardrails": GUARDRAILS,
        "predictions": [],
        "driver_channels": DRIVER_CHANNELS,
        "passes": passes(experiment),
        "excluded": {"setup_mismatch": 0, "source_mismatch": 0},
        "offtrack": offtrack(experiment),
    } | overrides
