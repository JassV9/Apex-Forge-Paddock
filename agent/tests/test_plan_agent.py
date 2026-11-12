"""The agent following a run plan on the mocked CSP API: reading the outbox, loading a block's
setup through the setup spinners, and tagging passes with their block and lap.

Everything here is offline. Nothing in this file shows that it works in Assetto Corsa.
"""

import json
from collections import Counter
from typing import Any

import jsonschema

from conftest import Agent
from harness import to_py
from test_spool_format import schema

CORNERS = 7
LAPS = 2  # push laps per block


def write_plan(
    agent: Agent, blocks: list[tuple[str, dict[str, float], int]], revision: int = 1
) -> None:
    """Put a runplan.json into the outbox, the way core does."""
    plan = {
        "v": 1,
        "experiment_id": 12,
        "revision": revision,
        "blocks": [
            {"order": order, "label": label, "laps_target": laps, "values": values}
            for order, (label, values, laps) in enumerate(blocks, start=1)
        ],
    }
    jsonschema.Draft7Validator(schema("runplan")).validate(plan)
    outbox = agent.spool_root.parent / "outbox"
    outbox.mkdir(parents=True, exist_ok=True)
    (outbox / "runplan.json").write_text(json.dumps(plan), encoding="utf-8")


def setups(agent: Agent) -> tuple[dict[str, float], dict[str, float]]:
    """The setup on the mocked car, and the same with a softer front anti-roll bar."""
    base = to_py(agent.mock["setup"])
    return base, base | {"ARB_FRONT": base["ARB_FRONT"] - 1}


def window(agent: Agent) -> list[str]:
    """Draw the agent's window once and return the buttons it showed."""
    agent.mock["seen"] = agent.rt.table()
    agent.rt.eval("script.windowMain")()
    return list(to_py(agent.mock["seen"]) or [])


def pit_and_load(agent: Agent, label: str) -> list[str]:
    """Stop in the pits, open the setup screen, click the load button, and drive out."""
    agent.drive.pit(True)
    agent.drive.idle(130)  # longer than the outbox poll interval
    agent.mock["editable"] = True
    agent.mock["clicks"]["Load " + label] = True
    buttons = window(agent)
    agent.mock["editable"] = False
    agent.drive.pit(False)
    return buttons


def stint(agent: Agent, laps: int) -> None:
    """Whole laps from the start line, then across the line once more as on the way to the pits."""
    agent.drive.run(agent.rt.table_from({"laps": laps}))
    agent.drive.run(agent.rt.table_from({"laps": 1, "stopAt": agent.drive.t + 1}))
    agent.drive.idle(40)  # let the last pass finish encoding and reach the disk


def tags(agent: Agent) -> Counter[tuple[int, int] | None]:
    return Counter(
        (p["block"]["order"], p["block"]["lap"]) if "block" in p else None for p in agent.passes()
    )


def test_two_blocks_are_loaded_driven_and_tagged(agent: Agent) -> None:
    base, soft = setups(agent)
    write_plan(agent, [("Setup K", base, LAPS), ("Setup R", soft, LAPS)])

    assert "Load Setup K" in pit_and_load(agent, "Setup K")
    stint(agent, 1 + LAPS)
    assert "Load Setup R" in pit_and_load(agent, "Setup R")
    stint(agent, 1 + LAPS + 1)  # one lap more than the block needs

    assert agent.mock["setup"]["ARB_FRONT"] == soft["ARB_FRONT"]
    expected = {(block, lap): CORNERS for block in (1, 2) for lap in range(LAPS + 1)}
    assert tags(agent) == expected | {None: CORNERS}, "the extra lap belongs to no block"
    lines = agent.lines()
    for record in lines:
        jsonschema.Draft7Validator(schema(f"spool_{record['kind']}")).validate(record)
    reported = [record["values"]["ARB_FRONT"] for record in lines if record["kind"] == "setup"]
    assert reported == [base["ARB_FRONT"], soft["ARB_FRONT"]]
    hashes = {p["block"]["order"]: p["setup_hash"] for p in agent.passes() if "block" in p}
    assert hashes[1] != hashes[2], "each block's passes carry the fingerprint of its own setup"
    assert all(p["block"]["experiment_id"] == 12 for p in agent.passes() if "block" in p)


def test_passes_stay_untagged_until_the_blocks_setup_is_on_the_car(agent: Agent) -> None:
    base, soft = setups(agent)
    write_plan(agent, [("Setup K", soft, LAPS), ("Setup R", base, LAPS)])
    agent.drive.pit(True)
    agent.drive.idle(130)
    agent.drive.pit(False)  # the driver leaves without loading Setup K

    stint(agent, 2)

    assert tags(agent) == {None: 2 * CORNERS}
    assert "pit and load Setup K" in agent.rt.eval('require("game.plan").status()')


def test_a_value_that_does_not_take_undoes_the_whole_load(agent: Agent) -> None:
    base, soft = setups(agent)
    changed = soft | {"ARB_REAR": base["ARB_REAR"] - 1}
    write_plan(agent, [("Setup K", changed, LAPS)])
    agent.mock["reject"] = agent.rt.table_from({"ARB_REAR": True})  # the game ignores this one

    pit_and_load(agent, "Setup K")
    stint(agent, 2)

    assert to_py(agent.mock["setup"]) == base, "the car is back on the setup it had"
    assert "every change was undone" in agent.rt.eval('require("game.plan").message')
    assert tags(agent) == {None: 2 * CORNERS}


def test_load_button_is_only_offered_in_the_setup_screen(agent: Agent) -> None:
    base, _ = setups(agent)
    write_plan(agent, [("Setup K", base, LAPS)])
    agent.drive.pit(True)
    agent.drive.idle(130)

    assert window(agent) == [], "no button while the setup cannot be edited"
    agent.mock["editable"] = True
    assert window(agent) == ["Load Setup K"]
    agent.mock["clicks"]["Load Setup K"] = True
    window(agent)
    assert window(agent) == [], "nothing to load once the setup is on the car"


def test_extended_plan_continues_and_a_restore_block_ends_it(agent: Agent) -> None:
    base, soft = setups(agent)
    write_plan(agent, [("Setup K", base, 1), ("Setup R", soft, 1)])
    pit_and_load(agent, "Setup K")
    stint(agent, 2)
    pit_and_load(agent, "Setup R")
    stint(agent, 2)
    plan = agent.rt.eval('require("game.plan")')
    assert plan.status() == "Experiment 12: run plan complete."

    # the verdict was rejected: core appends a block that only puts Setup K back
    write_plan(
        agent, [("Setup K", base, 1), ("Setup R", soft, 1), ("Setup K", base, 0)], revision=2
    )
    buttons = pit_and_load(agent, "Setup K")
    stint(agent, 1)

    assert "Load Setup K" in buttons
    assert agent.mock["setup"]["ARB_FRONT"] == base["ARB_FRONT"]
    assert plan.status() == "Experiment 12: run plan complete."
    assert tags(agent) == {
        (1, 0): CORNERS,
        (1, 1): CORNERS,
        (2, 0): CORNERS,
        (2, 1): CORNERS,
        None: CORNERS,
    }


def test_position_is_written_down_only_when_it_changes(agent: Agent) -> None:
    base, _ = setups(agent)
    write_plan(agent, [("Setup K", base, LAPS)])
    pit_and_load(agent, "Setup K")

    stint(agent, 2)

    state_writes = [w for w in agent.writes() if w["path"].endswith("agent_state.json")]
    state = json.loads((agent.spool_root.parent / "agent_state.json").read_text(encoding="utf-8"))
    assert state == {"experiment_id": 12, "order": 1, "push_done": 1}
    assert 1 <= len(state_writes) <= 4, "a few writes for one stint, not one per frame"
    assert all(w["async"] for w in state_writes)


def test_a_broken_run_plan_is_ignored_and_recording_goes_on(agent: Agent) -> Any:
    outbox = agent.spool_root.parent / "outbox"
    outbox.mkdir(parents=True)
    (outbox / "runplan.json").write_text('{"v": 1, "blocks": "none"', encoding="utf-8")
    agent.drive.pit(True)
    agent.drive.idle(130)
    agent.drive.pit(False)

    stint(agent, 1)

    assert "Run plan ignored" in agent.rt.eval('require("game.plan").message')
    assert tags(agent) == {None: CORNERS}
