"""Runs the agent's real Lua files under LuaJIT (lupa), the same interpreter family CSP uses.

Adapted from Apex Forge v0 `tests/harness.py` (see docs/adr/0002-reuse-from-v0.md). The helpers
that read AI lines from a local Assetto Corsa install are not carried over yet: they cannot run
in CI and nothing in phase 0 needs them.
"""

import json
from pathlib import Path
from typing import Any

from lupa import luajit21 as lupa

TESTS_DIR = Path(__file__).resolve().parent
AGENT_DIR = TESTS_DIR.parent
SRC_DIR = AGENT_DIR / "src"
FIXTURES_DIR = TESTS_DIR / "fixtures"


def runtime() -> Any:
    """A fresh LuaJIT state whose `require` resolves modules under agent/src."""
    rt = lupa.LuaRuntime(unpack_returned_tuples=True)
    rt.execute(f'package.path = "{SRC_DIR.as_posix()}/?.lua;" .. package.path')
    return rt


def _is_lua_array(keys: list[Any]) -> bool:
    return (
        bool(keys)
        and all(isinstance(key, int) for key in keys)
        and sorted(keys) == list(range(1, len(keys) + 1))
    )


def to_py(value: Any) -> Any:
    """Lua table to dict or list (1..n integer keys become a list); functions are dropped."""
    if lupa.lua_type(value) != "table":
        return value
    keys = list(value.keys())
    if _is_lua_array(keys):
        return [to_py(value[key]) for key in sorted(keys)]
    return {str(k): to_py(v) for k, v in value.items() if lupa.lua_type(v) != "function"}


def to_lua(rt: Any, value: Any) -> Any:
    """dict or list to a Lua table (lists become 1-based arrays)."""
    if isinstance(value, dict):
        table = rt.table()
        for key, item in value.items():
            table[key] = to_lua(rt, item)
        return table
    if isinstance(value, list | tuple):
        table = rt.table()
        for index, item in enumerate(value):
            table[index + 1] = to_lua(rt, item)
        return table
    return value


def load_fixture(name: str) -> Any:
    return json.loads((FIXTURES_DIR / name).read_text(encoding="utf-8"))
