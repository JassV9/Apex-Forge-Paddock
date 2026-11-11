"""Corner map on a real AI line. Needs a local Assetto Corsa install with the track, so it is
skipped in CI; the synthetic track covers the same code everywhere else.

The AI line reader is ported from Apex Forge v0 `tests/harness.py`.
"""

import math
import os
import struct
from pathlib import Path
from typing import Any

import pytest

from harness import load_fixture, to_lua, to_py

AC_ROOT = Path(
    os.environ.get("AC_ROOT", r"C:\Program Files (x86)\Steam\steamapps\common\assettocorsa")
)
COTA_LINE = AC_ROOT / "content/tracks/cota_2022/layout_f1_2025/ai/fast_lane.ai"

pytestmark = pytest.mark.skipif(not COTA_LINE.is_file(), reason="COTA is not installed here")

Point = tuple[float, float]


def read_ai_line(path: Path) -> tuple[list[Point], list[float], float]:
    """Points (x, z), their distance from the start, and the closed lap length."""
    data = path.read_bytes()
    _version, count, _lap, _samples = struct.unpack_from("<4i", data, 0)
    points, distances = [], []
    for i in range(count):
        x, _y, z, distance, _index = struct.unpack_from("<4fi", data, 16 + i * 20)
        points.append((x, z))
        distances.append(distance)
    closing = math.hypot(points[0][0] - points[-1][0], points[0][1] - points[-1][1])
    return points, distances, distances[-1] + closing


def resample(points: list[Point], distances: list[float], length: float, count: int) -> list[Point]:
    """Even spacing along the line, as ac.trackProgressToWorldCoordinate gives in game."""
    out = []
    j = 0
    for i in range(count):
        target = length * i / count
        while j + 1 < len(points) and distances[j + 1] <= target:
            j += 1
        a = points[j]
        b = points[j + 1] if j + 1 < len(points) else points[0]
        span = (distances[j + 1] if j + 1 < len(points) else length) - distances[j]
        share = 0.0 if span <= 0 else (target - distances[j]) / span
        out.append((a[0] + (b[0] - a[0]) * share, a[1] + (b[1] - a[1]) * share))
    return out


def test_cota_corner_map_matches_the_one_v0_recorded_in_game(rt: Any) -> None:
    expected = load_fixture("real_cota_map.json")
    points, distances, length = read_ai_line(COTA_LINE)
    count = max(300, min(3000, int(expected["L"] / 4)))  # as agent.lua samples it in game
    even = resample(points, distances, length, count)

    # In game the lap length comes from the sim, not from summing the line's segments: they
    # differ by a few metres on this track, and the map is built with the sim's value.
    built = rt.eval('require("core.corners")').build(
        to_lua(rt, [{"x": x, "z": z} for x, z in even]), expected["L"]
    )

    corners = to_py(built["list"])
    assert abs(length - expected["L"]) / expected["L"] < 0.005
    assert [c["name"] for c in corners] == [c["name"] for c in expected["list"]]
    for ours, theirs in zip(corners, expected["list"], strict=True):
        assert ours["dir"] == theirs["dir"]
        assert abs(ours["apex"] - theirs["apex"]) < 4.5  # one sample step
        assert abs(ours["zStart"] - theirs["zStart"]) < 2.0
