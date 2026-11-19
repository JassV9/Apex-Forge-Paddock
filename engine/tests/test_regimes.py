"""Speed regimes per corner, frozen from the baseline, cut per car and track (ADR 0031)."""

import json
from pathlib import Path
from typing import Any

import pytest
from hypothesis import given, settings
from hypothesis import strategies as st

from paddock_engine.contracts.evaluate_request_schema import EvaluatePass
from paddock_engine.experiments.channel_effects import experiment_regimes
from paddock_engine.features.regimes import (
    FIXED_BREAKS_KMH,
    MIN_PER_CLASS,
    aero_fit,
    corner_regimes,
    gap_breaks,
)

MEDIANS = json.loads(
    (Path(__file__).parent / "fixtures/corner_medians_2026-11-18.json").read_text("utf-8")
)


def track(name: str, car: str) -> list[dict[str, Any]]:
    entry = next(t for t in MEDIANS["tracks"] if t["track"] == name and t["car"] == car)
    corners: list[dict[str, Any]] = entry["corners"]
    return corners


def regimes_of(corners: list[dict[str, Any]]) -> dict[str, list[int]]:
    regimes = corner_regimes((c["corner"], c["v_min_kmh"], None) for c in corners)
    out: dict[str, list[int]] = {"slow": [], "medium": [], "fast": []}
    for corner, name in sorted(regimes.classes.items()):
        out[name].append(corner)
    return out


def test_the_formula_hybrid_at_the_hungaroring_breaks_at_130_and_161_kmh() -> None:
    corners = track("fn_hungaroring", "rss_formula_hybrid_2026")

    breaks = gap_breaks(c["v_min_kmh"] for c in corners)

    assert breaks == pytest.approx((130.2, 161.6), abs=0.5)
    assert regimes_of(corners) == {
        "slow": [1, 2, 6, 7, 12, 13, 14],
        "medium": [5, 8, 9],
        "fast": [3, 4, 10, 11],
    }


def test_the_488_gt3_at_cota_gets_its_own_breaks() -> None:
    corners = track("fn_cota", "ks_ferrari_488_gt3")

    breaks = gap_breaks(c["v_min_kmh"] for c in corners)

    assert breaks == pytest.approx((83.0, 150.6), abs=0.5)
    assert regimes_of(corners)["fast"] == [3, 4]


def test_a_single_outlying_corner_is_not_a_class_of_its_own() -> None:
    corners = track("fn_cota", "rss_formula_hybrid_2026")  # corner 1 at 86 km/h, the rest > 140

    classes = regimes_of(corners)

    assert all(len(members) >= MIN_PER_CLASS for members in classes.values())
    assert 1 in classes["slow"]


def test_too_few_corners_fall_back_to_the_fixed_bands_on_corner_medians() -> None:
    regimes = corner_regimes([(1, 90.0, None), (1, 95.0, None), (2, 160.0, None)])

    assert regimes.method == "fixed" and regimes.breaks_kmh == FIXED_BREAKS_KMH
    assert regimes.classes == {1: "slow", 2: "fast"}


def test_corners_without_speeds_keep_their_most_common_stored_class() -> None:
    regimes = corner_regimes([(1, None, "slow"), (1, None, "slow"), (1, None, "medium")])

    assert regimes.method == "stored" and regimes.classes == {1: "slow"}


def evaluate_passes(variant_shift_kmh: float, variant_class: str | None) -> list[EvaluatePass]:
    corners = track("fn_hungaroring", "rss_formula_hybrid_2026")
    out = []
    for corner in corners:
        for arm, shift in (("baseline", 0.0), ("variant", variant_shift_kmh)):
            for block in (1, 2):
                metrics = {"v_min_kmh": corner["v_min_kmh"] + shift}
                item = {
                    "corner": corner["corner"],
                    "arm": arm,
                    "block": block,
                    "metrics": metrics,
                    "covariates": {},
                    "speed_class": variant_class if arm == "variant" else None,
                }
                out.append(EvaluatePass.model_validate(item))
    return out


@settings(max_examples=30, deadline=None)
@given(
    st.floats(min_value=-60, max_value=60),
    st.sampled_from([None, "slow", "medium", "fast"]),
)
def test_a_corner_keeps_its_class_whatever_the_variant_does(
    shift: float, stored: str | None
) -> None:
    reference = experiment_regimes(evaluate_passes(0.0, None)).classes

    assert experiment_regimes(evaluate_passes(shift, stored)).classes == reference


def test_the_aero_fit_is_adequate_at_the_hungaroring_and_not_at_cota_with_the_gt3() -> None:
    def fit_of(corners: list[dict[str, Any]]):  # type: ignore[no-untyped-def]
        return aero_fit({c["corner"]: (c["v_min_kmh"], c["lat_g_peak"]) for c in corners})

    fast = fit_of(track("fn_hungaroring", "rss_formula_hybrid_2026"))
    gt3 = fit_of(track("fn_cota", "ks_ferrari_488_gt3"))

    assert fast is not None and gt3 is not None
    assert fast.adequate and fast.r2 == pytest.approx(0.73, abs=0.01)
    assert 0.25 < (fast.share(202.0) or 0) < 0.4  # T4, an ESTIMATE
    assert not gt3.adequate and gt3.share(170.0) is None
