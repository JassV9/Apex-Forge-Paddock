"""Ghost Driver level 2: AC's AI drives the run plan on the player's car.

The state machine (core/ghost.lua) is tested on its own, then the whole agent runs a two-block
plan on the mocked CSP API with a recorded physics API. Everything here is offline; what was
seen in Assetto Corsa is in docs/runbooks/ghost-driver.md.
"""

import json
import time
from collections import Counter
from typing import Any

from conftest import Agent
from harness import to_py
from test_plan_agent import CORNERS, LAPS, setups, stint, write_plan

FRAME = 1 / 60


def machine(rt: Any) -> tuple[Any, Any]:
    ghost = rt.eval('require("core.ghost")')
    return ghost, ghost.new()


def obs(rt: Any, **changes: Any) -> Any:
    base = {
        "dt": FRAME,
        "planId": 7,
        "driverAi": True,
        "allowed": True,
        "block": 1,
        "parked": False,
        "onTrack": True,
        "speedKmh": 150,
        "ai": True,
        "fuel": 40,
        "damage": 0,
    }
    return rt.table_from(base | changes)


def names(actions: Any) -> list[str]:
    return [a["name"] for a in to_py(actions) or []]


def run_for(rt: Any, ghost: Any, state: Any, seconds: float, **changes: Any) -> list[str]:
    seen: list[str] = []
    for _ in range(round(seconds / FRAME)):
        seen += names(ghost.step(state, obs(rt, dt=FRAME, **changes)))
    return seen


def start_driving(rt: Any, ghost: Any, state: Any) -> None:
    ghost.step(state, obs(rt))
    run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False)
    ghost.loaded(state, True, "ok")


def test_a_plan_for_the_ai_sends_the_car_to_the_pits_then_loads_and_hands_it_over(rt):
    ghost, state = machine(rt)

    assert names(ghost.step(state, obs(rt))) == ["autopilot_off", "teleport_pits"]
    assert state.phase == "to_pits" and state.fuel == 40
    seen = run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False, fuel=12)
    assert seen == ["repair", "set_fuel", "load"]
    assert names(ghost.loaded(state, True, "ok")) == ["autopilot_on", "start"]
    assert state.phase == "driving" and ghost.active(state)


def test_every_block_gets_the_fuel_of_the_first(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.step(state, obs(rt, block=2, fuel=31))) == ["autopilot_off", "teleport_pits"]
    actions: list[Any] = []
    for _ in range(round((ghost.SETTLE_S + 0.1) / FRAME)):
        actions += to_py(ghost.step(state, obs(rt, block=2, parked=True, onTrack=False))) or []
    fuel = [a for a in actions if a["name"] == "set_fuel"]
    assert fuel == [{"name": "set_fuel", "liters": 40}]


def test_a_finished_plan_stops_the_ai_and_parks_the_car(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.step(state, obs(rt, block=None))) == ["autopilot_off", "teleport_pits"]
    assert state.phase == "off" and "complete" in state.message
    assert names(ghost.step(state, obs(rt, block=None))) == []


def test_without_physics_access_the_ai_never_takes_the_car(rt):
    ghost, state = machine(rt)

    assert names(ghost.step(state, obs(rt, allowed=False))) == []
    assert state.phase == "off" and "physics access" in state.message


def test_a_plan_for_a_human_is_left_to_the_human_until_they_ask_for_the_ai(rt):
    ghost, state = machine(rt)

    assert names(ghost.step(state, obs(rt, driverAi=False))) == []
    ghost.resume(state)
    assert names(ghost.step(state, obs(rt, driverAi=False))) == ["autopilot_off", "teleport_pits"]


def test_a_stuck_car_retries_its_block_and_gives_up_after_three_tries(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    for attempt in range(ghost.MAX_RETRIES):
        seen = run_for(rt, ghost, state, ghost.STUCK_S + 0.1, speedKmh=3)
        assert seen[-2:] == ["autopilot_off", "teleport_pits"], f"retry {attempt + 1}"
        run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False)
        ghost.loaded(state, True, "ok")
    assert run_for(rt, ghost, state, ghost.STUCK_S + 0.1, speedKmh=3)[-1:] == ["autopilot_off"]
    assert state.phase == "failed" and "stuck" in state.message


def test_driving_down_the_pit_lane_is_not_stuck_but_parking_in_it_is(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert run_for(rt, ghost, state, ghost.STUCK_S + 5, speedKmh=60, onTrack=False) == []
    seen = run_for(rt, ghost, state, ghost.STUCK_S + 0.1, speedKmh=0, onTrack=False, parked=True)
    assert seen[-2:] == ["autopilot_off", "teleport_pits"], "the AI's own pit stop is not a lap"


def test_damage_during_a_block_retries_it_on_a_repaired_car(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.step(state, obs(rt, damage=3))) == [], "a scrape is not damage"
    assert names(ghost.step(state, obs(rt, damage=24))) == ["autopilot_off", "teleport_pits"]
    assert "damaged" in state.message
    seen = run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False, damage=24)
    assert seen[0] == "repair"


def test_a_setup_that_does_not_load_stops_the_run(rt):
    ghost, state = machine(rt)
    ghost.step(state, obs(rt))
    run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False)

    assert names(ghost.loaded(state, False, "1 value did not take")) == ["autopilot_off"]
    assert state.phase == "failed" and "did not load" in state.message


def test_stop_hands_the_car_back_until_a_new_experiment(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.stop(state)) == ["autopilot_off"]
    assert names(ghost.step(state, obs(rt))) == [], "a stopped run does not restart by itself"
    assert names(ghost.step(state, obs(rt, planId=8))) == ["autopilot_off", "teleport_pits"]


def test_a_reset_by_the_game_retries_the_block(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.step(state, obs(rt, resets=4))) == []
    assert names(ghost.step(state, obs(rt, resets=5))) == ["autopilot_off", "teleport_pits"]
    assert "reset by the game" in state.message


def test_a_block_without_a_counted_lap_for_ten_minutes_is_retried(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert run_for(rt, ghost, state, ghost.LAP_TIMEOUT_S - 1, lapsDone=0) == []
    assert run_for(rt, ghost, state, 2, lapsDone=0)[-2:] == ["autopilot_off", "teleport_pits"]
    assert "no lap counted" in state.message


def test_counted_laps_keep_the_lap_watchdog_quiet(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    for done in range(4):
        assert run_for(rt, ghost, state, ghost.LAP_TIMEOUT_S / 2, lapsDone=done) == []


def test_the_ai_is_put_back_in_control_if_it_lost_the_car(rt):
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert names(ghost.step(state, obs(rt, ai=False))) == ["autopilot_on"]


# The whole agent ---------------------------------------------------------------------------


def let_ai_load(agent: Agent) -> None:
    """Frames standing in the pit box: the agent loads the block and hands the car over."""
    agent.drive.idle(round(2.5 / FRAME))
    agent.drive.pit(False)


def physics_calls(agent: Agent) -> list[tuple[str, list[Any]]]:
    return [(c["name"], c.get("args") or []) for c in to_py(agent.mock["physicsCalls"]) or []]


def test_the_agent_drives_a_two_block_plan_with_the_ai_in_its_own_session(agent: Agent) -> None:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    agent.mock["editable"] = True  # in the pit box the setup can be edited
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.pit(True)
    agent.drive.idle(130)  # the outbox is polled in the pits

    let_ai_load(agent)
    assert agent.mock["car"]["isAIControlled"] is True
    stint(agent, 1 + LAPS)
    let_ai_load(agent)
    stint(agent, 1 + LAPS)

    blocks = [p for p in agent.passes() if "block" in p]
    tagged = Counter((p["block"]["order"], p["block"]["lap"]) for p in blocks)
    assert tagged == {(block, lap): CORNERS for block in (1, 2) for lap in range(LAPS + 1)}
    starts = [line for line in agent.lines() if line["kind"] == "session"]
    sessions = {line["session_id"]: line["source"] for line in starts}
    assert {sessions[p["session_id"]] for p in blocks} == {"ai"}
    assert "human" in sessions.values(), "the session before the AI took over stays the human's"
    calls = physics_calls(agent)
    fuels = {args[0] for name, args in calls if name == "fuel"}
    assert len(fuels) == 1, "both blocks start with the same fuel"
    assert agent.mock["setup"]["ARB_FRONT"] == soft["ARB_FRONT"]
    assert agent.mock["car"]["isAIControlled"] is False, "the car is handed back at the end"
    assert "complete" in agent.rt.eval('require("game.ghost").message()')
    pit_requests = [args for name, args in calls if name == "pit_request"]
    assert len(pit_requests) > 10 and all(args == [False] for args in pit_requests), (
        "the AI is kept from making its own pit stops while it drives"
    )


def test_without_physics_access_the_agent_waits_for_a_human(agent: Agent) -> None:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = False
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.pit(True)
    agent.drive.idle(130)

    assert physics_calls(agent) == []
    assert "physics access" in agent.rt.eval('require("game.ghost").message()')


def test_a_free_run_laps_the_car_on_its_setup_until_a_plan_arrives(rt):
    ghost, state = machine(rt)
    assert names(ghost.step(state, obs(rt, block=None, planId=None))) == []

    ghost.lap(state)
    assert names(ghost.step(state, obs(rt, block=None, planId=None))) == [
        "autopilot_off",
        "teleport_pits",
    ]
    run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, block=None, planId=None, parked=True)
    ghost.loaded(state, True, "")
    assert state.block == 0 and "free laps" in state.message
    assert run_for(rt, ghost, state, 30, block=None, planId=None) == [], "it keeps lapping"
    seen = names(ghost.step(state, obs(rt, block=1, planId=9, driverAi=False)))
    assert seen == ["autopilot_off", "teleport_pits"] and state.phase == "off"


def test_the_agent_records_free_ai_laps_in_an_ai_session(agent: Agent) -> None:
    agent.mock["physicsAllowed"] = True
    commands = agent.spool_root.parent / "outbox"
    commands.mkdir(parents=True, exist_ok=True)
    command = {"id": 1, "at": int(time.time()), "command": "ai_laps"}
    (commands / "agent_command.json").write_text(json.dumps(command), encoding="utf-8")
    agent.drive.pit(True)
    agent.drive.idle(130)

    let_ai_load(agent)
    stint(agent, 2)

    starts = [line for line in agent.lines() if line["kind"] == "session"]
    sources = {line["session_id"]: line["source"] for line in starts}
    assert agent.passes() and {sources[p["session_id"]] for p in agent.passes()} == {"ai"}
    assert all("block" not in p for p in agent.passes()), "free laps belong to no experiment"


def test_an_old_command_is_not_replayed_by_a_reloaded_agent(agent: Agent) -> None:
    agent.mock["physicsAllowed"] = True
    commands = agent.spool_root.parent / "outbox"
    commands.mkdir(parents=True, exist_ok=True)
    stale = {"id": "free-1", "at": int(time.time()) - 3600, "command": "ai_laps"}
    (commands / "agent_command.json").write_text(json.dumps(stale), encoding="utf-8")
    agent.drive.pit(True)
    agent.drive.idle(200)

    assert physics_calls(agent) == []


def test_a_plan_for_the_ai_is_picked_up_during_a_free_run(agent: Agent) -> None:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    agent.mock["editable"] = True
    agent.rt.eval('require("game.ghost")').lap()
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    agent.drive.run(agent.rt.table_from({"laps": 1}))  # free laps, on track

    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.run(agent.rt.table_from({"laps": 1, "stopAt": agent.drive.t + 5}))

    assert agent.rt.eval('require("game.ghost").phase()') == "to_pits"
    assert "block 1" in agent.rt.eval('require("game.ghost").message()')
