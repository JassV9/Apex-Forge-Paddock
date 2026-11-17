"""A person at the wheel keeps the car (docs/adr/0021-human-keeps-the-car.md).

A game that starts with an experiment for the AI open on the car must not hand the car to the
AI, load its setups, or count the person's laps into the AI's blocks. All offline, on the
mocked CSP API: nothing here was seen in Assetto Corsa.
"""

from pathlib import Path
from types import SimpleNamespace
from typing import Any

from conftest import Agent, boot_agent
from harness import TESTS_DIR, runtime, to_py
from test_ghost import FRAME, machine, obs, physics_calls, run_for
from test_plan_agent import CORNERS, LAPS, setups, stint, window, write_plan
from track import Track

DRIVER_ID = "ks_mazda_mx5_cup"


def open_experiment_then_start_game(
    rt: Any, track: Track, tmp_path: Path, driver: str = "ai"
) -> Agent:
    """Boot the agent on a car whose outbox already holds an experiment's run plan."""
    other = runtime()  # a second game, only to read the car's own setup
    other.execute(f'package.path = "{TESTS_DIR.as_posix()}/?.lua;" .. package.path')
    probe = boot_agent(other, track, tmp_path / "probe")
    base, soft = setups(probe)
    outbox = SimpleNamespace(spool_root=tmp_path / "paddock" / "spool")
    write_plan(outbox, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver=driver)
    agent = boot_agent(rt, track, tmp_path)
    agent.mock["physicsAllowed"] = True
    return agent


def ghost(agent: Agent) -> Any:
    return agent.rt.eval('require("game.ghost")')


# The state machine ---------------------------------------------------------------------------


def test_a_held_ai_does_not_take_the_car_for_a_plan_and_says_why(rt) -> None:
    core, state = machine(rt)
    core.hold(state)

    assert run_for(rt, core, state, 5) == []
    assert state.phase == "off" and not core.active(state)
    assert "Experiment 7 is for the AI to drive" in state.message
    assert "your laps are not counted" in state.message


def test_the_hold_outlasts_the_experiment_it_was_made_for(rt) -> None:
    core, state = machine(rt)
    core.hold(state)
    run_for(rt, core, state, 1)

    assert run_for(rt, core, state, 5, planId=8) == [], "the next experiment is held too"
    assert "Experiment 8" in state.message


def test_the_driver_hands_the_car_over_and_the_hold_ends(rt) -> None:
    core, state = machine(rt)
    core.hold(state)
    run_for(rt, core, state, 1)

    core.resume(state)

    assert state.held is False
    seen = [a["name"] for a in to_py(core.step(state, obs(rt))) or []]
    assert seen == ["autopilot_off", "teleport_pits"]


def test_free_laps_and_calibration_are_a_request_for_the_ai_too(rt) -> None:
    for ask in ("lap", "calibrate"):
        core, state = machine(rt)
        core.hold(state)
        getattr(core, ask)(state)

        assert state.held is False, ask


# The whole agent -----------------------------------------------------------------------------


def test_a_game_started_with_an_ai_experiment_open_leaves_the_car_to_the_driver(
    rt, track, tmp_path
) -> None:
    agent = open_experiment_then_start_game(rt, track, tmp_path)
    agent.mock["editable"] = True
    agent.drive.pit(True)
    agent.drive.idle(round(4 / FRAME))
    agent.drive.pit(False)
    buttons = window(agent)
    stint(agent, 1 + LAPS)

    assert physics_calls(agent) == [], "no teleport, repair, fuel or autopilot"
    assert agent.mock["car"]["isAIControlled"] is False
    assert not any(button.startswith("Load ") for button in buttons), "its setups are not offered"
    assert "Let the AI drive this plan" in buttons
    assert ghost(agent).held() is True
    assert "for the AI to drive" in ghost(agent).message()
    assert agent.rt.eval('require("game.plan").state.pushDone') == 0, "no lap of yours is counted"
    assert len(agent.passes()) >= (1 + LAPS) * CORNERS
    assert all("block" not in p for p in agent.passes()), "none of your passes joins its blocks"
    assert {line["source"] for line in agent.lines() if line["kind"] == "session"} == {"human"}


def test_the_driver_can_hand_the_held_plan_to_the_ai(rt, track, tmp_path) -> None:
    agent = open_experiment_then_start_game(rt, track, tmp_path)
    agent.mock["editable"] = True
    agent.drive.pit(True)
    agent.drive.idle(round(4 / FRAME))
    agent.mock["clicks"]["Let the AI drive this plan"] = True
    window(agent)
    agent.drive.idle(round(2.5 / FRAME))
    agent.drive.pit(False)

    assert ghost(agent).held() is False
    assert agent.mock["car"]["isAIControlled"] is True
    assert physics_calls(agent), "the AI took the car only when asked"


def test_an_experiment_for_a_human_is_not_held_and_still_counts_the_drivers_laps(
    rt, track, tmp_path
) -> None:
    agent = open_experiment_then_start_game(rt, track, tmp_path, driver="human")
    agent.mock["editable"] = True
    agent.drive.pit(True)
    agent.drive.idle(round(4 / FRAME))
    agent.mock["clicks"]["Load Setup K"] = True
    window(agent)
    agent.drive.pit(False)
    stint(agent, 1 + LAPS)

    assert ghost(agent).held() is False
    assert physics_calls(agent) == []
    assert sum(1 for p in agent.passes() if "block" in p) == (1 + LAPS) * CORNERS


def test_a_stopped_ai_does_not_let_the_drivers_laps_into_its_experiment(agent: Agent) -> None:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.pit(True)
    agent.drive.idle(130)
    ghost(agent).stop()
    agent.drive.pit(False)

    stint(agent, 1 + LAPS)

    assert agent.passes() and all("block" not in p for p in agent.passes())


def test_a_reloaded_agent_does_not_take_the_car_from_the_ai_that_has_it(
    rt, track, tmp_path
) -> None:
    agent = open_experiment_then_start_game(rt, track, tmp_path)
    assert ghost(agent).held() is True
    agent.rt.eval('require("game.ghost")').resume()

    agent.mock["car"]["isAIControlled"] = True
    agent.rt.eval('require("game.agent")').keepCarFromAi()

    assert ghost(agent).held() is False, "a run in progress goes on after a reload"


def test_the_status_file_says_the_ai_is_held(rt, track, tmp_path) -> None:
    agent = open_experiment_then_start_game(rt, track, tmp_path)
    agent.drive.idle(round(3 / FRAME))

    status = (tmp_path / "paddock" / "agent_status.json").read_text(encoding="utf-8")

    assert '"ghost_held":true' in status.replace(" ", "")
