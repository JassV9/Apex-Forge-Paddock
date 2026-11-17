"""Hybrid battery state per pass (docs/adr/0024-ers-state-per-pass.md): a car with KERS/ERS
records its charge at the start and end of every pass and its mean deployment; a car without
one records nothing about it.

Offline, on the mocked CSP API: nothing here shows what CSP reports for a real hybrid.
"""

from typing import Any

import jsonschema

from conftest import Agent
from harness import to_lua, to_py
from test_agent import drive_laps
from test_spool_format import pass_meta, schema, validate


def raw_pass_with_battery(rt: Any, charge: list[float], deploy: list[float]) -> Any:
    count = len(charge)
    names = to_py(rt.eval('require("core.channels")').names)
    cols = {name: [0.0] * count for name in names}
    cols["t"] = [100.0 + i / 60 for i in range(count)]
    cols["kers_charge"], cols["kers_input"] = charge, deploy
    return to_lua(rt, {"corner": {"id": 4}, "n": count, "cols": cols, "tIn": 100.0, "tOut": 100.05})


def test_the_battery_is_summarised_not_written_per_frame(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    raw = raw_pass_with_battery(rt, [0.81234, 0.8, 0.77], [0.0, 1.0, 1.0])

    record = validate(lines.finish(lines.beginPass(raw, pass_meta(rt, ers=True))), "spool_pass")

    assert record["ers"] == {"charge_start": 0.8123, "charge_end": 0.77, "deploy_mean": 0.6667}
    assert not {"kers_charge", "kers_input"} & set(record["samples"]["columns"])


def test_no_battery_no_ers_object(rt: Any) -> None:
    lines = rt.eval('require("core.spool_lines")')
    raw = raw_pass_with_battery(rt, [0.5, 0.5, 0.5], [0.0, 0.0, 0.0])

    record = validate(lines.finish(lines.beginPass(raw, pass_meta(rt))), "spool_pass")

    assert "ers" not in record


def test_a_hybrid_records_its_battery_on_every_pass(agent: Agent) -> None:
    agent.mock["car"]["kersPresent"] = True
    agent.mock["car"]["kersCharge"] = 0.6
    drive_laps(agent, 2)

    passes = agent.passes()
    for record in passes:
        jsonschema.Draft7Validator(schema("spool_pass")).validate(record)
    batteries = [record["ers"] for record in passes]
    assert len(batteries) == len(passes) > 0
    assert all(0.0 <= b["charge_start"] <= 1.0 and 0.0 <= b["charge_end"] <= 1.0 for b in batteries)
    assert any(b["deploy_mean"] > 0 for b in batteries)
    assert any(b["charge_end"] != b["charge_start"] for b in batteries)
    # read as each pass starts, not once per lap or session
    starts = [b["charge_start"] for b in batteries]
    assert len(set(starts)) == len(starts)


def test_a_car_without_a_battery_records_nothing_about_it(agent: Agent) -> None:
    drive_laps(agent, 1)

    passes = agent.passes()
    assert passes
    assert all("ers" not in record for record in passes)
