"""Run context and AI provenance (docs/adr/0028-run-context-and-ai-provenance.md): the session
line says what the AI was set to and which agent build recorded it, every pass says which hint
the AI drove with, and the new CSP channels reach the spool.

Offline, on the mocked CSP API: nothing here shows what CSP reports in Assetto Corsa.
"""

import json
import time
from typing import Any

import jsonschema

from conftest import Agent, boot_agent
from harness import TESTS_DIR, runtime, to_lua, to_py
from test_agent import drive_laps
from test_ghost import let_ai_load, physics_calls
from test_plan_agent import LAPS, setups, stint, write_plan
from test_spool_format import schema

NEW_COLUMNS = [
    *(f"{c}_{w}" for c in ("nd", "w", "st") for w in ("fl", "fr", "rl", "rr")),
    *("vx", "vz", "x", "z", "gear", "rpm"),
]


def sessions(agent: Agent) -> list[dict[str, Any]]:
    return [line for line in agent.lines() if line["kind"] == "session"]


def test_a_human_session_line_says_what_the_ai_would_drive_with(agent: Agent) -> None:
    drive_laps(agent, 1)

    line = sessions(agent)[0]
    jsonschema.Draft7Validator(schema("spool_session")).validate(line)
    assert line["agent_version"] == "0.2.0+dev", "a copy the installer did not write is dev"
    context = line["context"]
    assert context["cars_count"] == 1
    assert context["csp_version"] == "0.3.0-preview581" and context["csp_build"] == 3400
    assert context["ai"] == {
        "tyre_hint_base": 1,
        "learned_share": 1,
        "level": 1,
        "aggression": 0.5,
    }, "no hint applied, no extra grip: the AI did not drive"


def test_the_session_line_names_the_installed_build_and_the_cars_ai_hints(
    rt: Any, track: Any, tmp_path: Any
) -> None:
    rt.execute('package.loaded.build = { hash = "a1b2c3d4e5f6" }')
    agent = boot_agent(rt, track, tmp_path)
    agent.mock["aiIni"] = agent.rt.table_from(
        {"PHYSICS_HINTS/AERO_HINT": 1.4, "PEDALS/BRAKE_HINT": 0.97}
    )
    agent.mock["physicsAllowed"] = True
    agent.rt.eval('require("game.ghost")').lap()
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    stint(agent, 1)

    ai_line = next(line for line in sessions(agent) if line["source"] == "ai")
    jsonschema.Draft7Validator(schema("spool_session")).validate(ai_line)
    assert ai_line["agent_version"] == "0.2.0+a1b2c3d4e5f6"
    ai = ai_line["context"]["ai"]
    assert ai["aero_hint"] == 1.4 and ai["brake_hint"] == 0.97
    assert ai["applied_hint"] == 1 and ai["extra_grip"] == 1
    assert "pinned_share" not in ai, "free laps that learn pin nothing"


def test_every_pass_carries_its_context_and_the_new_channels(agent: Agent) -> None:
    drive_laps(agent, 2)

    passes = agent.passes()
    assert passes
    for record in passes:
        jsonschema.Draft7Validator(schema("spool_pass")).validate(record)
        columns, n = record["samples"]["columns"], record["samples"]["n"]
        assert all(len(columns[name]) == n for name in NEW_COLUMNS)
        context = record["context"]
        assert context["clock"] == "physics" and context["resets"] == 0
        assert context["damage_max"] == 0 and "ai_hint" not in context, "a human drove"
        assert 0 <= context["abs_share"] <= 1 and context["tc_share"] == 0
    assert any(record["context"]["abs_share"] > 0 for record in passes)
    first = passes[0]["samples"]["columns"]
    assert len(set(first["x"])) > 1 and set(first["gear"]) == {3}
    assert all(w > 0 for w in first["w_fl"]), "wheels turn with the car"


def test_samples_are_stamped_with_physics_time_and_fall_back_to_the_app_clock(rt: Any) -> None:
    rt.execute(f'package.path = "{TESTS_DIR.as_posix()}/?.lua;" .. package.path')
    carlink = rt.eval('require("game.carlink")')
    car = rt.eval(
        "{ timestamp = 5000, wheels = { [0] = {}, {}, {}, {} }, rideHeight = { [0] = 0, 0 },"
        " acceleration = { x = 0, z = 0 }, localAngularVelocity = { y = 0 }, splinePosition = 0,"
        " speedMs = 0, gas = 0, brake = 0, steer = 0, wheelsOutside = 0 }"
    )

    assert carlink.chooseClock(car) == "physics"
    assert carlink.read(car, 1000, 1.0).t == 5.0, "CSP's physics time, in seconds"
    car.timestamp = 5016
    assert carlink.read(car, 1000, 1.016).t == 5.016
    assert carlink.read(car, 1000, 1.6).t == 1.6, "physics time that stands still is not a clock"
    assert carlink.clock == "app"
    car.timestamp = None
    assert carlink.chooseClock(car) == "app"


def pinned_plan(agent: Agent, share: float) -> dict[str, float]:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    agent.mock["editable"] = True
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    plans = agent.spool_root.parent / "outbox" / "runplans" / "ks_mazda_mx5_cup.json"
    plan = json.loads(plans.read_text(encoding="utf-8")) | {"ai_hint_share": share}
    jsonschema.Draft7Validator(schema("runplan")).validate(plan)
    plans.write_text(json.dumps(plan), encoding="utf-8")
    return base


def write_calibration(agent: Agent, hint: float) -> Any:
    folder = agent.spool_root.parent / "ai_tune"
    folder.mkdir(parents=True, exist_ok=True)
    path = folder / "ks_mazda_mx5_cup__synthetic__.json"
    saved = {
        "v": 2,
        "sections": 200,
        "hint": hint,
        "clean_laps": 2,
        "laps": 6,
        "last_incidents": [],
    }
    path.write_text(json.dumps(saved), encoding="utf-8")
    return path


def test_a_pinned_hint_is_driven_instead_of_the_learned_one_and_never_saved(agent: Agent) -> None:
    calibration = write_calibration(agent, 0.885)
    before = calibration.read_text(encoding="utf-8")
    agent.mock["aiTyresHint"] = 1.1
    pinned_plan(agent, 0.93)
    off = agent.rt.eval("function(d) return d > 300 and d < 330 end")
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    agent.drive.run(agent.rt.table_from({"laps": 1 + LAPS, "off": off}))

    hints = [args[0] for name, args in physics_calls(agent) if name == "tyres_hint"]
    assert abs(hints[-1] - 1.1 * 0.93) < 1e-9, "the plan's share of the car's own hint"
    assert abs(1.1 * 0.885 - 1.1 * 0.93) > 0.01 and all(abs(h - 1.1 * 0.885) > 1e-9 for h in hints)
    assert calibration.read_text(encoding="utf-8") == before, "the ai_tune file is not touched"
    state = to_py(agent.rt.eval('require("game.calibration").state'))
    assert state["hint"] == 0.885 and state["laps"] == 6, "nothing is learned or counted"
    blocks = [p for p in agent.passes() if "block" in p]
    assert blocks and {p["context"]["ai_hint"] for p in blocks} == {round(1.1 * 0.93, 4)}
    ai_line = next(line for line in sessions(agent) if line["source"] == "ai")
    assert ai_line["context"]["ai"]["pinned_share"] == 0.93
    assert ai_line["context"]["ai"]["learned_share"] == 0.885


def test_a_hint_the_game_does_not_take_stops_a_pinned_run(agent: Agent) -> None:
    pinned_plan(agent, 0.93)
    agent.mock["hintFails"] = True
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    agent.drive.run(agent.rt.table_from({"laps": 1, "stopAt": (agent.drive.t or 0) + 2}))

    ghost = agent.rt.eval('require("game.ghost")')
    assert ghost.phase() == "failed"
    assert "tyre hint is not set, the plan pins 0.9300" in ghost.message()
    assert agent.mock["car"]["isAIControlled"] is False


def test_a_refuel_that_does_not_take_stops_the_run(agent: Agent) -> None:
    pinned_plan(agent, 0.93)
    agent.mock["car"]["fuel"] = 24
    agent.mock["fuelStuck"] = True
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    agent.drive.run(agent.rt.table_from({"laps": 1, "stopAt": (agent.drive.t or 0) + 8}))

    ghost = agent.rt.eval('require("game.ghost")')
    assert ghost.phase() == "failed"
    assert (
        "the car has 24.0 L after refuelling for block 1, the plan wants 30.0 L (set 4 times)"
        in (ghost.message())
    )
    assert not [p for p in agent.passes() if "block" in p and p["block"]["lap"] > 0]


def command(agent: Agent, name: str, **fields: Any) -> None:
    outbox = agent.spool_root.parent / "outbox"
    outbox.mkdir(parents=True, exist_ok=True)
    body = {"id": name, "at": int(time.time()), "command": name, **fields}
    (outbox / "agent_command.json").write_text(json.dumps(body), encoding="utf-8")


def test_fixed_free_laps_drive_at_a_test_hint_and_learn_nothing(agent: Agent) -> None:
    calibration = write_calibration(agent, 0.885)
    before = calibration.read_text(encoding="utf-8")
    agent.mock["physicsAllowed"] = True
    command(agent, "ai_laps_fixed", ai_hint_share=0.97)
    off = agent.rt.eval("function(d) return d > 300 and d < 330 end")
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    agent.drive.run(agent.rt.table_from({"laps": 3, "off": off}))

    hints = [args[0] for name, args in physics_calls(agent) if name == "tyres_hint"]
    assert hints[-1] == 0.97
    state = to_py(agent.rt.eval('require("game.calibration").state'))
    assert state["hint"] == 0.885, "laps off the track at a test hint do not lower the share"
    assert calibration.read_text(encoding="utf-8") == before
    assert {p["context"]["ai_hint"] for p in agent.passes()} == {0.97}


def test_fixed_free_laps_refuse_a_share_out_of_range(agent: Agent) -> None:
    agent.mock["physicsAllowed"] = True
    command(agent, "ai_laps_fixed", ai_hint_share=3)
    agent.drive.pit(True)
    agent.drive.idle(200)

    assert physics_calls(agent) == []


def test_a_plan_with_a_share_out_of_range_is_ignored(rt: Any) -> None:
    runplan = rt.eval('require("core.runplan")')
    block = {"order": 1, "label": "Setup K", "laps_target": 3, "values": {"ARB_FRONT": 2}}
    plan = {"v": 1, "experiment_id": 3, "revision": 1, "ai_hint_share": 2.5, "blocks": [block]}

    loaded, problem = runplan.read(to_lua(rt, plan))
    assert loaded is None and "ai_hint_share" in problem


def test_the_fuel_never_asks_more_than_the_tank_holds(rt: Any) -> None:
    ghost = rt.eval('require("core.ghost")')
    state = ghost.new()
    base = {"dt": 1 / 60, "planId": 7, "driverAi": True, "allowed": True, "block": 1}
    ghost.step(state, rt.table_from(base | {"fuel": 12, "maxFuel": 20, "speedKmh": 0}))

    assert state.fuel == 20


def test_a_hint_other_than_the_pinned_one_is_named(rt: Any) -> None:
    aitune = rt.eval('require("core.aitune")')

    assert aitune.pinMismatch(1.1, 0.93, 1.1 * 0.93) is None
    assert aitune.pinMismatch(1.1, None, 0.5) is None, "nothing pinned, nothing to check"
    assert aitune.pinMismatch(1.1, 0.93, 1.1 * 0.885) == (
        "the AI's tyre hint is 0.9735, the plan pins 1.0230 (0.9300 of the car's 1.1000)"
    )


def test_a_fresh_runtime_has_no_build_module() -> None:
    rt = runtime()
    ok = rt.eval('(pcall(require, "build"))')
    assert ok is False, "agent/src has no build.lua: only the installer writes one"
