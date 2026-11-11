"""Writes a small spool sample with the real agent, for other modules' tests.

    cd agent && uv run python tests/export_spool_sample.py ../core/internal/ingest/testdata/spool

The output is exactly what PaddockAgent.lua writes on the mocked CSP API while the synthetic
car drives until three passes are recorded: one session line and three pass lines in two or
more chunk files under <target>/sample-session/. Core's ingest tests read it, which keeps the
Lua writer and the Go reader honest about the same contract.
"""

import shutil
import sys
import tempfile
from pathlib import Path

import conftest
from harness import runtime, to_lua
from track import synthetic_track

SESSION_ID = "sample-session"


def export(target: Path) -> None:
    rt = runtime()
    rt.execute(f'package.path = "{conftest.TESTS_DIR.as_posix()}/?.lua;" .. package.path')
    with tempfile.TemporaryDirectory() as root:
        rt.globals().MOCK = conftest._fill_mock(rt, Path(root), synthetic_track())
        rt.execute(f'dofile("{conftest.TESTS_DIR.as_posix()}/mock_csp.lua")')
        drive = rt.eval('require("drive")')
        drive.boot()
        drive.profile(to_lua(rt, conftest.CAR))
        drive.run(rt.table_from({"laps": 1, "untilPasses": 3}))
        drive.release()
        (written,) = (Path(root) / "paddock/spool").iterdir()
        out = target / SESSION_ID
        shutil.rmtree(out, ignore_errors=True)
        out.mkdir(parents=True)
        for chunk in sorted(written.glob("*.jsonl")):
            # the real id holds the wall-clock start time; a fixed one keeps the sample stable
            text = chunk.read_text(encoding="utf-8").replace(written.name, SESSION_ID)
            (out / chunk.name).write_text(text, encoding="utf-8", newline="\n")
            print("wrote", out / chunk.name, len(text), "bytes")


if __name__ == "__main__":
    export(Path(sys.argv[1]))
