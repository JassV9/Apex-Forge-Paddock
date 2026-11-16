"""The setup desk in the agent's window: core's answers turned into lines and buttons, and the
requests a click sends. Core is faked in Python; nothing here was run in Assetto Corsa."""

import json
from typing import Any

from conftest import Agent
from harness import to_lua, to_py

PROPOSALS = {
    "stage": "cold_start",
    "skipped": [],
    "ranked": [
        {
            "parameter": "WING_1",
            "label": "Rear wing",
            "clicks": 2,
            "why": "more rear grip",
            "hypotheses": ["entry_oversteer.fast"],
            "basis": "none",
            "prior_source": "",
            "expected_gain": 0.01,
            "gain_low": 0.004,
            "gain_high": 0.03,
            "laps_needed": 16,
            "value_per_lap": 0.001,
        },
        {
            "parameter": "ARB_FRONT",
            "label": "",
            "clicks": -1,
            "why": "less understeer",
            "hypotheses": [],
            "basis": "none",
            "prior_source": "",
            "expected_gain": 0.004,
            "gain_low": -0.02,
            "gain_high": 0.02,
            "laps_needed": 20,
            "value_per_lap": 0.0002,
        },
    ],
}


def experiment(id_: int, state: str, car: str = "Mazda MX-5 Cup", **extra: Any) -> dict[str, Any]:
    blocks = [
        {"order": o, "label": "K", "laps_target": 3, "laps_done": 3, "passes": 21} for o in (1, 2)
    ]
    return {
        "id": id_,
        "state": state,
        "car_name": car,
        "parameter": "WING_1",
        "parameter_label": "Rear wing",
        "clicks": 2,
        "driver": "ai",
        "blocks": blocks,
    } | extra


def desk(rt: Any) -> Any:
    return rt.eval('require("core.desk")')


def test_proposals_read_as_titles_with_the_laps_they_need(rt):
    lines = to_py(desk(rt).proposals(to_lua(rt, PROPOSALS)))

    assert [line["title"] for line in lines] == ["Rear wing +2 clicks", "ARB_FRONT -1 click"]
    assert lines[0]["detail"] == "more rear grip. About 16 laps."
    assert lines[0]["hypothesis"] == "entry_oversteer.fast" and "hypothesis" not in lines[1]


def test_the_request_predicts_a_gain_only_when_the_ranking_expects_one(rt):
    d = desk(rt)
    lines = d.proposals(to_lua(rt, PROPOSALS))

    sure = to_py(d.createBody(7, lines[1], "ai"))
    unsure = to_py(d.createBody(7, lines[2], "human"))

    assert sure["predictions"] == [
        {"channel": "corner_time_s", "direction": "decrease", "low": 0.004, "high": 0.03}
    ]
    assert sure["driver"] == "ai" and sure["hypothesis_key"] == "entry_oversteer.fast"
    assert unsure["predictions"] in ([], {}) and "hypothesis_key" not in unsure
    assert unsure["statement"] == "Test: less understeer."


def test_open_experiments_of_this_car_show_their_laps_and_when_to_judge(rt):
    experiments = [
        experiment(3, "running"),
        experiment(4, "proposed", blocks=[]),
        experiment(5, "running", car="Ferrari 488 GT3"),
        experiment(2, "accepted", verdict={"reason": "Faster by 0.04 s."}),
    ]

    lines = to_py(desk(rt).openExperiments(to_lua(rt, experiments), "Mazda MX-5 Cup"))
    verdict = desk(rt).lastVerdict(to_lua(rt, experiments), "Mazda MX-5 Cup")

    assert [(e["id"], e["lapsDone"], e["lapsTarget"], e["readyToJudge"]) for e in lines] == [
        (4, 0, 0, False),
        (3, 6, 6, True),
    ]
    assert lines[1]["title"] == "#3 Rear wing +2"
    assert verdict == "#2 accepted: Faster by 0.04 s."


def test_the_core_session_is_found_by_the_spool_id(rt):
    sessions = to_lua(rt, [{"id": 4, "spool_id": "a"}, {"id": 9, "spool_id": "b"}])
    assert desk(rt).sessionId(sessions, "b") == 9
    assert desk(rt).sessionId(sessions, "c") is None


# The window against a fake core ------------------------------------------------------------


class FakeCore:
    def __init__(self, agent: Agent) -> None:
        self.agent = agent
        self.requests: list[tuple[str, str, Any]] = []
        self.experiments: list[dict[str, Any]] = []
        self.training: list[dict[str, Any]] = []

    def __call__(self, url: str, headers: Any, body: Any) -> tuple[int, str]:
        path = url.removeprefix("http://127.0.0.1:8080")
        self.requests.append(("POST" if body is not None else "GET", path, body))
        spool = self.agent.rt.eval('require("game.agent").sessionId')
        answers: dict[str, Any] = {
            "/sessions": [{"id": 11, "spool_id": spool}],
            "/experiments": self.experiments,
            "/training": self.training,
            "/sessions/11/proposals": PROPOSALS,
            "/sessions/11/training": {"id": 3, "message": "Experiment 21 is on the car."},
            "/training/3/stop": {"id": 3, "state": "stopped"},
        }
        if path == "/experiments" and body is not None:
            return 201, json.dumps(experiment(21, "proposed"))
        if path.startswith("/experiments/21/"):
            return 200, json.dumps(experiment(21, path.rsplit("/", 1)[1] + "ned"))
        return 200, json.dumps(answers.get(path, {}))


def draw(agent: Agent) -> list[str]:
    agent.mock["seen"] = agent.rt.table()
    agent.rt.eval("script.windowMain")()
    return list(to_py(agent.mock["seen"]) or [])


def test_the_window_offers_the_proposals_and_starts_one_for_the_ai(agent: Agent) -> None:
    core = FakeCore(agent)
    agent.mock["web_post"] = core
    agent.mock["physicsAllowed"] = True

    buttons = draw(agent)
    assert "I drive it##1" in buttons and "AI drives it##1" in buttons

    agent.mock["clicks"]["AI drives it##1"] = True
    draw(agent)

    posts = [
        (path, json.loads(body) if body else None)
        for verb, path, body in core.requests
        if verb == "POST"
    ]
    assert [path for path, _ in posts] == [
        "/experiments",
        "/experiments/21/plan",
        "/experiments/21/start",
    ]
    assert posts[0][1]["driver"] == "ai" and posts[0][1]["parameter"] == "WING_1"


def posted(core: FakeCore) -> list[tuple[str, Any]]:
    return [
        (path, json.loads(body) if body else None)
        for verb, path, body in core.requests
        if verb == "POST"
    ]


def test_the_window_starts_ai_training_and_shows_it_until_stopped(agent: Agent) -> None:
    core = FakeCore(agent)
    agent.mock["web_post"] = core
    agent.mock["physicsAllowed"] = True
    assert "Start training" in draw(agent)

    agent.mock["clicks"]["Start training"] = True
    draw(agent)
    assert posted(core) == [("/sessions/11/training", {"experiments": 4})]

    agent.mock["clicks"]["Start training"] = False
    core.training = [
        {
            "id": 3,
            "car_name": "Mazda MX-5 Cup",
            "state": "running",
            "target": 4,
            "message": "The AI is driving experiment 21.",
            "steps": [{"experiment_id": 21}],
        },
    ]
    agent.rt.eval('require("game.desk_link").refresh')(99, None, "Mazda MX-5 Cup", True)
    assert "Stop training" in draw(agent)
    agent.mock["clicks"]["Stop training"] = True
    draw(agent)
    assert posted(core)[-1] == ("/training/3/stop", None)


def test_only_this_cars_open_program_is_shown(rt):
    programs = to_lua(
        rt,
        [
            {
                "id": 1,
                "car_name": "Ferrari 488 GT3",
                "state": "done",
                "target": 2,
                "steps": [{}, {}],
            },
            {
                "id": 2,
                "car_name": "Ferrari 488 GT3",
                "state": "waiting",
                "target": 4,
                "message": "trade-off",
                "steps": [{}],
            },
            {"id": 5, "car_name": "Mazda MX-5 Cup", "state": "running", "target": 4, "steps": []},
        ],
    )
    program = to_py(desk(rt).training(programs, "Ferrari 488 GT3"))
    assert program == {"id": 2, "state": "waiting", "message": "trade-off", "done": 1, "target": 4}
    assert desk(rt).training(programs, "Lotus 49") is None


def test_the_window_asks_core_at_most_every_ten_seconds(agent: Agent) -> None:
    core = FakeCore(agent)
    agent.mock["web_post"] = core
    for _ in range(120):  # two seconds of frames with the window open
        draw(agent)
        agent.drive.idle(1)

    assert len([r for r in core.requests if r[1] == "/sessions"]) == 1


def test_without_core_the_window_says_so_and_recording_goes_on(agent: Agent) -> None:
    def down(url: str, headers: Any, body: Any) -> tuple[int, str]:
        raise ConnectionError("refused")

    agent.mock["web_post"] = down
    draw(agent)
    message = agent.rt.eval('require("game.desk_link").message')
    assert "not answering" in message or "not available" in message
    assert agent.rt.eval('require("game.agent").ready') is True
