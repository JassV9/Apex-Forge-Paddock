"""Corner pass recorder: complete passes only. Restarts, teleports, pit visits and replays drop
the pass in progress instead of recording a truncated one.

Ported from Apex Forge v0 `tests/test_core.py` (test_restarts), on a synthetic track.
"""

from typing import Any

import pytest

from conftest import CAR, lua_points
from harness import to_lua, to_py
from track import Track

PassTimes = dict[int, list[float]]


class Loop:
    """sim -> recorder, the two real Lua modules, with nothing in between."""

    def __init__(self, rt: Any, track: Track, corner_map: Any) -> None:
        self.rt = rt
        sim = rt.eval('require("sim")')
        self.sim = sim
        self.line = sim.track(lua_points(rt, track), track.length)
        self.speeds = sim.profile(self.line, to_lua(rt, CAR))
        self.rec = rt.eval('require("core.recorder")').new(corner_map)
        self.times: PassTimes = {}
        self.sample_counts: list[int] = []
        self.clock = 0.0
        self.driving = True

    def _feed(self, sample: Any) -> None:
        finished = self.rec.feed(self.rec, sample, self.driving)
        if finished is not None:
            corner = int(finished["corner"]["id"])
            self.times.setdefault(corner, []).append(finished["tOut"] - finished["tIn"])
            self.sample_counts.append(int(finished["n"]))

    def drive(self, **options: Any) -> float:
        options = {"car": CAR, "t0": self.clock, **options}
        stopped_at, _ = self.sim.drive(self.line, self.speeds, to_lua(self.rt, options), self._feed)
        self.clock = stopped_at + 1.0
        return float(stopped_at)

    def aborts(self) -> dict[str, int]:
        return to_py(self.rec["aborts"]) or {}

    def total(self) -> int:
        return sum(len(times) for times in self.times.values())


@pytest.fixture
def loop(rt: Any, track: Track, corner_map: Any) -> Loop:
    return Loop(rt, track, corner_map)


def test_clean_laps_record_every_corner_each_lap(loop: Loop, corner_map: Any) -> None:
    corner_count = len(to_py(corner_map["list"]))

    loop.drive(laps=3)

    assert sorted(loop.times) == list(range(1, corner_count + 1))
    # the first corner's zone starts after the start line, so all three laps count
    assert all(len(times) == 3 for times in loop.times.values())
    assert all(count >= 2 for count in loop.sample_counts)


def test_restarts_mid_corner_never_record_a_partial_pass(loop: Loop) -> None:
    loop.drive(laps=2)
    best = {corner: min(times) for corner, times in loop.times.items()}
    before = loop.total()

    for attempt in range(10):
        loop.drive(laps=1, stopAt=loop.clock + 12 + attempt * 2)
        loop.rec.reset(loop.rec, "jump")

    new_times = {corner: times[2:] for corner, times in loop.times.items() if len(times) > 2}
    assert loop.total() > before
    for corner, times in new_times.items():
        assert all(0.8 * best[corner] < time < 1.2 * best[corner] for time in times)
    assert sum(loop.aborts().values()) >= 5


def test_teleport_drops_the_pass_in_progress(loop: Loop, corner_map: Any) -> None:
    first = to_py(corner_map["list"])[0]
    far_away = (first["zEnd"] + 600) % corner_map["L"]

    loop.drive(laps=1, startD=first["zStart"] - 60, stopAt=loop.clock + 2.5)  # ends in the zone
    loop.drive(laps=1, startD=far_away, stopAt=loop.clock + 1.0)

    assert loop.aborts().get("jump") == 1
    assert 1 not in loop.times


@pytest.mark.parametrize("reason", ["not driving"])
def test_pits_and_replay_samples_drop_the_pass(loop: Loop, reason: str) -> None:
    loop.drive(laps=1, stopAt=loop.clock + 6)
    loop.driving = False
    loop.drive(laps=1, stopAt=loop.clock + 1)
    loop.driving = True
    recorded_while_stopped = loop.total()

    loop.drive(laps=1)

    assert loop.aborts().get(reason, 0) >= 1
    assert loop.total() > recorded_while_stopped


def test_going_backwards_drops_the_pass(rt: Any, corner_map: Any) -> None:
    rec = rt.eval('require("core.recorder")').new(corner_map)
    first = to_py(corner_map["list"])[0]
    start = first["zStart"]
    for step, distance in enumerate([start - 5, start + 1, start + 3, start - 2]):
        finished = rec.feed(rec, to_lua(rt, {"t": step / 60, "d": distance, "v": 30}), True)
        assert finished is None

    assert to_py(rec["aborts"]) == {"backwards": 1}
