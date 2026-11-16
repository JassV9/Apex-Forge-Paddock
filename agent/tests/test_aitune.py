"""AI calibration: free laps teach the AI where it needs a lower top speed, experiments freeze
it, and the AI always runs on the car's own grip. Offline only: what was seen in Assetto Corsa
is in docs/runbooks/ghost-driver.md."""

from typing import Any

from conftest import Agent
from harness import to_lua, to_py
from test_ghost import let_ai_load, physics_calls
from test_plan_agent import LAPS, setups, stint, write_plan


def tune(rt: Any) -> Any:
    return rt.eval('require("core.aitune")')


def lap(rt: Any, state: Any, incident_at: float | None, speed: float = 150) -> None:
    a = tune(rt)
    for i in range(a.SECTIONS):
        pos = (i + 0.5) / a.SECTIONS
        a.observe(state, pos, speed, incident_at is not None and abs(pos - incident_at) < 0.003)


def caps(rt: Any, state: Any) -> dict[int, float]:
    return {int(s): float(v) for s, v in (to_py(tune(rt).snapshot(state))["caps"] or [])}


def test_an_incident_caps_the_speed_on_its_approach(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, incident_at=0.5025)

    assert a.endLap(state, True) == 1
    capped = caps(rt, state)
    assert sorted(capped) == list(range(100 - a.LOOKBACK, 101))
    assert all(abs(v - 150 * a.CAP_FACTOR) < 0.06 for v in capped.values())
    assert a.capAt(state, 0.5025) is not None and a.capAt(state, 0.2) is None


def test_an_out_lap_caps_only_the_sections_it_drove(rt):
    a = tune(rt)
    state = a.new()
    for i in range(14, a.SECTIONS):  # joins the track from the pit exit at section 14
        pos = (i + 0.5) / a.SECTIONS
        a.observe(state, pos, 74, i == 14)

    a.endLap(state, True)
    assert caps(rt, state) == {14: round(74 * a.CAP_FACTOR, 1)}


def test_a_repeated_incident_lowers_the_cap_again_and_never_raises_it(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, incident_at=0.5025)
    a.endLap(state, True)
    first = caps(rt, state)[100]
    lap(rt, state, incident_at=0.5025, speed=200)
    a.endLap(state, True)

    assert abs(caps(rt, state)[100] - first * a.STEP) < 0.06


def test_clean_laps_mark_it_calibrated(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, incident_at=0.3025)
    a.endLap(state, True)
    for _ in range(a.CLEAN_LAPS):
        assert not a.calibrated(state)
        lap(rt, state, incident_at=None)
        a.endLap(state, True)

    assert a.calibrated(state)
    assert "calibrated" in a.summary(state)


def test_an_experiment_never_changes_the_caps(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, incident_at=0.3025)

    assert a.endLap(state, False) == 1, "the incident is still counted"
    assert caps(rt, state) == {}


def test_the_caps_survive_a_restart_and_a_change_of_method_drops_them(rt):
    a = tune(rt)
    state = a.new()
    lap(rt, state, incident_at=0.5025)
    a.endLap(state, True)
    saved = to_py(a.snapshot(state))

    assert caps(rt, a.restore(to_lua(rt, saved))) == caps(rt, state)
    assert caps(rt, a.restore(to_lua(rt, saved | {"sections": 50}))) == {}


# The whole agent ---------------------------------------------------------------------------


def test_free_laps_learn_a_cap_where_the_ai_went_off_and_apply_it(agent: Agent) -> None:
    agent.mock["physicsAllowed"] = True
    agent.rt.eval('require("game.ghost")').lap()
    agent.drive.pit(True)
    agent.drive.idle(130)
    let_ai_load(agent)
    off = agent.rt.eval("function(d) return d > 300 and d < 330 end")

    agent.drive.run(agent.rt.table_from({"laps": 2, "off": off}))

    state = agent.rt.eval('require("game.calibration").state')
    assert caps(agent.rt, state), "a cap where the car went off"
    speeds = [args[0] for name, args in physics_calls(agent) if name == "top_speed"]
    assert any(s < 1e8 for s in speeds), "the cap is applied on track"
    grips = {args[0] for name, args in physics_calls(agent) if name == "extra_grip"}
    assert grips == {1}, "the AI drives on the car's own grip"
    saved = list((agent.spool_root.parent / "ai_tune").glob("*.json"))
    assert len(saved) == 1


def test_an_experiment_applies_the_caps_without_learning(agent: Agent) -> None:
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
    assert caps(agent.rt, state) == {}, "nothing is learned during an experiment"
    assert to_py(state)["lastIncidents"] >= 1, "but the incidents are counted"
    stint(agent, 0)
