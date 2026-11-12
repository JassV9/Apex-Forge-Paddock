"""Phase 3 acceptance: experiments end to end through the mocked CSP agent.

The real pieces run together: PaddockAgent.lua under LuaJIT on the mocked game, core (built
from source) ingesting its spool into PostgreSQL and Parquet, and the engine computing features
and verdicts. Only the game is simulated: a synthetic car whose grip and rear slip depend on
the front anti-roll bar, driven with lap-to-lap scatter.

Nothing here shows that the agent works in Assetto Corsa.
"""

import os
import random
import time
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import pytest

from conftest import CAR, TESTS_DIR, Agent, boot_agent
from e2e import stack
from harness import runtime, to_lua, to_py
from track import Track

CORNERS = 7
BASELINE_BAR = 3  # ARB_FRONT on the mocked car
GRIP_PER_CLICK = 0.05  # softer front bar: 5 % more grip per click, in this simulation only
REAR_SLIP_PER_CLICK = 1.5  # ...and 1.5 degrees more rear slip: the price of the grip
PACE_SIGMA = 0.001  # lap-to-lap scatter of the simulated driver
LOAD_FRAMES = 130  # frames in the pits: longer than the agent's outbox poll interval


@dataclass
class Rig:
    agent: Agent
    backend: stack.Stack
    documents: Path
    rng: random.Random

    def plan(self) -> Any:
        return self.agent.rt.eval('require("game.plan")')

    def bar(self) -> float:
        return float(self.agent.mock["setup"]["ARB_FRONT"])

    def _options(self, **extra: Any) -> Any:
        softer = BASELINE_BAR - self.bar()
        pace = 1.0 + self.rng.gauss(0.0, PACE_SIGMA)
        options = {"laps": 1, "pace": pace, "slip0": 3.0 + REAR_SLIP_PER_CLICK * softer}
        return self.agent.rt.table_from(options | extra)

    def stint(self, laps: int) -> None:
        """Whole laps on the setup that is on the car, then across the line towards the pits."""
        softer = BASELINE_BAR - self.bar()
        self.agent.drive.profile(
            to_lua(self.agent.rt, CAR | {"mu": CAR["mu"] * (1 + GRIP_PER_CLICK * softer)})
        )
        for _ in range(laps):
            self.agent.drive.run(self._options())
        self.agent.drive.run(self._options(stopAt=self.agent.drive.t + 1))
        self.agent.drive.idle(40)

    def follow_plan(self) -> list[str]:
        """Do what the agent's window says until the run plan is complete: pit, load the
        block's setup, drive its out-lap and push laps. Returns the code names loaded."""
        loaded = []
        while True:
            self.agent.drive.pit(True)
            self.agent.drive.idle(LOAD_FRAMES)
            label = str(self.plan().label())
            if label == "":
                self.agent.drive.pit(False)
                return loaded
            laps = int(
                self.agent.rt.eval('require("core.runplan")').block(self.plan().state).lapsTarget
            )
            self.agent.mock["editable"] = True
            self.agent.mock["clicks"]["Load " + label] = True
            self.agent.rt.eval("script.windowMain")()
            self.agent.mock["editable"] = False
            self.agent.drive.pit(False)
            loaded.append(label)
            if laps > 0:
                self.stint(1 + laps)

    def wait(self, what: str, check: Callable[[], Any], timeout: float = 90) -> Any:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            result = check()
            if result:
                return result
            time.sleep(0.3)
        raise AssertionError(f"timed out waiting for {what}\n{self.backend.tail()}")

    def run_experiment(self, parameter: str, clicks: int, statement: str) -> dict[str, Any]:
        """Propose, plan, start, drive every block and have the engine judge it."""
        session = self.backend.get("/sessions")[0]["id"]
        prediction = {"channel": "corner_time_s", "direction": "decrease", "low": 0.01, "high": 0.3}
        exp = self.backend.post("/experiments", {
            "session_id": session, "parameter": parameter, "clicks": clicks,
            "statement": statement, "predictions": [prediction],
        })  # fmt: skip
        number = exp["id"]
        assert exp["state"] == "proposed"
        planned = self.backend.post(f"/experiments/{number}/plan")
        assert planned["state"] == "planned" and len(planned["blocks"]) == 4
        assert planned["plan"]["mde_decide"] > planned["plan"]["mde_detect"] > 0
        assert self.backend.post(f"/experiments/{number}/start")["state"] == "running"
        assert len(self.follow_plan()) == 4

        def driven() -> bool:
            blocks = self.backend.get(f"/experiments/{number}")["blocks"]
            return all(b["passes"] == CORNERS * (b["laps_target"] + 1) for b in blocks)

        self.wait(f"the passes of experiment {number} to be ingested", driven)

        def judged() -> Any:
            status, body = self.backend.request("POST", f"/experiments/{number}/evaluate")
            assert status in (200, 409), f"evaluate -> {status}: {body}\n{self.backend.tail()}"
            return body if status == 200 else None

        return dict(self.wait(f"the verdict on experiment {number}", judged))


@pytest.fixture(scope="module")
def rig(tmp_path_factory: pytest.TempPathFactory, track: Track) -> Iterator[Rig]:
    reason = stack.missing()
    if reason and os.environ.get(stack.REQUIRED_ENV):
        pytest.fail(f"the end-to-end test cannot run: {reason}")
    if reason:
        pytest.skip(f"end-to-end test needs the whole stack: {reason}")
    documents = tmp_path_factory.mktemp("documents")
    (documents / "paddock" / "spool").mkdir(parents=True)
    backend = stack.start(tmp_path_factory.mktemp("backend"), documents)
    rt = runtime()
    rt.execute(f'package.path = "{TESTS_DIR.as_posix()}/?.lua;" .. package.path')
    try:
        yield Rig(boot_agent(rt, track, documents), backend, documents, random.Random(2026))
    finally:
        backend.stop()


def states(exp: dict[str, Any]) -> list[str]:
    return [event["to"] for event in exp["events"]]


def result(exp: dict[str, Any], channel: str) -> dict[str, Any]:
    return next(r for r in exp["results"] if r["channel"] == channel and r["corner_class"] == "all")


def test_three_experiments_through_the_whole_stack(rig: Rig) -> None:
    backend = rig.backend

    # A session on the baseline: the passes every experiment is proposed against.
    rig.stint(4)
    rig.wait(
        "the baseline session",
        lambda: [s for s in backend.get("/sessions") if s["passes"] == 4 * CORNERS],
    )

    # 1. A softer front bar is faster, but the rear slides more: a trade-off for the driver.
    softer = rig.run_experiment("ARB_FRONT", -1, "A softer front bar gives the front more grip.")
    assert softer["state"] == "tradeoff", softer["verdict"]
    time_effect, slip_effect = result(softer, "corner_time_s"), result(softer, "rear_slip_peak_deg")
    assert time_effect["ci_high"] < -0.005, "faster by more than the equivalence band"
    assert slip_effect["ci_low"] > 0.5, "the rear-slip guardrail broke"
    assert softer["verdict"]["prediction_hits"] == softer["verdict"]["prediction_total"] == 1
    setup_files = sorted((rig.documents / "paddock/outbox/setups/ks_mazda_mx5_cup").glob("*.ini"))
    assert len(setup_files) == 2, "core wrote one setup file per arm"

    accepted = backend.post(f"/experiments/{softer['id']}/decision", {"accept": True})
    assert accepted["state"] == "accepted" and accepted["verdict"]["decided_by"] == "driver"
    assert rig.bar() == BASELINE_BAR - 1, "the accepted variant is what is on the car"

    rolled_back = backend.post(f"/experiments/{softer['id']}/revert")
    baseline = next(arm["label"] for arm in rolled_back["arms"] if arm["is_baseline"])
    assert rig.follow_plan() == [baseline], "the agent is told to load the baseline again"
    assert rig.bar() == BASELINE_BAR
    assert states(rolled_back) == [
        "proposed", "planned", "running", "evaluating", "tradeoff", "accepted", "reverted",
    ]  # fmt: skip

    # 2. A planted regression: a stiffer front bar costs grip. It is rejected and undone.
    stiffer = rig.run_experiment("ARB_FRONT", 1, "A stiffer front bar sharpens turn-in.")
    assert states(stiffer)[-3:] == ["evaluating", "rejected", "reverted"], stiffer["verdict"]
    assert (
        stiffer["verdict"]["decision"] == "rejected" and stiffer["verdict"]["prediction_hits"] == 0
    )
    assert result(stiffer, "corner_time_s")["ci_low"] > 0.005, "slower by more than the band"
    rig.follow_plan()
    assert rig.bar() == BASELINE_BAR, "the baseline is back on the car"

    # 3. A knob that does nothing in this simulation: unclear, and the baseline is kept.
    nothing = rig.run_experiment("ARB_REAR", -1, "A softer rear bar calms the rear.")
    assert nothing["state"] == "inconclusive", nothing["verdict"]
    unclear = result(nothing, "corner_time_s")
    assert unclear["ci_low"] < 0 < unclear["ci_high"]
    reverted = backend.post(f"/experiments/{nothing['id']}/revert")
    assert reverted["state"] == "reverted" and reverted["verdict"]["decided_by"] == "driver"
    rig.follow_plan()
    assert to_py(rig.agent.mock["setup"])["ARB_REAR"] == 1

    # The ledger keeps all three, newest first, and every one says how it got where it is.
    ledger = backend.get("/experiments")
    assert [exp["state"] for exp in ledger] == ["reverted", "reverted", "reverted"]
    assert [exp["parameter"] for exp in ledger] == ["ARB_REAR", "ARB_FRONT", "ARB_FRONT"]
    assert all(event["reason"] for exp in ledger for event in exp["events"])
