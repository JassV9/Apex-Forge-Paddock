"""The engine's response analysis as the ResponseResponse contract."""

from paddock_engine.contracts.response_response_schema import (
    Expected,
    Finding,
    Kind,
    Method,
    Name,
    Phase,
    Regime,
    Regime1,
    ResponseAeroFit,
    ResponseCornerCuts,
    ResponseDeltaPoint,
    ResponseEffect,
    ResponsePhaseSpan,
    ResponseRegimes,
    Status,
)
from paddock_engine.features.regimes import MIN_R2, AeroFit, Regimes
from paddock_engine.response.config import ParameterRow
from paddock_engine.response.effects import CellEffect
from paddock_engine.response.passes import CornerView


def corner_out(
    number: int, view: CornerView, regime: str, fit: AeroFit | None, counts: tuple[int, int]
) -> ResponseCornerCuts:
    """`fit` only when adequate: an inadequate fit gives no share at all."""
    cuts = view.cuts
    phases = [
        ResponsePhaseSpan(
            name=Name(s.name), start_m=s.start_m, end_m=s.end_m, status=Status(s.status),
            note=s.note,
        )
        for s in cuts.phases
    ]  # fmt: skip
    return ResponseCornerCuts(
        number=number, regime=Regime(regime), v_min_kmh=view.baseline_v_min_kmh,
        aero_share_estimate=fit.share(view.baseline_v_min_kmh) if fit else None,
        braking_m=cuts.braking_m, turn_in_m=cuts.turn_in_m, apex_m=cuts.apex_m,
        pickup_m=cuts.pickup_m, end_m=cuts.end_m, mid_half_m=cuts.mid_half_m,
        apex_by_curvature=cuts.by_curvature, phases=phases, flags=cuts.flags,
        delta_t=[ResponseDeltaPoint(x_m=x, delta_s=d) for x, d in view.delta_t],
        baseline_passes=counts[0], variant_passes=counts[1],
    )  # fmt: skip


def effect_out(cell: CellEffect, margin: float, row: ParameterRow | None) -> ResponseEffect:
    expected = row.observes(cell.metric, cell.phase, cell.regime) if row else None
    return ResponseEffect(
        metric=cell.metric, phase=Phase(cell.phase), regime=Regime1(cell.regime),
        kind=Kind(cell.kind), finding=Finding(cell.finding), estimate=cell.estimate,
        ci_low=cell.ci_low, ci_high=cell.ci_high, baseline_mean=cell.baseline_mean,
        margin=margin, n_passes=cell.n_passes, corners=cell.corners,
        expected=Expected(expected or "none"), note=cell.note,
    )  # fmt: skip


def regimes_out(regimes: Regimes, fit: AeroFit | None) -> ResponseRegimes:
    aero = None
    if fit is not None:
        aero = ResponseAeroFit(
            c0_g=fit.c0_g, c1_g_per_ms2=fit.c1_g_per_ms2, r2=fit.r2, corners=fit.corners,
            adequate=fit.adequate,
        )  # fmt: skip
    return ResponseRegimes(
        method=Method(regimes.method), breaks_kmh=list(regimes.breaks_kmh), aero_fit=aero
    )


def notes(fit: AeroFit | None, row: ParameterRow | None, cells: int, level: float) -> list[str]:
    out = [
        f"{cells} cells, each a {level:.0%} interval with no correction for their number: about "
        f"{1 - level:.0%} of cells exclude zero by chance.",
        "Speed regimes are per corner, frozen from the baseline (ADR 0031).",
    ]
    if fit is not None and not fit.adequate:
        out.append(f"No aero share: the fit's R^2 is {fit.r2:.2f} (it needs {MIN_R2}).")
    if row is None:
        out.append("The changed item matched no parameter row: no cell is marked expected.")
    return out
