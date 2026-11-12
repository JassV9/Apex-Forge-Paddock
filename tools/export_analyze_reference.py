"""Records what Apex Forge v0's analyze.lua computes, as the reference for the engine's port.

    uv run --project agent python tools/export_analyze_reference.py

Needs the v0 prototype on this machine (V0_ROOT, default C:\\Users\\Jason\\Desktop\\ApexForge);
it is only read and executed, never modified. v0's own synthetic driver (tests/sim.lua) drives
Paddock's synthetic track through v0's recorder and analyze.pass. For every pass the output
holds the raw samples in Paddock's column names and v0's results for it.

Output: engine/tests/fixtures/analyze_reference.json.gz (committed; CI does not need v0).
"""

import gzip
import json
import os
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "agent" / "tests"))

from lupa import luajit21 as lupa  # noqa: E402

from harness import to_lua, to_py  # noqa: E402
from track import synthetic_track  # noqa: E402

V0_ROOT = Path(os.environ.get("V0_ROOT", r"C:\Users\Jason\Desktop\ApexForge"))
OUTPUT = ROOT / "engine/tests/fixtures/analyze_reference.json.gz"
CAR = {"mu": 1.6, "b": 1.5, "a": 0.6, "vmax": 80, "wheelbase": 2.7, "ratio": 14}

# Two drivers: a trail-braker with entry understeer, and a tidy one who locks a front into the
# first corner and runs wide at the third.
DRIVERS = [
    {"laps": 1, "trail": 0.6, "balance": "entry_us"},
    {"laps": 1, "trail": 0.1, "pace": 0.92, "lock": 1, "off": 3},
]
SCALARS = [
    "time", "vEntry", "vMin", "vExit", "latPeak", "flat", "off", "brakeDist", "brakePeak",
    "attack", "trail", "release", "pickup", "toFull", "lifts", "coast", "corr", "cs", "csExit",
    "lockF", "lockR", "spin", "apexByLat",
]  # fmt: skip


def v0_runtime() -> Any:
    rt = lupa.LuaRuntime(unpack_returned_tuples=True)
    src, tests = (V0_ROOT / "ApexForge/src").as_posix(), (V0_ROOT / "tests").as_posix()
    rt.execute(f'package.path = "{src}/?.lua;{tests}/?.lua;" .. package.path')
    return rt


def driver_options(rt: Any, driver: dict[str, Any], corners: list[dict[str, Any]]) -> Any:
    options = to_lua(rt, {"car": CAR, "laps": driver["laps"], "trail": driver["trail"]})
    if "pace" in driver:
        options["pace"] = driver["pace"]
    if driver.get("balance") == "entry_us":
        options["balance"] = rt.eval(
            'function(phase, class) return (phase == "entry" and class == "slow") and 1.6 or 0 end'
        )
    if "lock" in driver:
        c = corners[driver["lock"] - 1]
        options["lock"] = rt.eval(f"function(d) return d > {c['zStart']} and d < {c['gStart']} end")
    if "off" in driver:
        c = corners[driver["off"] - 1]
        options["off"] = rt.eval(
            f"function(d) return d > {c['apex'] - 5} and d < {c['apex'] + 5} end"
        )
    return options


def paddock_columns(cols: dict[str, list[float]], t_in: float) -> dict[str, list[float]]:
    out = {
        "t": [t - t_in for t in cols["t"]],
        "lat_g": cols["latG"],
        "lon_g": cols["lonG"],
        "rh_f": cols["rhF"],
        "rh_r": cols["rhR"],
    }
    out.update({name: cols[name] for name in ("d", "v", "gas", "brake", "steer", "yaw", "out")})
    for wheel in ("fl", "fr"):
        out[f"sa_{wheel}"], out[f"sr_{wheel}"] = cols["aF"], cols["srF"]
    for wheel in ("rl", "rr"):
        out[f"sa_{wheel}"], out[f"sr_{wheel}"] = cols["aR"], cols["srR"]
    for wheel in ("fl", "fr", "rl", "rr"):
        out[f"load_{wheel}"] = [3000.0] * len(cols["t"])
    return out


def record(rt: Any, finished: Any, track_length: float) -> dict[str, Any]:
    analyze = rt.eval('require("core.analyze")')
    count = int(finished["n"])
    cols = {
        name: [column[i] for i in range(1, count + 1)] for name, column in finished["cols"].items()
    }
    result = to_py(analyze["pass"](finished, to_lua(rt, {"L": track_length, "driven": "rear"})))
    corner = to_py(finished["corner"])
    return {
        "corner": {
            "number": corner["id"], "zone_start_m": corner["zStart"], "zone_end_m": corner["zEnd"],
            "start_m": corner["gStart"], "end_m": corner["gEnd"], "apex_m": corner["apex"],
        },
        "duration_s": finished["tOut"] - finished["tIn"],
        "columns": paddock_columns(cols, finished["tIn"]),
        "expected": {
            **{key: result.get(key) for key in SCALARS},
            "phases": {name: dict(values) for name, values in (result.get("diag") or {}).items()},
        },
    }  # fmt: skip


def main() -> None:
    track = synthetic_track()
    rt = v0_runtime()
    points = to_lua(rt, [{"x": x, "z": z} for x, z in track.points])
    corner_map = rt.eval('require("core.corners")').build(points, track.length)
    sim = rt.eval('require("sim")')
    line = sim.track(points, track.length)
    speeds = sim.profile(line, to_lua(rt, CAR))
    corners = to_py(corner_map["list"])
    passes: list[dict[str, Any]] = []
    for driver in DRIVERS:
        recorder = rt.eval('require("core.recorder")').new(corner_map)

        def feed(sample: Any, recorder: Any = recorder) -> None:
            finished = recorder.feed(recorder, sample, True)
            if finished is not None:
                passes.append(record(rt, finished, track.length))

        sim.drive(line, speeds, driver_options(rt, driver, corners), feed)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "source": "apexforge-0.3.0 analyze.lua",
        "track_length_m": track.length,
        "passes": passes,
    }
    with gzip.GzipFile(OUTPUT, "wb", mtime=0) as file:
        file.write(json.dumps(payload, separators=(",", ":")).encode())
    print(f"wrote {OUTPUT.relative_to(ROOT)}: {len(passes)} passes, {OUTPUT.stat().st_size} bytes")


if __name__ == "__main__":
    main()
