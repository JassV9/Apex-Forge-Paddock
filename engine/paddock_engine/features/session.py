"""Features for every pass of a session: the work behind POST /features/session."""

from pathlib import Path

from paddock_engine.contracts.features_request_schema import FeaturePassInput, FeaturesRequest
from paddock_engine.contracts.features_response_schema import FeaturesResponse, SpeedClass
from paddock_engine.contracts.features_response_schema import PassFeatures as PassFeaturesOut
from paddock_engine.features.pass_features import PassFeatures, compute, from_legacy
from paddock_engine.io.samples import PassSamples, read_session_samples
from paddock_engine.phases.segment import Corner


def _no_data(duration_s: float, reason: str) -> PassFeatures:
    return PassFeatures({"corner_time_s": duration_s}, "slow", valid=False, invalid_reason=reason)


def _one_pass(
    item: FeaturePassInput,
    samples: PassSamples | None,
    corners: dict[int, Corner],
    request: FeaturesRequest,
) -> PassFeatures:
    corner = corners.get(item.corner)
    if samples is not None and corner is not None:
        return compute(
            samples, corner, request.track_length_m, item.duration_s, request.driven_axle.value
        )
    if item.legacy is not None:
        return from_legacy(item.legacy, item.duration_s)
    if corner is None:
        return _no_data(item.duration_s, "unknown corner")
    return _no_data(item.duration_s, "no samples")


def session_features(request: FeaturesRequest) -> FeaturesResponse:
    """A pass with raw samples gets the full feature set; an imported v0 pass gets what its
    legacy summary holds; a pass with neither is returned as not valid."""
    corners = {
        c.number: Corner(c.number, c.zone_start_m, c.start_m, c.end_m, c.apex_m, c.zone_end_m)
        for c in request.corners
    }
    samples = read_session_samples(Path(request.parquet_dir)) if request.parquet_dir else {}
    out = []
    for item in request.passes:
        features = _one_pass(item, samples.get(item.seq), corners, request)
        out.append(
            PassFeaturesOut(
                seq=item.seq,
                valid=features.valid,
                invalid_reason=features.invalid_reason,
                speed_class=SpeedClass(features.speed_class),
                metrics=features.metrics,
                flags=features.flags,
            )
        )
    return FeaturesResponse(passes=out)
