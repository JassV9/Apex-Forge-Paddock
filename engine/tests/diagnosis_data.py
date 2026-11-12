"""Requests for the diagnosis and explain tests: the real COTA session and small made-up ones."""

import json
from pathlib import Path

from paddock_engine.contracts.diagnose_request_schema import (
    DiagnoseItem,
    DiagnosePass,
    DiagnoseRequest,
    SpeedClass,
)
from paddock_engine.features.pass_features import from_legacy

REPO = Path(__file__).resolve().parents[2]
DEMO_SPOOL = REPO / "datasets/demo/spool/demo-fa26-cota/000001.jsonl"
CONFIDENT = {"confidence_entry": 1.0, "confidence_mid": 1.0, "confidence_exit": 1.0}


def cota_request(items: list[DiagnoseItem] | None = None) -> DiagnoseRequest:
    """The valid passes of the v0 session at COTA (Ferrari F2026), as core would send them."""
    lines = [json.loads(line) for line in DEMO_SPOOL.read_text(encoding="utf-8").splitlines()]
    passes = []
    for line in lines[1:]:
        features = from_legacy(line["legacy"], line["duration_s"])
        if features.valid:
            passes.append(
                DiagnosePass(
                    corner=line["corner"],
                    speed_class=SpeedClass(features.speed_class),
                    metrics=features.metrics,
                )
            )
    return DiagnoseRequest(passes=passes, items=items or [])


def passes_of(speed_class: str, count: int = 4, **metrics: float | None) -> list[DiagnosePass]:
    """`count` identical passes of one corner class, confident in every phase."""
    return [
        DiagnosePass(
            corner=1, speed_class=SpeedClass(speed_class), metrics={**CONFIDENT, **metrics}
        )
        for _ in range(count)
    ]


def item(name: str, role: str, axle: str = "", **changes: float | str) -> DiagnoseItem:
    fields: dict[str, float | str] = {
        "name": name,
        "label": name.replace("_", " ").title(),
        "role": role,
        "axle": axle,
        "shown_scale": 1.0,
        "min": 0.0,
        "max": 10.0,
        "step": 1.0,
        "value": 5.0,
    }
    return DiagnoseItem.model_validate({**fields, **changes})


def car() -> list[DiagnoseItem]:
    return [
        item("ARB_FRONT", "arb", "F"),
        item("ARB_REAR", "arb", "R"),
        item("WING_1", "wing", "F"),
        item("WING_2", "wing", "R"),
        item("BRAKE_BIAS", "bias", min=50.0, max=70.0, value=60.0),
        item("CAMBER_LF", "camber", "F"),
        item("CAMBER_RF", "camber", "F"),
        item("SPRING_RATE_F", "spring", "F", value=0.0),
    ]
