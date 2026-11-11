"""Spool lines and the outgoing queue: what the agent writes must match the contracts."""

import json
from pathlib import Path
from typing import Any

import jsonschema
import pytest

from harness import AGENT_DIR, to_lua, to_py

CONTRACTS = AGENT_DIR.parent / "contracts"


def schema(name: str) -> dict[str, Any]:
    return json.loads((CONTRACTS / f"{name}.schema.json").read_text(encoding="utf-8"))


def validate(line: str, name: str) -> dict[str, Any]:
    record = json.loads(line)
    jsonschema.Draft7Validator(schema(name)).validate(record)
    return record


def raw_pass(rt: Any, count: int = 3) -> Any:
    names = to_py(rt.eval('require("core.channels")').names)
    cols = {name: [float(i) + 0.123456 for i in range(count)] for name in names}
    cols["t"] = [100.0 + i / 60 for i in range(count)]
    return to_lua(rt, {"corner": {"id": 4}, "n": count, "cols": cols, "tIn": 100.0, "tOut": 100.05})


CONDITIONS = {
    "fuel": 41.257,
    "core": [88.04, 89.0, 91.5, 92.25],
    "surface": [80.0, 81.0, 82.0, 83.0],
    "pressure": [26.512, 26.5, 25.9, 26.0],
    "wear": [0.01, 0.02, 0.03, 0.04],
    "grip": 0.97,
    "air": 24.0,
    "road": 31.0,
}


def pass_meta(rt: Any, **overrides: Any) -> Any:
    meta = {"sessionId": "s-1", "seq": 7, "startMs": 1793037003123, "lap": 2, "setupHash": "sabc"}
    return to_lua(rt, {**meta, "conditions": CONDITIONS, **overrides})


def test_channel_names_are_the_contract_columns(rt: Any) -> None:
    names = to_py(rt.eval('require("core.channels")').names)
    columns = schema("spool_pass")["properties"]["samples"]["properties"]["columns"]

    assert names == columns["required"]
    assert set(names) == set(columns["properties"])


def test_pass_line_matches_the_contract(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')

    record = validate(lines.finish(lines.beginPass(raw_pass(rt), pass_meta(rt))), "spool_pass")

    assert record["t_start_ms"] == 1793037003123  # an integer, never 1.79e+12
    assert record["duration_s"] == 0.05
    assert record["conditions"]["tyre_core_c"] == [88.0, 89.0, 91.5, 92.3]
    assert record["samples"]["n"] == 3
    assert record["samples"]["columns"]["t"] == [0.0, 0.0167, 0.0333]  # relative to pass start
    assert record["samples"]["columns"]["load_fl"] == [0.0, 1.0, 2.0]  # rounded to whole newtons


def test_pass_is_encoded_one_column_per_step(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    column_count = len(to_py(rt.eval('require("core.channels")').names))
    job = lines.beginPass(raw_pass(rt), pass_meta(rt))

    steps_without_a_line = 0
    while lines.step(job) is None:
        steps_without_a_line += 1

    assert steps_without_a_line == column_count


def test_optional_fields_are_left_out_when_unknown(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    meta = to_lua(rt, {"sessionId": "s-1", "seq": 1, "startMs": 5})

    record = validate(lines.finish(lines.beginPass(raw_pass(rt), meta)), "spool_pass")

    assert not {"lap_in_session", "setup_hash", "conditions"} & set(record)


def test_non_finite_samples_become_zero(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    raw = raw_pass(rt)
    raw["cols"]["yaw"][2] = float("nan")
    raw["cols"]["v"][1] = float("inf")

    record = validate(lines.finish(lines.beginPass(raw, pass_meta(rt))), "spool_pass")

    assert record["samples"]["columns"]["yaw"][1] == 0
    assert record["samples"]["columns"]["v"][0] == 0


def test_session_line_matches_the_contract_and_escapes_names(rt: Any, corner_map: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    meta = {
        "sessionId": "20261111T120000-car",
        "startedAtMs": 1793037003000,
        "source": "human",
        "agentVersion": "0.1.0",
        "carId": "some_car",
        "carName": 'Car "GT" \\ special\tedition',
        "trackId": "synthetic",
        "layout": "",
        "trackName": "Synthetic",
        "mapVersion": 1,
    }

    record = validate(lines.session(to_lua(rt, meta), corner_map), "spool_session")

    assert record["car"]["name"] == meta["carName"]
    assert [corner["number"] for corner in record["track"]["corners"]] == list(range(1, 8))
    assert {corner["direction"] for corner in record["track"]["corners"]} == {"L", "R"}


@pytest.mark.parametrize(
    ("times", "expected_writes"),
    [([0.0, 0.1, 0.19, 0.2, 0.3, 0.41], 3), ([0.0, 5.0], 2)],
)
def test_queue_writes_at_most_five_times_per_second(
    rt: Any, times: list[float], expected_writes: int
) -> None:
    queue = rt.eval('require("core.spool")').new()
    written = []
    for now in times:
        queue.add(queue, f'{{"at":{now}}}')
        name, text = queue.take(queue, now) or (None, None)
        if name:
            written.append((name, text))

    assert len(written) == expected_writes
    assert [name for name, _ in written] == [f"{i:06d}.jsonl" for i in range(1, len(written) + 1)]
    assert all(text.endswith("\n") for _, text in written)


def test_queue_keeps_waiting_lines_together_and_flushes_on_demand(rt: Any) -> None:
    queue = rt.eval('require("core.spool")').new()
    queue.add(queue, "a")
    assert queue.take(queue, 0.0) == ("000001.jsonl", "a\n")
    queue.add(queue, "b")
    queue.add(queue, "c")

    assert queue.take(queue, 0.05) is None  # too soon
    assert queue.take(queue, 0.05, True) == ("000002.jsonl", "b\nc\n")
    assert queue.take(queue, 9.0) is None  # nothing left


def test_contract_files_exist() -> None:
    assert Path(CONTRACTS / "spool_pass.schema.json").is_file()
