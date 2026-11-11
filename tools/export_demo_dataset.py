"""Converts the real Apex Forge v0 session log into a Paddock spool, for demo mode and tests.

    python tools/export_demo_dataset.py

Source: agent/tests/fixtures/real_fa26_cota_log.json (136 corner passes, FA26 at COTA, one
setup) and real_cota_map.json (the corner map recorded in game).
Output: datasets/demo/spool/demo-fa26-cota/000001.jsonl

What the source does not contain, and what this script does about it:
- No raw samples: v0 kept per-phase averages only. They are carried as the pass's `legacy`
  summary; there is no `samples` object.
- No conditions (fuel, tyres, grip): left out. This is finding 5 of docs/audit-v0.md.
- No time per pass: v0 logged passes in order without a clock. `t_start_ms` is synthetic: the
  session start plus the driving time of the passes before it. The order is real, the gaps
  between passes are not.
v0's own judgements (push or not, balance, verdicts) are deliberately not carried over.
"""

import json
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
FIXTURES = ROOT / "agent/tests/fixtures"
OUTPUT = ROOT / "datasets/demo/spool/demo-fa26-cota/000001.jsonl"
SESSION_ID = "demo-fa26-cota"
SOURCE = "apexforge-0.3.0"

# v0 phase field -> contract field
PHASE_FIELDS = {
    "dur": "duration_s",
    "cov": "coverage",
    "aF": "front_slip_deg",
    "aR": "rear_slip_deg",
    "aFx": "front_slip_max_deg",
    "aRx": "rear_slip_max_deg",
    "lat": "lat_g",
    "yaw": "yaw_rate",
    "srate": "steer_rate",
    "gas": "gas",
    "brk": "brake",
}


def load(name: str) -> Any:
    return json.loads((FIXTURES / name).read_text(encoding="utf-8"))


def session_line(log: dict[str, Any], corner_map: dict[str, Any]) -> dict[str, Any]:
    track_id, layout = log["track"].split("__")
    corners = [
        {
            "number": c["id"],
            "name": c["name"],
            "direction": c["dir"],
            "apex_m": round(c["apex"], 2),
            "radius_m": round(c["radius"], 2),
            "angle_deg": round(c["angle"], 1),
            "zone_start_m": round(c["zStart"], 2),
            "zone_end_m": round(c["zEnd"], 2),
        }
        for c in corner_map["list"]
    ]
    return {
        "kind": "session",
        "v": 1,
        "session_id": SESSION_ID,
        "started_at_ms": log["started"] * 1000,
        "source": "demo",
        "agent_version": f"{SOURCE}-import",
        "car": {"ac_id": "vrc_formula_alpha_2026_csp", "name": "VRC Formula Alpha 2026"},
        "track": {
            "ac_id": track_id,
            "layout": layout,
            "name": "Circuit of the Americas",
            "length_m": round(corner_map["L"], 2),
            "corner_map_version": corner_map["v"],
            "corners": corners,
        },
    }


def legacy_summary(v0_pass: dict[str, Any]) -> dict[str, Any]:
    phases = {}
    for phase, values in v0_pass.get("diag", {}).items():
        summary = {"confidence": v0_pass["conf"][phase]}
        summary.update({ours: values[theirs] for theirs, ours in PHASE_FIELDS.items()})
        phases[phase] = summary
    return {
        "source": SOURCE,
        "v_min_kmh": v0_pass["vMin"],
        "off_track": bool(v0_pass.get("off", False)),
        "phases": phases,
    }


def pass_lines(log: dict[str, Any]) -> list[dict[str, Any]]:
    clock_ms = log["started"] * 1000
    lines = []
    for seq, v0_pass in enumerate(log["passes"], start=1):
        lines.append(
            {
                "kind": "pass",
                "v": 1,
                "session_id": SESSION_ID,
                "seq": seq,
                "corner": v0_pass["c"],
                "t_start_ms": clock_ms,
                "duration_s": v0_pass["t"],
                "setup_hash": v0_pass["ver"],
                "legacy": legacy_summary(v0_pass),
            }
        )
        clock_ms += round(v0_pass["t"] * 1000)
    return lines


def main() -> None:
    log, corner_map = load("real_fa26_cota_log.json"), load("real_cota_map.json")
    lines = [session_line(log, corner_map), *pass_lines(log)]
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    text = "".join(json.dumps(line, separators=(",", ":")) + "\n" for line in lines)
    OUTPUT.write_text(text, encoding="utf-8", newline="\n")
    print(f"wrote {OUTPUT.relative_to(ROOT)}: 1 session line, {len(lines) - 1} passes")


if __name__ == "__main__":
    main()
