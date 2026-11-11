"""Setup catalog on real spinner exports: roles come from the in-game labels, not item names.

Ported from Apex Forge v0 `tests/test_setup.py` (test_roles, test_display).
"""

from typing import Any

import pytest

from harness import load_fixture, runtime, to_lua, to_py

FA26 = "vrc_formula_alpha_2026_csp"
RSS = "rss_formula_hybrid_2021"
MX5 = "ks_mazda_mx5_cup"


def build_catalog(rt: Any, car: str, tabs: dict[str, str] | None = None) -> Any:
    spinners = load_fixture(f"spinners_{car}.json")["spinners"]
    tab_of = None
    if tabs:
        tab_of = rt.eval("function(t) return function(name) return t[name] end end")(
            to_lua(rt, tabs)
        )
    return rt.eval('require("core.catalog")').build(to_lua(rt, spinners), tab_of, None)


def fa26_tabs() -> dict[str, str]:
    return {
        item["section"]: item["tab"] for item in load_fixture("fa26_setup_report.json")["items"]
    }


def roles(catalog: Any) -> dict[str, tuple[Any, Any, Any]]:
    return {
        item["name"]: (item.get("role"), item.get("axle"), item.get("included"))
        for item in to_py(catalog["items"])
    }


@pytest.mark.parametrize(
    ("item", "role", "axle"),
    [
        ("WING_1", "wing", "F"),  # labelled "FW Flap": the front wing despite the name
        ("CUSTOM_SCRIPT_ITEM_1959866273", "wing", "R"),  # "Gurney Flap Rear"
        ("ARB_F", "arb", "F"),
        ("ARB_R", "arb", "R"),
        ("TORSION_RATE_LF_0", "spring", "F"),
    ],
)
def test_fa26_roles_and_axles(item: str, role: str, axle: str) -> None:
    found = roles(build_catalog(runtime(), FA26, fa26_tabs()))
    assert found[item][:2] == (role, axle)


def test_fa26_roles_without_axle() -> None:
    found = roles(build_catalog(runtime(), FA26, fa26_tabs()))
    assert found["FRONT_BIAS"][0] == "bias"
    assert found["SPRING_RATE_LF_C1"][0] == "heave"


def test_fa26_excludes_power_unit_and_display_items() -> None:
    catalog = build_catalog(runtime(), FA26, fa26_tabs())
    assert roles(catalog)["MGUK_DELIVERY"][2] is False
    excluded_words = ("Display", "Deploy", "Split", "Version", "Debounce")
    included_labels = [item["label"] for item in to_py(catalog["items"]) if item["included"]]
    assert [label for label in included_labels if any(w in label for w in excluded_words)] == []


def test_rss_roles_follow_labels() -> None:
    found = roles(build_catalog(runtime(), RSS))
    assert found["WING_0"][:2] == ("wing", "F")
    assert found["WING_1"][:2] == ("wing", "R")
    assert found["DIFF_POWER"][0] == "diffpower"
    assert found["DIFF_COAST"][0] == "diffcoast"
    assert found["WING_11"][2] is False  # the "HALO" pseudo-wing
    assert found["SPRING_RATE_HF"][0] == "heave"
    assert found["ROD_LENGTH_LF"][0] == "height"


def test_mx5_roles() -> None:
    found = roles(build_catalog(runtime(), MX5))
    assert found["ARB_FRONT"][:2] == ("arb", "F")
    assert found["ARB_REAR"][:2] == ("arb", "R")
    assert found["CAMBER_LF"][:2] == ("camber", "F")
    assert found["TOE_OUT_LR"][0] == "toe"


@pytest.mark.parametrize(
    ("item", "raw", "shown"),
    [
        ("CAMBER_LF", -30, "-3.0 deg"),
        ("PRESSURE_LF", 17, "17 psi"),
        ("DAMP_BUMP_LF", 5, "click 5 of 11"),
    ],
)
def test_display_matches_the_setup_menu(item: str, raw: int, shown: str) -> None:
    rt = runtime()
    catalog = build_catalog(rt, MX5)
    display = rt.eval('require("core.catalog")').display
    assert display(catalog["byName"][item], raw) == shown


def test_compound_displays_its_option_name() -> None:
    rt = runtime()
    catalog = build_catalog(rt, MX5)
    shown = rt.eval('require("core.catalog")').display(catalog["byName"]["COMPOUND"], 0)
    assert shown not in ("0", "?")
