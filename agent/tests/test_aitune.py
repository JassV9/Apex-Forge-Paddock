"""AI calibration: free laps teach the AI how much grip to assume, experiments freeze it, and
the AI always runs on the car's own grip. Offline only: what was seen in Assetto Corsa is in
docs/runbooks/ghost-driver.md."""

from typing import Any

from conftest import Agent
from harness import to_lua, to_py
from test_ghost import let_ai_load, physics_calls
from test_plan_agent import LAPS, setups, stint, write_plan


def tune(rt: Any) -> Any:
    return rt.eval('require("core.aitune")')


def lap(rt: Any, state: Any, off: range | None = None, start: int = 0) -> None:
    a = tune(rt)
    for i in range(start, a.SECTIONS):
        a.observe(state, (i + 0.5) / a.SECTIONS, off is not None and i in off)


def test_a_lap_with_an_incident_lowers_the_grip_the_ai_assumes(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, off=range(100, 101))

    assert a.endLap(state, True) == 1
    assert abs(a.hint(state) - a.STEP) < 1e-9
    assert to_py(state)["where"] == [100]


def test_the_sections_crawled_through_after_going_off_are_one_incident(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, off=range(184, 188))

    assert a.endLap(state, True) == 1, "off at 184, then crawling back is the same mistake"


def test_the_hint_only_comes_down_and_stops_at_its_floor(rt):
    a = tune(rt)
    state = a.new()
    for _ in range(40):
        lap(rt, state, off=range(50, 51))
        a.endLap(state, True)

    assert abs(a.hint(state) - a.MIN_HINT) < 1e-9
    lap(rt, state)
    a.endLap(state, True)
    assert abs(a.hint(state) - a.MIN_HINT) < 1e-9, "a clean lap does not raise it"


def test_clean_laps_mark_it_calibrated(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, off=range(60, 61))
    a.endLap(state, True)
    for _ in range(a.CLEAN_LAPS):
        assert not a.calibrated(state)
        lap(rt, state)
        a.endLap(state, True)

    assert a.calibrated(state)
    assert "calibrated, assumes 97 % grip" in a.summary(state)


def test_an_experiment_never_changes_the_hint(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, off=range(60, 61))

    assert a.endLap(state, False) == 1, "the incident is still counted"
    assert a.hint(state) == 1


def test_the_calibration_survives_a_restart_and_a_change_of_method_drops_it(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, off=range(60, 61))
    a.endLap(state, True)
    saved = to_py(a.snapshot(state))

    assert a.hint(a.restore(to_lua(rt, saved))) == a.hint(state)
    assert a.hint(a.restore(to_lua(rt, saved | {"v": 1}))) == 1, "speed caps of version 1 are dropped"


# The whole agent ---------------------------------------------------------------------------


def test_free_laps_lower_the_hint_where_the_ai_went_off_and_apply_it(agent: Agent) -> None:
    agent.mock["physicsAllowed"] = True
    agent.mock["aiTyresHint"] = 1.1
    agent.rt.eval('require("game.ghost")').lap()
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    off = agent.rt.eval("function(d) return d > 300 and d < 330 end")

    agent.drive.run(agent.rt.table_from({"laps": 3, "off": off}))

    state = agent.rt.eval('require("game.calibration").state')
    hint = tune(agent.rt).hint(state)
    assert hint < 1, "laps with an incident lower the grip the AI assumes"
    hints = [args[0] for name, args in physics_calls(agent) if name == "tyres_hint"]
    assert hints and hints[0] == 1.1, "starts from the car's own hint"
    assert abs(hints[-1] - 1.1 * hint) < 1e-9, "and applies the learned share of it"
    grips = {args[0] for name, args in physics_calls(agent) if name == "extra_grip"}
    assert grips == {1}, "the AI drives on the car's own grip"
    saved = list((agent.spool_root.parent / "ai_tune").glob("*.json"))
    assert len(saved) == 1


def test_an_experiment_applies_the_hint_without_learning(agent: Agent) -> None:
    base, soft = setups(agent)
    agent.mock["physicsAllowed"] = True
    agent.mock["editable"] = True
    off = agent.rt.eval("function(d) return d > 300 and d < 330 end")
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)], driver="ai")
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)

    agent.drive.run(agent.rt.table_from({"laps": 1 + LAPS, "off": off}))

    state = agent.rt.eval('require("game.calibration").state')
    assert tune(agent.rt).hint(state) == 1, "nothing is learned during an experiment"
    assert to_py(state)["lastIncidents"] >= 1, "but the incidents are counted"
    stint(agent, 0)
