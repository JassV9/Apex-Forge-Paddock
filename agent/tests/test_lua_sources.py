"""Static checks on every Lua file: it compiles under LuaJIT, and pure modules stay pure."""

import re
from pathlib import Path

import pytest

from harness import AGENT_DIR, SRC_DIR, runtime

LUA_FILES = sorted(AGENT_DIR.rglob("*.lua"))
PURE_FILES = sorted((SRC_DIR / "core").glob("*.lua"))

# Namespaces CSP injects. A pure module must be runnable with none of them defined.
CSP_CALL = re.compile(r"\b(ac|ui|io|os|web|render|physics)\.\w")
LINE_COMMENT = re.compile(r"--.*$", re.MULTILINE)


def relative_id(path: Path) -> str:
    return path.relative_to(AGENT_DIR).as_posix()


def test_sources_were_found() -> None:
    assert len(LUA_FILES) >= 8
    assert len(PURE_FILES) >= 5


@pytest.mark.parametrize("path", LUA_FILES, ids=relative_id)
def test_compiles_under_luajit(path: Path) -> None:
    compile_chunk = runtime().eval(
        "function(source, name) local f, err = loadstring(source, name) return f ~= nil, err end"
    )
    ok, error = compile_chunk(path.read_text(encoding="utf-8"), relative_id(path))
    assert ok, error


@pytest.mark.parametrize("path", PURE_FILES, ids=relative_id)
def test_pure_module_makes_no_csp_calls(path: Path) -> None:
    code = LINE_COMMENT.sub("", path.read_text(encoding="utf-8"))
    found = CSP_CALL.search(code)
    assert found is None, f"{relative_id(path)} uses a CSP namespace: {found.group(0)!r}"
