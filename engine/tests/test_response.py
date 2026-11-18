"""POST /response: per-phase, per-regime vehicle response on synthetic experiments (ADR 0031)."""

from typing import Any

import numpy as np
import pytest
from fastapi.testclient import TestClient
from response_data import ZONE_M, Shape, sampled

from paddock_engine import app as app_module
from paddock_engine.app import create_app
from paddock_engine.contracts.response_request_schema import ResponseRequest
from paddock_engine.io.samples import PassSamples
from paddock_engine.response.config import observables, parse
from paddock_engine.response.metrics import CORNER_METRICS, PHASE_METRICS
from paddock_engine.response.service import response

APEX_SPEEDS = (15.0, 17.0, 28.0, 31.0, 45.0, 49.0)  # m/s: two corners per regime
TRACK = ZONE_M * len(APEX_SPEEDS)
SMALL = parse(
    {
        "metrics": {
            name: {"kind": "direct", "unit": "-", "margin": margin, "phases": phases, "about": ""}
            for name, margin, phases in (
                ("rear_slip_deg", 0.05, ["entry", "mid", "exit"]),
                ("front_slip_deg", 0.05, ["mid"]),
                ("corner_time_s", 0.005, ["corner"]),
                ("phase_time_s", 0.002, ["mid"]),
            )
        },
        "unavailable": {"normalised_slip": "not recorded"},
        "parameters": [
            {
                "key": "rear_arb",
                "match": [{"role": "arb", "axle": "R"}],
                "direct": ["rear_slip_deg"],
                "proxy": [],
                "phases": ["mid"],
                "regimes": ["slow"],
            },
        ],
    }
)


def experiment(
    blocks: int, laps: int, variant: dict[str, float], seed: int = 11
) -> tuple[dict[str, Any], dict[tuple[int, int], PassSamples]]:
    """An A-B-A-B experiment over contiguous zones; `variant` overrides Shape fields."""
    passes, samples, seq, clock = [], {}, 0, 0.0
    for block in range(1, blocks + 1):
        arm = "baseline" if block % 2 else "variant"
        for lap in range(laps):
            for index, v_apex in enumerate(APEX_SPEEDS):
                seq += 1
                rng = np.random.default_rng((seed, seq))  # a pass does not depend on another
                noise = (0.0, TRACK, float(rng.normal(1.0, 0.004)))
                extra = {"rear_slip_extra_deg": float(rng.normal(0, 0.05))}
                if arm == "variant":
                    extra = {k: extra.get(k, 0.0) + v for k, v in variant.items()}
                shape = Shape(v_apex=v_apex, slow_stretch=noise, **extra)
                one = sampled(shape, rng, zone_start_m=index * ZONE_M, track_length=TRACK)
                samples[(1, seq)] = one
                duration = float(one.t[-1]) + 0.018
                passes.append(
                    {"session": 1, "seq": seq, "corner": index + 1, "arm": arm, "block": block,
                     "lap": block * 10 + lap, "t_start_s": clock, "duration_s": duration,
                     "valid": True, "covariates": {}}
                )  # fmt: skip
                clock += duration
    corners = [
        {"number": i + 1, "zone_start_m": i * ZONE_M, "start_m": i * ZONE_M + 180,
         "end_m": i * ZONE_M + 300, "zone_end_m": ((i + 1) * ZONE_M) % TRACK}
        for i in range(len(APEX_SPEEDS))
    ]  # fmt: skip
    request = {"track_length_m": TRACK, "item_role": "arb", "item_axle": "R", "corners": corners,
               "sessions": [{"session": 1, "parquet_dir": "unused"}], "passes": passes}  # fmt: skip
    return request, samples


def test_every_configured_metric_is_computed_and_every_computation_configured() -> None:
    config = observables()

    assert set(config.metrics) == set(PHASE_METRICS) | set(CORNER_METRICS)
    assert {m for m, spec in config.metrics.items() if "corner" in spec.phases} == set(
        CORNER_METRICS
    )
    assert config.row_for("wing", "r") is not None and config.row_for("camber", "F") is None


def test_a_rear_slip_effect_is_found_and_the_time_is_not_claimed() -> None:
    request, samples = experiment(8, 3, {"rear_slip_extra_deg": 0.3})

    out = response(ResponseRequest.model_validate(request), samples, SMALL)

    def cell(metric: str, phase: str, regime: str = "all"):  # type: ignore[no-untyped-def]
        return next(
            e for e in out.effects if (e.metric, e.phase, e.regime.value) == (metric, phase, regime)
        )

    slip = cell("rear_slip_deg", "mid")
    assert slip.finding.value == "increase" and slip.ci_low is not None
    assert slip.ci_low < 0.3 < (slip.ci_high or 0)
    assert slip.expected.value == "direct"
    assert cell("front_slip_deg", "mid").finding.value in ("neutral", "insufficient")
    assert cell("corner_time_s", "corner").finding.value in ("neutral", "insufficient")
    assert cell("rear_slip_deg", "mid", "slow").note == "2 corners: too few for a regime"
    assert out.parameter == "rear_arb" and out.passes_used == 8 * 3 * len(APEX_SPEEDS)
    assert out.regimes.method.value == "gaps"
    assert [c.regime.value for c in out.corners] == ["slow"] * 2 + ["medium"] * 2 + ["fast"] * 2
    assert [u.observable for u in out.unavailable] == ["normalised_slip"]


def test_both_arms_are_cut_at_the_baseline_s_metres_whatever_the_variant_does() -> None:
    first, samples_a = experiment(4, 2, {"rear_slip_extra_deg": 0.3})
    second, samples_b = experiment(4, 2, {"apex_m": 250.0, "turn_in_m": 205.0})

    one = response(ResponseRequest.model_validate(first), samples_a, SMALL)
    two = response(ResponseRequest.model_validate(second), samples_b, SMALL)

    assert [c.phases for c in one.corners] == [c.phases for c in two.corners]
    assert [c.regime for c in one.corners] == [c.regime for c in two.corners]
    assert all(c.phases[-1].status.value == "ok" for c in one.corners[:-1])  # joined: traction


def test_the_endpoint_answers_with_the_default_mapping(monkeypatch: pytest.MonkeyPatch) -> None:
    request, samples = experiment(2, 2, {"rear_slip_extra_deg": 0.3})
    request["corners"], request["passes"] = (
        request["corners"][:2],
        [p for p in request["passes"] if p["corner"] <= 2],
    )
    monkeypatch.setattr(app_module, "load_samples", lambda _request: samples)

    reply = TestClient(create_app()).post("/response", json=request)

    assert reply.status_code == 200, reply.text
    body = reply.json()
    assert body["parameter"] == "rear_arb" and body["regimes"]["method"] == "fixed"
    assert {u["observable"] for u in body["unavailable"]} >= {"normalised_slip", "body_slip_deg"}
    assert {e["phase"] for e in body["effects"]} >= {"braking", "mid", "traction", "corner"}
