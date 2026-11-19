"""Re-deriving experiments for the profile (POST /sensitivity/measure), and the replay of the
FH26's four rear ARB +1 runs, experiments 7, 11, 16 and 19 (ADR 0033)."""

from functools import cache
from typing import Any

import pytest
import real_experiments as real
from fastapi.testclient import TestClient
from sensitivity_data import STRATUM, request_of
from test_experiments import TEST, TIME, experiment_passes

from paddock_engine.app import create_app
from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.sensitivity_measure_schema import FindingClass, SensitivityMeasure
from paddock_engine.contracts.sensitivity_response_schema import KnowledgeLevel, Method
from paddock_engine.sensitivity.measure import measure
from paddock_engine.sensitivity.pooling import Estimate
from paddock_engine.sensitivity.profile import update
from paddock_engine.stats.sequential import look_of

REAR_ARB = (7, 11, 16, 19)
GUARDRAIL = {"channel": "rear_slip_peak_deg", "tolerance": 0.5, "worse": "higher"}


def simulated(**overrides: Any) -> dict[str, Any]:
    passes = experiment_passes({TIME: (-0.02, 0.008), "rear_slip_peak_deg": (0.1, 0.2)}, seed=5)
    base = {"success_test": TEST, "guardrails": [GUARDRAIL], "predictions": []}
    return base | {"driver_channels": [], "passes": passes} | overrides


def test_the_primary_and_each_guardrail_come_back_with_error_and_degrees_of_freedom() -> None:
    got = measure(EvaluateRequest.model_validate(simulated()))

    channels = {(e.channel, e.corner_class.value) for e in got.effects}
    assert {(TIME, "all"), ("rear_slip_peak_deg", "all")} <= channels
    primary = next(e for e in got.effects if (e.channel, e.corner_class.value) == (TIME, "all"))
    assert primary.df == primary.blocks - 3  # 8 blocks less the arm, mean and time covariate
    assert primary.standard_error > 0 and abs(primary.estimate + 0.02) < 4 * primary.standard_error
    assert all(e.df >= 3 for e in got.effects if e.corner_class.value != "all")
    assert got.engine_version and got.finding_class is not FindingClass.invalid


def test_an_invalid_experiment_measures_nothing() -> None:
    passes = [p for p in simulated()["passes"] if p["arm"] == "baseline"]

    got = measure(EvaluateRequest.model_validate(simulated(passes=passes)))

    assert got.finding_class is FindingClass.invalid and got.effects == [] and got.reason


def test_the_endpoint_speaks_the_contract() -> None:
    reply = TestClient(create_app()).post("/sensitivity/measure", json=simulated())

    assert reply.status_code == 200
    SensitivityMeasure.model_validate(reply.json())


@cache
def rederived(experiment: int) -> SensitivityMeasure:
    request = real.request(experiment, parameter="ARB_R", extensions_left=0)
    request["look"] = look_of(len({p["block"] for p in request["passes"]}))
    return measure(EvaluateRequest.model_validate(request))


def measurements(experiment: int) -> list[dict[str, Any]]:
    got = rederived(experiment)
    out = []
    for e in got.effects:
        low, high = Estimate(e.estimate, e.standard_error, e.df).interval(0.90)
        out.append(
            STRATUM
            | {"hint_bucket": "unknown", "baseline_value": 2.0}
            | {
                "experiment_id": experiment,
                "parameter": "ARB_R",
                "clicks": 1,
                "channel": e.channel,
                "corner_class": e.corner_class.value,
                "estimate": e.estimate,
                "ci_low": low,
                "ci_high": high,
                "standard_error": e.standard_error,
                "df": e.df,
                "n_passes": e.n_passes,
                "stale": False,
                "note": "",
                "finding_class": got.finding_class.value,
                "engine_version": got.engine_version,
            }
        )
    return out


def test_replay_rear_arb_plus_one_pooled_by_random_effects() -> None:
    """Before (fixed effect, z, the ledger's intervals of all four): -2.64 [-6.46, +1.17] ms.
    After: 7 and 11 are 4-block runs (1 block degree of freedom each) and are left out beside
    the 10- and 8-block runs; 16 and 19 pool with a half-normal tau: -1.84 [-8.03, +4.67] ms,
    insufficient. With all four by HKSJ it would be -2.23 [-7.66, +3.20]."""
    profile = update(request_of([m for e in REAR_ARB for m in measurements(e)]))

    (row,) = [e for e in profile.effects if e.channel == TIME and e.corner_class.value == "all"]
    assert row.method is Method.half_normal and row.experiment_ids == [16, 19]
    assert (row.estimate, row.ci_low, row.ci_high) == pytest.approx(
        (-0.00184, -0.00803, 0.00467), abs=5e-5
    )
    assert row.knowledge_level is KnowledgeLevel.insufficient
    assert [rederived(e).effects[0].df for e in REAR_ARB] == [1, 1, 6, 5]
    assert [(x.experiment_id, x.kind.value) for x in profile.excluded] == [
        (7, "few_blocks"),
        (11, "few_blocks"),
    ]
    four = update(request_of([m | {"df": 3} for e in REAR_ARB for m in measurements(e)]))
    hksj = next(e for e in four.effects if e.channel == TIME and e.corner_class.value == "all")
    assert hksj.method is Method.hksj and hksj.experiments == 4
