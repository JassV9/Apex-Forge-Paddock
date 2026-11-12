"""Shared fixtures: the synthetic track, its corner map, and a booted agent on the mocked CSP."""

import json
import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import pytest

from harness import AGENT_DIR, TESTS_DIR, load_fixture, runtime, to_lua, to_py
from track import Track, synthetic_track

# A plausible single-seater for the synthetic driver (grip, braking and acceleration in g).
CAR = {"mu": 1.6, "b": 1.5, "a": 0.6, "vmax": 80, "wheelbase": 2.7, "ratio": 14}
MOCK_CAR = "ks_mazda_mx5_cup"


@pytest.fixture(scope="session")
def track() -> Track:
    return synthetic_track()


def lua_points(rt: Any, track: Track) -> Any:
    return to_lua(rt, [{"x": x, "z": z} for x, z in track.points])


@pytest.fixture
def rt() -> Any:
    runtime_ = runtime()
    runtime_.execute(f'package.path = "{TESTS_DIR.as_posix()}/?.lua;" .. package.path')
    return runtime_


@pytest.fixture
def corner_map(rt: Any, track: Track) -> Any:
    return rt.eval('require("core.corners")').build(lua_points(rt, track), track.length)


@dataclass
class Agent:
    """The real PaddockAgent.lua running against mock_csp.lua."""

    rt: Any
    mock: Any
    drive: Any
    spool_root: Path

    def chunks(self) -> list[Path]:
        return sorted(self.spool_root.glob("*/*.jsonl"))

    def lines(self) -> list[dict[str, Any]]:
        return [
            json.loads(line)
            for chunk in self.chunks()
            for line in chunk.read_text(encoding="utf-8").splitlines()
        ]

    def passes(self) -> list[dict[str, Any]]:
        return [line for line in self.lines() if line["kind"] == "pass"]

    def writes(self) -> list[dict[str, Any]]:
        return to_py(self.mock["writes"]) or []


def _fill_mock(rt: Any, root: Path, track: Track) -> Any:
    mock = to_lua(
        rt,
        {
            "points": [{"x": x, "z": z} for x, z in track.points],
            "L": track.length,
            "root": str(root),
            "spinners": load_fixture(f"spinners_{MOCK_CAR}.json")["spinners"],
            "tabs": {},
            "tyres": {},
            "carID": MOCK_CAR,
            "carName": "Mazda MX-5 Cup",
            "trackID": "synthetic",
            "layout": "",
            "trackName": "Synthetic test track",
            "appRoot": AGENT_DIR.as_posix(),
        },
    )
    mock["fs_exists"] = os.path.exists
    mock["fs_mkdir"] = lambda path: os.makedirs(path, exist_ok=True) or True
    mock["json_stringify"] = lambda table: json.dumps(to_py(table), allow_nan=False)
    mock["json_parse"] = lambda text: to_lua(rt, json.loads(text))
    return mock


def boot_agent(rt: Any, track: Track, documents: Path) -> Agent:
    """Start PaddockAgent.lua on the mocked CSP. `documents` stands in for the game's folder
    in Documents (Documents/Assetto Corsa)."""
    rt.globals().MOCK = _fill_mock(rt, documents, track)
    rt.execute(f'dofile("{TESTS_DIR.as_posix()}/mock_csp.lua")')
    drive = rt.eval('require("drive")')
    drive.boot()
    drive.profile(to_lua(rt, CAR))
    return Agent(rt=rt, mock=rt.globals().MOCK, drive=drive, spool_root=documents / "paddock/spool")


@pytest.fixture
def agent(rt: Any, track: Track, tmp_path: Path) -> Agent:
    return boot_agent(rt, track, tmp_path)
