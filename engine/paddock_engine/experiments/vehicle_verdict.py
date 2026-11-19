"""The response part of a verdict (ADR 0034): the measured passes, the findings on the expected
cells, and what the reader should know when there are none."""

from dataclasses import dataclass

from paddock_engine.contracts.evaluate_request_schema import EvaluateRequest
from paddock_engine.contracts.evaluate_response_schema import ResponseFinding, ResponseNote
from paddock_engine.experiments.response_findings import RESPONSE_ALPHA, response_findings
from paddock_engine.experiments.vehicle import Vehicle, vehicle
from paddock_engine.response.service import Samples


@dataclass(frozen=True)
class VehicleVerdict:
    data: Vehicle | None  # None: no response context, no samples, or no baseline samples
    findings: list[ResponseFinding] | None  # None: core sent no response context
    parameter: str | None = None
    notes: list[ResponseNote] | None = None


def _without(note: str) -> VehicleVerdict:
    return VehicleVerdict(data=None, findings=[], notes=[ResponseNote(note)])


def vehicle_verdict(request: EvaluateRequest, samples: Samples | None) -> VehicleVerdict:
    if request.response is None:
        return VehicleVerdict(data=None, findings=None)
    if samples is None:
        return _without("The raw samples could not be read: no response findings, no phase test.")
    data = vehicle(request.response, samples, request.success_test.channel)
    if data is None:
        return _without("No baseline pass has raw samples: no response findings, no phase test.")
    if data.row is None:
        note = "The changed item matched no parameter row: no cell is expected to move."
        return VehicleVerdict(data=data, findings=[], notes=[ResponseNote(note)])
    findings = response_findings(data, request.parameter, request.success_test.level)
    note = (
        f"{len(findings)} expected cells of {data.row.key}, Holm across them at "
        f"{RESPONSE_ALPHA:.0%}; cells it is not expected to move are descriptive only "
        "(POST /response) and never drive a finding."
    )
    return VehicleVerdict(
        data=data, findings=findings, parameter=data.row.key, notes=[ResponseNote(note)]
    )
