"""The AI drives a block only on that block's setup, and stops at once, saying why, when it is not.

Seen in Assetto Corsa (Formula Hybrid 2026, experiment 13): a setup set and checked in the pits
was not on the car on track, and the AI drove 28 laps that counted for nothing before the lap
watchdog gave up. Everything here is offline, on the mocked CSP API.
"""

import json
from typing import Any

from conftest import Agent
from harness import to_lua
from test_ghost import (
    FRAME,
    let_ai_load,
    machine,
    names,
    obs,
    run_for,
    start_driving,
    start_free_laps,
)
from test_plan_agent import LAPS, setups, stint, write_plan

WING = "Setup S is not on the car after leaving the pits: WING_1 reads 1, the plan wants 0"


# The rules -----------------------------------------------------------------------------------


def test_the_reason_names_every_value_that_differs_with_the_car_and_the_plan(rt) -> None:
    runplan = rt.eval('require("core.runplan")')
    state = runplan.new()
    plan = {
        "experimentId": 13,
        "revision": 1,
        "driver": "ai",
        "blocks": [
            {"order": 1, "label": "Setup S", "lapsTarget": 3, "values": {"WING_1": 0, "WING_9": 1}},
        ],
    }
    runplan.load(state, to_lua(rt, plan), None)

    runplan.setupIs(state, to_lua(rt, {"WING_1": 1, "WING_9": 0.5}), "after loading")
    assert runplan.offCar(state) == (
        "Setup S is not on the car after loading: WING_1 reads 1, the plan wants 0;"
        " WING_9 reads 0.5, the plan wants 1"
    )
    assert state.onCar is False
    runplan.setupIs(state, to_lua(rt, {"WING_1": 0, "WING_9": 1}), "after leaving the pits")
    assert runplan.offCar(state) is None and state.onCar is True


def test_a_block_setup_off_the_car_stops_the_ai_at_once_and_says_why(rt) -> None:
    ghost, state = machine(rt)
    start_driving(rt, ghost, state)

    assert run_for(rt, ghost, state, 1, offCar=WING) == ["autopilot_off"]
    assert state.phase == "failed" and not ghost.active(state)
    assert state.message == "AI driver stopped: " + WING + "."
    assert run_for(rt, ghost, state, 5, offCar=WING) == [], "it stays stopped until resumed"


def test_a_load_that_reads_back_wrong_stops_the_ai_before_it_drives(rt) -> None:
    ghost, state = machine(rt)
    ghost.step(state, obs(rt))
    run_for(rt, ghost, state, ghost.SETTLE_S + 0.1, parked=True, onTrack=False)

    off = "Setup S is not on the car after loading: WING_9 reads 0, the plan wants 1"
    assert names(ghost.loaded(state, True, "ok", off)) == ["autopilot_off"]
    assert state.phase == "failed" and state.message == "AI driver stopped: " + off + "."


def test_free_laps_have_no_block_setup_to_check(rt) -> None:
    ghost, state = machine(rt)
    start_free_laps(rt, ghost, state)

    assert "autopilot_off" not in run_for(rt, ghost, state, 1, block=None, planId=None, offCar=WING)
    assert state.phase == "driving"


# The whole agent -----------------------------------------------------------------------------


def start_ai_plan(agent: Agent) -> dict[str, float]:
    """An AI plan whose second block softens the front anti-roll bar; its first block driven."""
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    agent.mock["editable"] = True
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    stint(agent, 1 + LAPS)
    return base


def ai_passes_of_block_two_or_none(agent: Agent) -> list[dict[str, Any]]:
    sources = {
        line["session_id"]: line["source"] for line in agent.lines() if line["kind"] == "session"
    }
    return [
        p
        for p in agent.passes()
        if sources[p["session_id"]] == "ai" and p.get("block", {}).get("order", 2) == 2
    ]


def assert_stopped_naming(agent: Agent, needle: str, before: int) -> None:
    game_ghost = agent.rt.eval('require("game.ghost")')
    assert game_ghost.phase() == "failed"
    assert agent.mock["car"]["isAIControlled"] is False
    message = game_ghost.message()
    assert message.startswith("AI driver stopped: Setup R is not on the car") and needle in message
    stint(agent, 2)  # the car is the driver's now: nothing the AI drives goes untagged
    assert len(ai_passes_of_block_two_or_none(agent)) == before
    agent.drive.idle(130)
    status = agent.spool_root.parent / "agent_status.json"
    assert needle in json.loads(status.read_text(encoding="utf-8"))["ghost_message"]


def test_a_value_the_car_moves_when_another_is_set_stops_the_ai_after_the_load(agent) -> None:
    base = start_ai_plan(agent)
    before = len(ai_passes_of_block_two_or_none(agent))
    agent.mock["linked"] = to_lua(agent.rt, {"ARB_FRONT": "ARB_REAR"})
    let_ai_load(agent)

    rear = f"ARB_REAR reads {base['ARB_FRONT'] - 1:g}, the plan wants {base['ARB_REAR']:g}"
    assert_stopped_naming(agent, "after loading: " + rear, before)


def test_a_value_that_reverts_on_pit_exit_stops_the_ai_as_it_leaves(agent) -> None:
    base = start_ai_plan(agent)
    before = len(ai_passes_of_block_two_or_none(agent))
    agent.drive.idle(round(2.5 / FRAME))  # parked: the block's setup is loaded and checked
    agent.mock["setup"]["ARB_FRONT"] = base["ARB_FRONT"]  # the car puts the old value back
    agent.drive.pit(False)

    front = f"ARB_FRONT reads {base['ARB_FRONT']:g}, the plan wants {base['ARB_FRONT'] - 1:g}"
    assert_stopped_naming(agent, "after leaving the pits: " + front, before)
