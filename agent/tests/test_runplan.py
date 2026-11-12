"""The run-plan rules (core/runplan.lua): blocks, out-laps, push laps and pass tags. Pure Lua."""

from typing import Any

import pytest

from harness import to_lua, to_py

BASE = {"ARB_FRONT": 3, "ARB_REAR": 1, "FUEL": 30}
SOFT = BASE | {"ARB_FRONT": 2}


def plan_json(revision: int = 1, blocks: int = 4, experiment: int = 12) -> dict[str, Any]:
    labels, values = ["Setup K", "Setup R"], [BASE, SOFT]
    return {
        "v": 1,
        "experiment_id": experiment,
        "revision": revision,
        "blocks": [
            {
                "order": n,
                "label": labels[(n - 1) % 2],
                "laps_target": 2,
                "values": values[(n - 1) % 2],
            }
            for n in range(1, blocks + 1)
        ],
    }


class Plan:
    """core/runplan.lua with one state, driven from Python."""

    def __init__(self, rt: Any) -> None:
        self.rt = rt
        self.lua = rt.eval('require("core.runplan")')
        self.state = self.lua.new()

    def load(self, decoded: dict[str, Any], saved: dict[str, Any] | None = None) -> None:
        plan = self.lua.read(to_lua(self.rt, decoded))
        assert not isinstance(plan, tuple), f"run plan refused: {plan}"
        self.lua.load(self.state, plan, to_lua(self.rt, saved) if saved else None)

    def setup_is(self, values: dict[str, float]) -> None:
        self.lua.setupIs(self.state, to_lua(self.rt, values))

    def lap(self) -> str | None:
        """One lap from the start line back to it; returns what the crossing reported."""
        result = None
        for pos in (0.05, 0.3, 0.5, 0.7, 0.95, 0.02):
            result = self.lua.position(self.state, pos) or result
        return result

    def tag(self) -> dict[str, Any] | None:
        tag = self.lua.tag(self.state)
        return to_py(tag) if tag is not None else None

    def status(self) -> str:
        return str(self.lua.status(self.state))


@pytest.fixture
def plan(rt: Any) -> Plan:
    return Plan(rt)


def test_a_block_is_an_out_lap_and_then_its_push_laps(plan: Plan) -> None:
    plan.load(plan_json())
    assert plan.tag() is None, "nothing is tagged before the block's setup is on the car"
    assert "pit and load Setup K" in plan.status()

    plan.setup_is(BASE)
    tags = [plan.tag()]
    results = []
    for _ in range(3):
        results.append(plan.lap())
        tags.append(plan.tag())

    block = {"experimentId": 12, "order": 1}
    assert tags[:3] == [block | {"lap": 0}, block | {"lap": 1}, block | {"lap": 2}]
    assert results == [None, "lap", "done"]
    assert tags[3] is None, "the in-lap after the last push lap belongs to no block"
    assert "block 2 of 4: pit and load Setup R" in plan.status()


def test_passes_are_not_tagged_on_the_wrong_setup(plan: Plan) -> None:
    plan.load(plan_json())

    plan.setup_is(SOFT)  # block 1 wants BASE
    plan.lap()

    assert plan.tag() is None
    plan.setup_is(BASE | {"FUEL": 31})  # one other item differs: still not the block's setup
    assert plan.tag() is None


def test_four_blocks_alternate_and_then_the_plan_is_complete(plan: Plan) -> None:
    plan.load(plan_json())
    orders = []

    for values in (BASE, SOFT, BASE, SOFT):
        plan.setup_is(values)
        orders.append(plan.tag()["order"])
        for _ in range(3):
            plan.lap()

    assert orders == [1, 2, 3, 4]
    assert plan.status() == "Experiment 12: run plan complete."
    plan.setup_is(SOFT)
    assert plan.tag() is None


def test_a_crossing_only_counts_after_the_middle_of_the_lap(plan: Plan) -> None:
    plan.load(plan_json())
    plan.setup_is(BASE)

    for pos in (0.9, 0.98, 0.01, 0.99, 0.02):  # pit exit before the line, then a spin on it
        plan.lua.position(plan.state, pos)

    assert plan.tag()["lap"] == 0, "still the out-lap"


def test_pitting_or_restarting_makes_the_next_lap_an_out_lap_again(plan: Plan) -> None:
    plan.load(plan_json())
    plan.setup_is(BASE)
    plan.lap()
    plan.lap()
    assert plan.tag()["lap"] == 2

    plan.lua.pit(plan.state)  # teleport to the pits half-way through push lap 2
    plan.setup_is(BASE)

    assert plan.tag()["lap"] == 0
    plan.lap()
    assert plan.tag()["lap"] == 2, "the push lap that was not finished is driven again"


def test_a_new_revision_keeps_the_position_and_adds_blocks(plan: Plan) -> None:
    plan.load(plan_json())
    for values in (BASE, SOFT, BASE, SOFT):
        plan.setup_is(values)
        for _ in range(3):
            plan.lap()

    plan.load(plan_json(revision=2, blocks=6))  # extended after an inconclusive verdict

    assert "block 5 of 6: pit and load Setup K" in plan.status()
    plan.setup_is(BASE)
    assert plan.tag() == {"experimentId": 12, "order": 5, "lap": 0}


def test_a_block_without_laps_is_done_once_its_setup_is_on_the_car(plan: Plan) -> None:
    decoded = plan_json(blocks=2)
    decoded["blocks"].append({"order": 3, "label": "Setup K", "laps_target": 0, "values": BASE})
    plan.load(decoded, saved={"experiment_id": 12, "order": 3, "push_done": 0})
    assert "block 3 of 3: pit and load Setup K" in plan.status()

    plan.setup_is(BASE)

    assert plan.status() == "Experiment 12: run plan complete."
    assert plan.tag() is None


def test_position_survives_closing_the_game_but_not_a_new_experiment(plan: Plan, rt: Any) -> None:
    plan.load(plan_json())
    plan.setup_is(BASE)
    for _ in range(3):
        plan.lap()
    plan.setup_is(SOFT)
    plan.lap()
    plan.lap()
    saved = to_py(plan.lua.snapshot(plan.state))
    assert saved == {"experiment_id": 12, "order": 2, "push_done": 1}

    reopened = Plan(rt)
    reopened.load(plan_json(), saved=saved)
    reopened.setup_is(SOFT)
    reopened.lap()  # out-lap again
    assert reopened.tag() == {"experimentId": 12, "order": 2, "lap": 2}

    other = Plan(rt)
    other.load(plan_json(experiment=13), saved=saved)
    assert "Experiment 13, block 1 of 4" in other.status()


def test_changes_are_the_items_that_differ_by_name(plan: Plan, rt: Any) -> None:
    block = to_lua(rt, {"values": SOFT | {"NOT_ON_THIS_CAR": 5}})

    changes = to_py(plan.lua.changes(block, to_lua(rt, BASE)))

    assert changes == [{"name": "ARB_FRONT", "from": 3, "to": 2}]
    assert to_py(plan.lua.changes(block, to_lua(rt, SOFT))) in ([], {})


@pytest.mark.parametrize(
    ("change", "problem"),
    [
        ({"v": 2}, "version 1"),
        ({"experiment_id": None}, "experiment id"),
        ({"blocks": []}, "no blocks"),
        ({"blocks": [{"order": 1, "label": "Setup K"}]}, "block 1 is incomplete"),
    ],
)
def test_a_broken_run_plan_is_refused(
    plan: Plan, rt: Any, change: dict[str, Any], problem: str
) -> None:
    decoded = {k: v for k, v in (plan_json() | change).items() if v is not None}

    loaded, why = plan.lua.read(to_lua(rt, decoded))

    assert loaded is None
    assert problem in why
    assert plan.lua.read(None)[0] is None
