"""The phase cuts and driver-input metrics are a port of Apex Forge v0's analyze.lua.

tests/fixtures/analyze_reference.json.gz holds 14 passes with what v0 itself computed for them
(tools/export_analyze_reference.py). The port must reproduce every number.
"""

import gzip
import json
from pathlib import Path
from typing import Any

import pytest

from paddock_engine.features.axles import axle_channels
from paddock_engine.features.inputs import driver_inputs
from paddock_engine.features.phase_stats import phase_stats
from paddock_engine.io.samples import PassSamples
from paddock_engine.phases.segment import Corner, Phase, segment

REFERENCE = Path(__file__).parent / "fixtures" / "analyze_reference.json.gz"
TOLERANCE = 1e-9

# ours -> v0's name
INPUT_FIELDS = {
    "brake_dist_m": "brakeDist", "brake_peak": "brakePeak", "brake_attack_s": "attack",
    "brake_release_m": "release", "trail_share": "trail", "throttle_pickup_m": "pickup",
    "throttle_to_full_s": "toFull", "lifts": "lifts", "coast_s": "coast",
    "steer_corrections_per_s": "corr", "countersteer_s": "cs", "countersteer_exit_s": "csExit",
    "front_lockups": "lockF", "rear_lockups": "lockR", "wheelspin": "spin",
}  # fmt: skip
# v0's worst-tyre fields (aFx, aRx) are left out: its test driver never filled them.
PHASE_FIELDS = {
    "duration_s": "dur", "coverage": "cov", "front_slip_deg": "aF", "rear_slip_deg": "aR",
    "lat_g": "lat", "yaw_rate": "yaw", "steer_rate": "srate", "gas": "gas", "brake": "brk",
}  # fmt: skip


def load_reference() -> tuple[float, list[dict[str, Any]]]:
    with gzip.open(REFERENCE) as file:
        data = json.load(file)
    return data["track_length_m"], data["passes"]


TRACK_LENGTH, PASSES = load_reference()


def pass_id(reference: dict[str, Any]) -> str:
    return f"T{reference['corner']['number']}-{len(reference['columns']['t'])}"


def close(ours: float | None, theirs: float | None) -> bool:
    if ours is None or theirs is None:
        return ours is None and theirs is None
    return abs(float(ours) - float(theirs)) <= TOLERANCE


@pytest.mark.parametrize("reference", PASSES, ids=pass_id)
def test_port_reproduces_what_v0_computed(reference: dict[str, Any]) -> None:
    samples = PassSamples.from_columns(reference["columns"])
    corner = Corner(**reference["corner"])
    expected = reference["expected"]

    seg = segment(samples, corner, TRACK_LENGTH)
    axles = axle_channels(samples)
    inputs = driver_inputs(samples, seg, axles)
    phases = phase_stats(samples, seg, axles, corner, TRACK_LENGTH)

    assert close(seg.v_min * 3.6, expected["vMin"])
    assert close(seg.v_entry * 3.6, expected["vEntry"])
    assert close(seg.lat_peak, expected["latPeak"])
    assert seg.apex_by_lat == bool(expected["apexByLat"])
    for ours, theirs in INPUT_FIELDS.items():
        assert close(getattr(inputs, ours), expected[theirs]), ours
    assert set(phases) == set(expected["phases"])
    for name, stats in phases.items():
        for ours, theirs in PHASE_FIELDS.items():
            assert close(getattr(stats, ours), expected["phases"][name][theirs]), (name, ours)


def test_reference_covers_both_drivers_and_the_special_cases() -> None:
    assert len(PASSES) == 14
    assert any(reference["expected"]["lockF"] for reference in PASSES)
    assert any(reference["expected"]["off"] for reference in PASSES)


def test_every_sample_between_turn_in_and_unwind_has_a_phase() -> None:
    samples = PassSamples.from_columns(PASSES[0]["columns"])

    seg = segment(samples, Corner(**PASSES[0]["corner"]), TRACK_LENGTH)

    assert (seg.labels[seg.turn_in : seg.unwind + 1] != Phase.NONE).all()
    assert seg.mid_start <= seg.apex <= seg.mid_end
    assert seg.labels[seg.apex] in (Phase.MID, Phase.ENTRY)
    assert (seg.labels[seg.unwind + 1 :] == Phase.NONE).all()


def test_columns_must_be_complete_and_of_equal_length() -> None:
    columns = dict(PASSES[0]["columns"])
    with pytest.raises(ValueError, match="sa_fl"):
        PassSamples.from_columns({k: v for k, v in columns.items() if k != "sa_fl"})
    with pytest.raises(ValueError, match="length"):
        PassSamples.from_columns({**columns, "yaw": columns["yaw"][:-1]})
