"""The hotlap window: a corner pass against your best and the AI on the same setup, what its
samples show, and the requests it sends to a faked core. Nothing here ran in Assetto Corsa."""

import json
from typing import Any
from urllib.parse import parse_qs, urlparse

import pytest

from conftest import Agent
from harness import to_lua, to_py

CHANNELS = ("brake", "gas", "sr_fl", "sr_fr", "sr_rl", "sr_rr", "out")


def raw_pass(rt: Any, samples: list[dict[str, float]]) -> Any:
    """A recorder pass whose samples default to coasting straight, clean."""
    cols = {name: [sample.get(name, 0) for sample in samples] for name in CHANNELS}
    corner = {"id": 1, "name": "T1"}
    return to_lua(rt, {"n": len(samples), "cols": cols, "corner": corner, "tIn": 0, "tOut": 5})


def hotlap(rt: Any) -> Any:
    return rt.eval('require("core.hotlap")')


BRAKING = {"brake": 0.8}
POWER = {"gas": 0.9}


@pytest.mark.parametrize(
    ("samples", "tags"),
    [
        ([BRAKING] * 10 + [POWER] * 10, []),
        ([BRAKING | {"sr_fl": -0.3}] * 3, ["front lock"]),
        ([BRAKING | {"sr_fr": -0.3}] * 2 + [BRAKING] * 4, []),  # 2 samples: too short
        ([BRAKING | {"sr_rr": -0.25}] * 4, ["rear lock"]),
        ([POWER | {"sr_rl": 0.3}] * 5, ["wheelspin"]),
        ([{"sr_rl": 0.3}] * 5, []),  # neither braking nor on the throttle
        (
            [BRAKING | {"out": 4}, BRAKING | {"sr_fl": 0.2}, BRAKING | {"sr_fl": 0.2}] * 2,
            ["off track"],
        ),
        (
            [BRAKING | {"sr_fl": 0.3, "out": 3}] * 3 + [POWER | {"sr_rr": 0.4}] * 3,
            ["off track", "front lock", "wheelspin"],
        ),
    ],
)
def test_the_verdict_names_only_what_the_samples_show(rt, samples, tags) -> None:
    got = to_py(hotlap(rt).verdict(raw_pass(rt, samples)))

    assert (got or []) == tags


REFERENCE = {
    "corners": [
        {"corner": 1, "name": "T1", "n": 9, "median_s": 5.0, "spread_s": 0.003, "margin_s": 0.01}
    ]
}


def test_a_pass_is_timed_against_your_best_and_the_ai_on_the_same_setup(rt) -> None:
    state = hotlap(rt).new()
    state.setReference(state, "s1", to_lua(rt, REFERENCE))

    first = to_py(state["pass"](state, 1, "T1", 5.20, "s1", to_lua(rt, []), 10))
    second = to_py(state["pass"](state, 1, "T1", 5.10, "s1", to_lua(rt, ["front lock"]), 20))
    other = to_py(state["pass"](state, 1, "T1", 5.30, "s2", to_lua(rt, []), 30))

    assert "deltaBest" not in first and first["deltaAI"] == pytest.approx(0.2)
    assert second["deltaBest"] == pytest.approx(-0.1) and second["verdict"] == "front lock"
    assert "deltaBest" not in other and "deltaAI" not in other  # a new setup starts afresh
    assert hotlap(rt).against(0.2, 0.01) == "driver"
    assert hotlap(rt).against(-0.2, 0.01) == "ahead"
    assert hotlap(rt).against(0.005, 0.01) == "even"


def test_the_last_pass_fades_after_six_seconds(rt) -> None:
    state = hotlap(rt).new()
    assert state.alpha(state, 0) == 0
    state["pass"](state, 1, "T1", 5.0, "s1", to_lua(rt, []), 100)

    assert [state.alpha(state, t) for t in (100, 104.9)] == [1, 1]
    assert state.alpha(state, 105.5) == pytest.approx(0.5)
    assert state.alpha(state, 106) == 0


def test_the_corners_table_lists_best_ai_and_delta_per_corner(rt) -> None:
    state = hotlap(rt).new()
    reference = {
        "corners": REFERENCE["corners"]
        + [{"corner": 3, "name": "T3", "n": 4, "median_s": 2.0, "spread_s": 0, "margin_s": 0.01}]
    }
    state.setReference(state, "s1", to_lua(rt, reference))
    state["pass"](state, 1, "T1", 5.25, "s1", to_lua(rt, []), 1)
    state["pass"](state, 2, "T2", 3.0, "s1", to_lua(rt, []), 2)
    state["pass"](state, 2, "T2", 2.5, "s9", to_lua(rt, []), 3)

    rows = to_py(state.rows(state, "s1"))

    assert [row["name"] for row in rows] == ["T1", "T2", "T3"]
    assert rows[0]["delta"] == pytest.approx(0.25) and rows[0]["best"] == 5.25
    assert rows[1]["best"] == 3.0 and "ai" not in rows[1]
    assert "best" not in rows[2] and rows[2]["ai"] == 2.0


def test_the_reference_path_escapes_ids(rt) -> None:
    path = hotlap(rt).referencePath("ks_ferrari_488_gt3", "fn cota", "", "s1&x")

    assert path == "/reference?car=ks_ferrari_488_gt3&track=fn%20cota&layout=&setup_hash=s1%26x"


# The window on the running agent ---------------------------------------------------------------


class FakeCore:
    def __init__(self, reply: dict[str, Any] | None = None, status: int = 200) -> None:
        self.reply, self.status = reply, status
        self.asked: list[dict[str, list[str]]] = []

    def __call__(self, url: str, headers: Any, body: Any) -> tuple[int, str]:
        parsed = urlparse(url)
        assert parsed.path == "/reference" and body is None
        self.asked.append(parse_qs(parsed.query, keep_blank_values=True))
        return self.status, json.dumps(self.reply)


def draw(agent: Agent) -> int:
    """Draw the hotlap window once; returns how many lines of text it drew."""
    before = agent.mock["texts"]
    agent.rt.eval("script.windowHotlap")()
    return agent.mock["texts"] - before


def drive_lap(agent: Agent) -> None:
    agent.drive.run(agent.rt.table_from({"laps": 1}))


def test_the_window_shows_each_pass_and_asks_core_once_per_setup(agent: Agent) -> None:
    hash_ = agent.rt.eval('require("game.agent").setupHash')
    core = FakeCore(
        {
            "corners": [
                {
                    "corner": 1,
                    "name": "T1",
                    "n": 5,
                    "median_s": 1.0,
                    "spread_s": 0.002,
                    "margin_s": 0.01,
                }
            ]
        }
    )
    agent.mock["web_post"] = core

    for _ in range(120):  # two seconds of frames with the window open
        draw(agent)
        agent.drive.idle(1)
    assert len(core.asked) == 1
    drive_lap(agent)
    agent.mock["clicks"]["Corners table"] = True
    lines = draw(agent)

    assert len(core.asked) == 2  # the lap took more than thirty seconds
    assert core.asked[0] == {
        "car": ["ks_mazda_mx5_cup"],
        "track": ["synthetic"],
        "layout": [""],
        "setup_hash": [hash_],
    }
    state = to_py(agent.rt.eval('require("game.hotlap").state'))
    assert state["last"]["name"].startswith("T") and state["last"]["time"] > 0
    assert state["best"][f"{hash_}#1"] > 1.0  # T1 was timed, and the AI's 1.0 s is known
    assert lines >= 1 + 1 + 7  # the last pass, the table header and its seven corners


def test_the_window_asks_again_after_thirty_seconds(agent: Agent) -> None:
    core = FakeCore({"corners": []})
    agent.mock["web_post"] = core
    draw(agent)
    agent.drive.idle(29 * 60)
    draw(agent)
    agent.drive.idle(2 * 60)
    draw(agent)

    assert len(core.asked) == 2
    assert agent.rt.eval('require("game.hotlap_link").message').startswith("The AI has not")


def test_nothing_is_shown_or_asked_while_the_ai_drives(agent: Agent) -> None:
    core = FakeCore({"corners": []})
    agent.mock["web_post"] = core
    agent.rt.eval('require("game.agent")').source = "ai"

    assert draw(agent) == 0 and core.asked == []


def test_without_core_the_window_says_so_and_recording_goes_on(agent: Agent) -> None:
    def down(url: str, headers: Any, body: Any) -> tuple[int, str]:
        raise ConnectionError("refused")

    agent.mock["web_post"] = down
    draw(agent)
    drive_lap(agent)

    assert "not available" in agent.rt.eval('require("game.hotlap_link").message')
    assert agent.rt.eval('require("game.agent").passes') == 7
    assert agent.rt.eval('require("game.hotlap_link").busy') is False
