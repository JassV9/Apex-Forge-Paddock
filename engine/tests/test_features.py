"""New channels, per-pass features, and the speed-binned slip finding on the real session."""

import json
from pathlib import Path
from typing import Any

import numpy as np
import pytest
from fastapi.testclient import TestClient

from paddock_engine.app import create_app
from paddock_engine.features import channels
from paddock_engine.features.pass_features import compute, from_legacy, speed_class
from paddock_engine.features.speed_bins import speed_binned_slip
from paddock_engine.io.samples import COLUMN_NAMES, PassSamples, read_session_samples
from paddock_engine.phases.segment import Corner, segment

FIXTURES = Path(__file__).parent / "fixtures"
REPO = Path(__file__).resolve().parents[2]
DEMO_SPOOL = REPO / "datasets/demo/spool/demo-fa26-cota/000001.jsonl"
SAMPLE_SPOOL = REPO / "core/internal/ingest/testdata/spool/sample-session"
CORNER = Corner(number=1, zone_start_m=0, start_m=150, end_m=250, apex_m=200, zone_end_m=400)
TRACK_LENGTH = 3000.0


def synthetic_pass(steer_keeps_rising: bool) -> PassSamples:
    """A 6 s corner at 60 Hz: brake, turn in, hold, unwind. Lateral g peaks at 1.5 g at 2.5 s.

    With `steer_keeps_rising` the driver winds on 40 % more lock after that, and lateral g
    does not follow: the front tyres are past their peak.
    """
    t = np.arange(0, 6, 1 / 60)
    turn = np.clip((t - 1.5) / 1.0, 0, 1) * np.clip((5.5 - t) / 1.0, 0, 1)
    extra = 0.4 * np.clip((t - 2.5) / 1.5, 0, 1) * np.clip((5.5 - t) / 1.0, 0, 1)
    steer = 90 * (turn + (extra if steer_keeps_rising else 0))
    v = 60 - 30 * np.clip(t / 2.5, 0, 1) + 25 * np.clip((t - 3.5) / 2.5, 0, 1)
    columns: dict[str, Any] = {name: np.zeros_like(t) for name in COLUMN_NAMES}
    columns.update(
        t=t, d=np.cumsum(v) / 60, v=v, steer=steer, lat_g=1.5 * turn, yaw=0.5 * turn,
        brake=np.where(t < 2.0, 0.8, 0.0), gas=np.where(t > 3.5, 1.0, 0.0),
    )  # fmt: skip
    for wheel in ("fl", "fr", "rl", "rr"):
        columns[f"load_{wheel}"] = np.full_like(t, 3000.0)
        columns[f"sa_{wheel}"] = (6.0 if wheel[0] == "f" else 3.0) * turn
    return PassSamples.from_columns(columns)


def test_saturation_index_separates_winding_on_lock_from_a_clean_corner() -> None:
    clean, saturated = synthetic_pass(False), synthetic_pass(True)

    clean_index = channels.front_saturation_index(clean, segment(clean, CORNER, TRACK_LENGTH))
    saturated_index = channels.front_saturation_index(
        saturated, segment(saturated, CORNER, TRACK_LENGTH)
    )

    assert clean_index is not None and saturated_index is not None
    assert clean_index < 0.05
    assert saturated_index > 0.4


def test_more_steering_for_the_same_grip_raises_steer_per_g() -> None:
    clean, saturated = synthetic_pass(False), synthetic_pass(True)

    clean_gradient = channels.steer_per_g(clean, segment(clean, CORNER, TRACK_LENGTH))
    saturated_gradient = channels.steer_per_g(saturated, segment(saturated, CORNER, TRACK_LENGTH))

    assert clean_gradient == pytest.approx(60.0, rel=0.02)  # 90 degrees for 1.5 g
    # the mid window sits at the slowest point, where about a third of the extra lock is on
    assert saturated_gradient is not None and saturated_gradient > 1.1 * 60.0


def test_pass_features_carry_every_channel_and_the_validity() -> None:
    samples = synthetic_pass(False)

    features = compute(samples, CORNER, TRACK_LENGTH, duration_s=6.0)

    assert features.valid and features.invalid_reason is None
    assert features.speed_class == "medium"  # minimum speed 108 km/h
    assert features.metrics["corner_time_s"] == 6.0
    assert features.metrics["v_min_kmh"] == pytest.approx(108.0)
    assert features.metrics["front_slip_mid_deg"] == pytest.approx(6.0, rel=0.02)
    assert features.metrics["rear_slip_mid_deg"] == pytest.approx(3.0, rel=0.02)
    measured = [
        "steer_per_g_mid",
        "front_saturation_index",
        "yaw_gain_entry",
        "brake_dist_m",
        "throttle_pickup_m",
        "trail_share",
        "rear_slip_peak_deg",
        "wheelspin_s",
    ]
    for metric in measured:
        assert features.metrics[metric] is not None, metric


def test_a_pass_with_three_wheels_off_is_not_valid() -> None:
    samples = synthetic_pass(False)
    samples.out[100:110] = 3

    features = compute(samples, CORNER, TRACK_LENGTH, duration_s=6.0)

    assert (features.valid, features.invalid_reason) == (False, "off track")


@pytest.mark.parametrize(
    ("v_min_kmh", "expected"), [(60, "slow"), (99.9, "slow"), (100, "medium"), (150, "fast")]
)
def test_speed_class_boundaries(v_min_kmh: float, expected: str) -> None:
    assert speed_class(v_min_kmh) == expected


def demo_passes() -> list[dict[str, Any]]:
    lines = [json.loads(line) for line in DEMO_SPOOL.read_text(encoding="utf-8").splitlines()]
    return lines[1:]


def test_real_session_front_slip_depends_on_corner_speed_and_rear_slip_does_not() -> None:
    """The audit's finding: about 7.7 degrees of front slip below 100 km/h, about 4 above 150,
    with rear slip near 3 degrees in both. A balance problem would move the rear too."""
    features = [from_legacy(p["legacy"], p["duration_s"]) for p in demo_passes()]

    bins = speed_binned_slip((f.speed_class, f.metrics) for f in features if f.valid)

    assert bins["slow"].front_slip_deg == pytest.approx(7.7, abs=0.2)
    assert bins["fast"].front_slip_deg == pytest.approx(4.0, abs=0.5)
    assert bins["slow"].rear_slip_deg == pytest.approx(3.1, abs=0.3)
    assert bins["fast"].rear_slip_deg == pytest.approx(3.1, abs=0.3)
    assert bins["slow"].front_minus_rear_deg > 3 * bins["fast"].front_minus_rear_deg
    assert bins["slow"].passes >= 30


def test_parquet_parts_written_by_core_are_read_back_per_pass() -> None:
    samples = read_session_samples(FIXTURES / "session_parts")

    spool = [
        json.loads(line)
        for chunk in sorted(SAMPLE_SPOOL.glob("*.jsonl"))
        for line in chunk.read_text(encoding="utf-8").splitlines()
    ]
    recorded = {line["seq"]: line["samples"] for line in spool if line["kind"] == "pass"}
    assert sorted(samples) == sorted(recorded) == [1, 2, 3]
    for seq, pass_samples in samples.items():
        assert pass_samples.n == recorded[seq]["n"]
        assert pass_samples.v.tolist() == recorded[seq]["columns"]["v"]
        assert pass_samples.load[:, 2].tolist() == recorded[seq]["columns"]["load_rl"]
    assert read_session_samples(FIXTURES / "does-not-exist") == {}


def features_request() -> dict[str, Any]:
    spool = [
        json.loads(line)
        for chunk in sorted(SAMPLE_SPOOL.glob("*.jsonl"))
        for line in chunk.read_text(encoding="utf-8").splitlines()
    ]
    session, passes = spool[0], spool[1:]
    keys = ("number", "zone_start_m", "start_m", "end_m", "apex_m", "zone_end_m")
    return {
        "track_length_m": session["track"]["length_m"],
        "driven_axle": "rear",
        "parquet_dir": str(FIXTURES / "session_parts"),
        "corners": [{key: corner[key] for key in keys} for corner in session["track"]["corners"]],
        "passes": [
            {"seq": p["seq"], "corner": p["corner"], "duration_s": p["duration_s"]} for p in passes
        ],
    }


def test_features_endpoint_from_agent_samples_through_core_parquet() -> None:
    """Lua agent -> spool -> Go ingest -> Parquet -> DuckDB -> features: the whole chain."""
    client = TestClient(create_app())

    response = client.post("/features/session", json=features_request())

    assert response.status_code == 200
    passes = response.json()["passes"]
    assert [p["seq"] for p in passes] == [1, 2, 3]
    for item in passes:
        assert item["valid"] is True and "invalid_reason" not in item
        assert item["metrics"]["corner_time_s"] > 5
        assert item["metrics"]["steer_per_g_mid"] > 0
        assert item["metrics"]["front_slip_mid_deg"] == pytest.approx(3.0, abs=0.5)
    assert [p["speed_class"] for p in passes] == ["medium", "fast", "slow"]


def test_features_endpoint_handles_imported_and_missing_passes() -> None:
    client = TestClient(create_app())
    request = features_request()
    legacy = demo_passes()[0]["legacy"]
    request["passes"] = [
        {"seq": 50, "corner": 1, "duration_s": 9.7, "legacy": legacy},
        {"seq": 51, "corner": 1, "duration_s": 9.7},
        {"seq": 1, "corner": 99, "duration_s": 9.7},
    ]

    passes = client.post("/features/session", json=request).json()["passes"]

    assert passes[0]["valid"] is True
    assert passes[0]["metrics"]["front_slip_mid_deg"] == pytest.approx(7.32, abs=0.01)
    assert "no raw samples" in passes[0]["flags"][0]
    assert (passes[1]["valid"], passes[1]["invalid_reason"]) == (False, "no samples")
    assert (passes[2]["valid"], passes[2]["invalid_reason"]) == (False, "unknown corner")


def test_features_endpoint_rejects_a_request_outside_the_contract() -> None:
    request = features_request()
    request["driven_axle"] = "sideways"

    assert TestClient(create_app()).post("/features/session", json=request).status_code == 422
