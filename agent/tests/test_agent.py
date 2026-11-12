"""The whole agent on the mocked CSP API: PaddockAgent.lua driven around the synthetic track.

Everything here is offline. Nothing in this file shows that the agent works in Assetto Corsa.
"""

import itertools
from collections import Counter

import jsonschema

from conftest import Agent
from test_spool_format import schema

CORNERS = 7
MIN_WRITE_GAP_S = 0.2  # at most 5 spool writes per second


def drive_laps(agent: Agent, laps: int) -> None:
    agent.drive.run(agent.rt.table_from({"laps": laps}))
    agent.drive.idle(40)  # let the last pass finish encoding and reach the disk


def test_two_laps_produce_a_valid_spool(agent: Agent) -> None:
    drive_laps(agent, 2)

    lines = agent.lines()
    for record in lines:
        jsonschema.Draft7Validator(schema(f"spool_{record['kind']}")).validate(record)
    session, passes = lines[0], agent.passes()
    assert [record["kind"] for record in lines[:3]] == ["session", "catalog", "setup"]
    assert Counter(record["corner"] for record in passes) == {n: 2 for n in range(1, CORNERS + 1)}
    assert [record["seq"] for record in passes] == list(range(1, 2 * CORNERS + 1))
    assert {record["session_id"] for record in lines} == {session["session_id"]}
    assert len(session["track"]["corners"]) == CORNERS


def test_pass_records_carry_conditions_and_raw_samples(agent: Agent) -> None:
    agent.mock["car"]["fuel"] = 37.5
    agent.mock["car"]["lapCount"] = 3
    drive_laps(agent, 1)

    record = agent.passes()[0]

    assert record["conditions"]["fuel_l"] == 37.5
    assert record["conditions"]["tyre_core_c"] == [90, 90, 90, 90]
    assert record["conditions"]["grip"] == 0.97
    assert record["lap_in_session"] == 3
    assert record["setup_hash"]
    columns = record["samples"]["columns"]
    assert all(len(column) == record["samples"]["n"] for column in columns.values())
    assert columns["t"][0] >= 0
    assert abs(columns["t"][-1] - record["duration_s"]) < 0.02
    sample_rate = (record["samples"]["n"] - 1) / columns["t"][-1]
    assert 50 <= sample_rate <= 61
    assert max(columns["load_rl"]) == 3400  # per-wheel values, not axle averages


def test_spool_is_written_at_most_five_times_per_second_and_only_in_its_folder(
    agent: Agent,
) -> None:
    drive_laps(agent, 2)

    writes = agent.writes()
    gaps = [later["at"] - earlier["at"] for earlier, later in itertools.pairwise(writes)]
    assert len(writes) >= CORNERS
    assert min(gaps) >= MIN_WRITE_GAP_S
    assert all(write["async"] for write in writes)
    assert all("/paddock/spool/" in write["path"] for write in writes)


def test_restart_teleport_pits_and_replay_never_leave_a_partial_pass(agent: Agent) -> None:
    drive_laps(agent, 1)
    clean = {record["corner"]: record["duration_s"] for record in agent.passes()}

    def drive_into_a_corner() -> None:
        agent.drive.run(agent.rt.table_from({"laps": 1, "stopAt": agent.drive.t + 9}))

    drive_into_a_corner()
    agent.drive.restartSession()
    drive_into_a_corner()
    agent.drive.teleport(0)
    drive_into_a_corner()
    agent.drive.pit(True)
    agent.drive.pit(False)
    drive_into_a_corner()
    agent.mock["sim"]["isReplayActive"] = True
    agent.drive.idle(5)
    agent.mock["sim"]["isReplayActive"] = False
    drive_laps(agent, 1)

    later = agent.passes()[len(clean) :]
    assert len(later) >= CORNERS
    for record in later:
        expected = clean[record["corner"]]
        assert 0.8 * expected < record["duration_s"] < 1.2 * expected
    aborts = dict(agent.drive.agent["rec"]["aborts"].items())
    assert {"session restart", "car reset", "not driving", "replay"} <= set(aborts)


def test_setup_menu_and_current_setup_are_written_once_and_on_change(agent: Agent) -> None:
    drive_laps(agent, 1)
    agent.drive.pit(True)
    agent.mock["setup"]["ARB_FRONT"] = agent.mock["setup"]["ARB_FRONT"] + 1
    agent.drive.pit(False)
    drive_laps(agent, 1)

    lines = agent.lines()
    catalogs = [record for record in lines if record["kind"] == "catalog"]
    setups = [record for record in lines if record["kind"] == "setup"]
    assert len(catalogs) == 1
    parameters = {item["name"]: item for item in catalogs[0]["parameters"]}
    assert parameters["ARB_FRONT"]["role"] == "arb" and parameters["ARB_FRONT"]["axle"] == "F"
    assert (parameters["CAMBER_LF"]["min"], parameters["CAMBER_LF"]["step"]) == (-38, 1)
    assert len(setups) == 2
    first, second = setups
    assert second["values"]["ARB_FRONT"] == first["values"]["ARB_FRONT"] + 1
    assert first["setup_hash"] != second["setup_hash"]
    hashes = [record["setup_hash"] for record in agent.passes()]
    assert hashes[0] == first["setup_hash"] and hashes[-1] == second["setup_hash"]


def test_ai_controlled_car_is_not_recorded(agent: Agent) -> None:
    agent.mock["car"]["isAIControlled"] = True

    drive_laps(agent, 1)

    assert agent.passes() == []


def test_shutdown_flushes_what_is_still_waiting(agent: Agent) -> None:
    agent.drive.run(agent.rt.table_from({"laps": 1, "untilPasses": 3}))
    assert len(agent.passes()) == 2  # the third pass is still being encoded

    agent.drive.release()

    assert len(agent.passes()) == 3
    assert agent.writes()[-1]["async"] is False  # the last write blocks: async may never finish


def test_frame_cost_stays_under_budget(agent: Agent) -> None:
    drive_laps(agent, 6)

    timing = dict(agent.drive.timing().items())

    assert timing["frames"] > 10_000
    assert timing["meanMs"] < 0.05, timing
