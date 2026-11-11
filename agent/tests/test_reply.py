"""Reply/value validation against real setup items: step grid, shown-vs-raw values, options.

Ported from Apex Forge v0 `tests/test_setup.py` (test_reply, test_versions). The same rules are
ported to Go in `core/internal/setups` in a later phase; these tests are the reference.
"""

from typing import Any

import pytest

from harness import load_fixture, runtime, to_lua, to_py
from test_catalog import FA26, MX5, build_catalog, fa26_tabs


def parse(rt: Any, catalog: Any, text: str) -> dict[str, Any]:
    current = {item["name"]: item["value"] for item in to_py(catalog["items"])}
    reply = rt.eval('require("core.reply")').parse(text, catalog, to_lua(rt, current))
    parsed = to_py(reply)
    # an empty Lua table converts to {}: normalise both lists
    return {key: parsed.get(key) or [] for key in ("changes", "problems")}


def changed_values(parsed: dict[str, Any]) -> dict[str, Any]:
    return {change["name"]: change["to"] for change in parsed["changes"]}


def test_whole_setup_file_paste_is_read() -> None:
    rt = runtime()
    ini = (
        "[ABOUT]\nAUTHOR=x\n[CAMBER_LF]\nVALUE=-25\n[ARB_FRONT]\nVALUE=2\n"
        "[TYRES]\nVALUE=1\n[CAR]\nMODEL=ks_mazda_mx5_cup\n"
    )
    values = changed_values(parse(rt, build_catalog(rt, MX5), ini))
    assert values["CAMBER_LF"] == -25
    assert values["ARB_FRONT"] == 2
    assert values["COMPOUND"] == 1  # [TYRES] in a setup file is the COMPOUND item in game


def test_shown_value_is_converted_to_raw() -> None:
    rt = runtime()
    parsed = parse(rt, build_catalog(rt, MX5), "```apexforge\nCAMBER_LF=-3.2\nCAMBER_RF=-3\n```")
    values = changed_values(parsed)
    assert values["CAMBER_LF"] == -32
    # -3 is the current -3.0 deg, so it is reported as "no change", not applied as raw -3
    assert "CAMBER_RF" not in values
    assert any("Camber RF" in problem for problem in parsed["problems"])


def test_option_names_and_step_grid_on_the_fa26() -> None:
    rt = runtime()
    catalog = build_catalog(rt, FA26, fa26_tabs())
    text = "```apexforge\nCOMPOUND=Piorani SS (C4)\nFRONT_BIAS=54.6\n```"
    values = changed_values(parse(rt, catalog, text))
    assert values["COMPOUND"] == 1
    assert values["FRONT_BIAS"] == 55


@pytest.mark.parametrize(
    ("text", "expected_word"),
    [("Sure! Here you go: nothing to change.", ""), ("", "empty")],
)
def test_reply_without_values_reports_a_problem(text: str, expected_word: str) -> None:
    rt = runtime()
    parsed = parse(rt, build_catalog(rt, FA26, fa26_tabs()), text)
    assert parsed["changes"] == []
    assert any(expected_word in problem for problem in parsed["problems"])


@pytest.mark.parametrize(
    ("current", "expected"),
    [
        ("base", "base_v2"),
        ("COTA_Quali_Jason_C4_V2", "COTA_Quali_Jason_C4_V3"),
        ("monza_v9", "monza_v10"),
        ("Istanbul_Quali_Jason_C4", "Istanbul_Quali_Jason_C4_v2"),
        ("C4", "C4_v2"),
    ],
)
def test_next_version_name(current: str, expected: str) -> None:
    assert runtime().eval('require("core.versions")').nextName(current) == expected


def test_free_name_never_reuses_an_existing_file() -> None:
    rt = runtime()
    taken = to_lua(rt, {"base_v2": True, "base_v3": True})
    exists = rt.eval("function(t) return function(name) return t[name] ~= nil end end")(taken)
    assert rt.eval('require("core.versions")').freeName("base", exists) == "base_v4"


def test_fixture_spinner_exports_are_present() -> None:
    assert len(load_fixture(f"spinners_{FA26}.json")["spinners"]) > 50
