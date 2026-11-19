"""POST /response: how an experiment's variant changed the car's response (ADR 0031).

Per corner: the phase cuts frozen from the baseline, the corner's speed regime (frozen from the
baseline too), and the aero-share ESTIMATE where its fit is adequate. Then one effect per
configured (metric, phase, regime), from the existing effect model. Observables the recorded
channels cannot give are listed as unavailable, never filled in.
"""

from collections.abc import Callable, Mapping
from pathlib import Path

from paddock_engine.contracts.response_request_schema import ResponseRequest
from paddock_engine.contracts.response_response_schema import (
    ResponseResponse,
    ResponseUnavailable,
)
from paddock_engine.features.regimes import AeroFit, Regimes, aero_fit, corner_regimes
from paddock_engine.io.samples import PassSamples, read_session_samples
from paddock_engine.response.config import Observables, observables
from paddock_engine.response.effects import all_effects
from paddock_engine.response.output import corner_out, effect_out, notes, regimes_out
from paddock_engine.response.passes import (
    BASELINE,
    CornerView,
    Geometry,
    Measured,
    PassRef,
    aligned_passes,
    corner_views,
    measured_passes,
)

DEFAULT_LEVEL = 0.90
Samples = Mapping[tuple[int, int], PassSamples]
Loader = Callable[[Path], dict[int, PassSamples]]


def _refs(request: ResponseRequest) -> list[PassRef]:
    return [
        PassRef(
            session=p.session, seq=p.seq, corner=p.corner, arm=p.arm.value, block=p.block,
            t_start_s=p.t_start_s, duration_s=p.duration_s, valid=p.valid, lap=p.lap,
            covariates=p.covariates or {},
        )
        for p in request.passes
    ]  # fmt: skip


def load_samples(request: ResponseRequest, load: Loader = read_session_samples) -> Samples:
    """Every pass's samples, keyed by (session, seq)."""
    out: dict[tuple[int, int], PassSamples] = {}
    for session in request.sessions:
        for seq, samples in load(Path(session.parquet_dir)).items():
            out[(session.session, seq)] = samples
    return out


def regimes_of(views: Mapping[int, CornerView]) -> tuple[Regimes, AeroFit | None]:
    """Classes and the aero fit from the baseline medians of each corner, never a pass."""
    regimes = corner_regimes((n, v.baseline_v_min_kmh, None) for n, v in views.items())
    fit = aero_fit({n: (v.baseline_v_min_kmh, v.baseline_lat_peak_g) for n, v in views.items()})
    return regimes, fit


def _arm_counts(measured: list[Measured], corner: int) -> tuple[int, int]:
    arms = [m.ref.arm for m in measured if m.ref.corner == corner]
    return arms.count(BASELINE), len(arms) - arms.count(BASELINE)


def response(
    request: ResponseRequest, samples: Samples, config: Observables | None = None
) -> ResponseResponse:
    config = config or observables()
    level = request.level or DEFAULT_LEVEL
    length = request.track_length_m
    geometry = {
        c.number: Geometry(c.number, c.zone_start_m, c.start_m, c.end_m, c.zone_end_m)
        for c in request.corners
    }
    refs = _refs(request)
    passes = aligned_passes(refs, samples, geometry, length)
    views = corner_views(passes, geometry, length)
    measured = measured_passes(passes, views, geometry, length, config.cells())
    regimes, fit = regimes_of(views)
    row = config.row_for(request.item_role or "", request.item_axle or "")
    cells = all_effects(measured, config.metrics, regimes.classes, level)
    shown_fit = fit if fit is not None and fit.adequate else None
    return ResponseResponse(
        regimes=regimes_out(regimes, fit),
        corners=[
            corner_out(n, view, regimes.classes[n], shown_fit, _arm_counts(measured, n))
            for n, view in views.items()
        ],
        effects=[effect_out(c, config.metrics[c.metric].margin, row) for c in cells],
        unavailable=[
            ResponseUnavailable(observable=k, reason=v) for k, v in config.unavailable.items()
        ],
        parameter=row.key if row else None,
        passes_used=len(measured),
        passes_excluded=len(refs) - len(measured),
        notes=notes(fit, row, len(cells), level),
    )
