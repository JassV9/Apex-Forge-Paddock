"""The curve classifier of a sensitivity sweep (ADR 0032), on synthetic responses of known shape."""

from collections import Counter
from collections.abc import Callable

import numpy as np
from fastapi.testclient import TestClient
from hypothesis import given
from hypothesis import strategies as st

from paddock_engine.app import create_app
from paddock_engine.contracts.discovery_curve_request_schema import (
    CurveObservable,
    CurveStep,
    DiscoveryCurveRequest,
)
from paddock_engine.discovery.curve import Point, Shape, classify, standard_error
from paddock_engine.discovery.service import points, unit_of

STEPS = (-1, 1, 2, 3)  # Experiment D: rear ARB -1, +1, +2, +3
TRIALS = 400


def simulate(
    truth: Callable[[int], float], se: float, margin: float | None, seed: int
) -> Counter[Shape]:
    """Shapes over TRIALS sweeps whose steps are truth(c) plus normal noise of `se`."""
    rng = np.random.default_rng(seed)
    shapes: Counter[Shape] = Counter()
    for _ in range(TRIALS):
        steps = [Point(c, truth(c) + float(rng.normal(0.0, se)), se) for c in STEPS]
        shapes[classify(steps, margin).shape] += 1
    return shapes


def test_a_linear_response_is_linear_with_its_slope_per_click() -> None:
    shapes = simulate(lambda c: 0.1 * c, 0.02, 0.05, seed=1)
    assert shapes[Shape.linear] >= 0.83 * TRIALS
    assert shapes[Shape.nonlinear] <= 0.15 * TRIALS  # the curvature test's nominal 10 %
    exact = classify([Point(c, 0.1 * c, 0.02) for c in STEPS], 0.05)
    assert exact.shape is Shape.linear and exact.slope is not None
    assert abs(exact.slope.estimate - 0.1) < 1e-12 and exact.slope.low < 0.1 < exact.slope.high


def test_a_quadratic_response_is_nonlinear() -> None:
    shapes = simulate(lambda c: 0.1 * c + 0.05 * c * c, 0.02, 0.05, seed=2)
    assert shapes[Shape.nonlinear] >= 0.9 * TRIALS


def test_a_flat_response_is_negligible_inside_its_margin_and_insufficient_without_one() -> None:
    assert simulate(lambda c: 0.0, 0.01, 0.05, seed=3)[Shape.negligible] >= 0.95 * TRIALS
    unbounded = simulate(lambda c: 0.0, 0.01, None, seed=4)
    assert unbounded[Shape.negligible] == 0
    assert unbounded[Shape.insufficient] >= 0.7 * TRIALS


def test_a_response_that_rises_then_falls_is_a_direction_change_where_it_turns() -> None:
    shapes = simulate(lambda c: 0.2 * c - 0.1 * c * c, 0.02, 0.05, seed=5)
    assert shapes[Shape.direction_change] >= 0.9 * TRIALS
    exact = classify([Point(c, 0.2 * c - 0.1 * c * c, 0.02) for c in STEPS], 0.05)
    assert exact.shape is Shape.direction_change and exact.turn_clicks == 1


def test_both_directions_raising_it_turns_at_the_baseline() -> None:
    v_shape = classify([Point(-1, 0.2, 0.02), Point(1, 0.2, 0.02)], 0.05)
    assert v_shape.shape is Shape.direction_change and v_shape.turn_clicks == 0


def test_noise_larger_than_the_effect_is_insufficient() -> None:
    found = classify([Point(c, 0.01 * c, 0.5) for c in STEPS], 0.05)
    assert found.shape is Shape.insufficient


@given(
    clicks=st.integers(-6, 6).filter(lambda c: c != 0),
    estimate=st.floats(-10.0, 10.0),
    se=st.floats(1e-4, 10.0),
    repeats=st.integers(1, 3),
    margin=st.one_of(st.none(), st.floats(1e-3, 100.0)),
)
def test_one_step_never_yields_a_curve(
    clicks: int, estimate: float, se: float, repeats: int, margin: float | None
) -> None:
    found = classify([Point(clicks, estimate, se)] * repeats, margin)
    assert found.shape is Shape.insufficient
    assert found.slope is None and found.curvature is None


def test_the_baseline_and_unusable_steps_are_not_steps() -> None:
    steps = [
        Point(0, 5.0, 0.01),
        Point(1, 0.3, 0.01),
        Point(2, 0.6, 0.0),
        Point(3, float("nan"), 0.01),
    ]
    assert classify(steps, 0.05).shape is Shape.insufficient


_sweep = st.lists(
    st.tuples(st.floats(-1.0, 1.0), st.floats(1e-3, 1.0)), min_size=len(STEPS), max_size=len(STEPS)
)


@given(sweep=_sweep, margin=st.one_of(st.none(), st.floats(1e-3, 1.0)))
def test_the_shape_does_not_depend_on_the_sign_or_the_order(
    sweep: list[tuple[float, float]], margin: float | None
) -> None:
    steps = [Point(c, y, se) for c, (y, se) in zip(STEPS, sweep, strict=True)]
    found = classify(steps, margin)
    flipped = classify([Point(p.clicks, -p.estimate, p.se) for p in steps], margin)
    shuffled = classify(list(reversed(steps)), margin)
    assert flipped.shape is found.shape and shuffled.shape is found.shape
    if found.slope is not None and flipped.slope is not None:
        assert flipped.slope.estimate == -found.slope.estimate


@given(sweep=_sweep, margin=st.floats(1e-3, 1.0), power=st.integers(-4, 4))
def test_the_shape_does_not_depend_on_the_unit(
    sweep: list[tuple[float, float]], margin: float, power: int
) -> None:
    scale = 2.0**power  # exact in binary floating point
    steps = [Point(c, y, se) for c, (y, se) in zip(STEPS, sweep, strict=True)]
    scaled = [Point(p.clicks, p.estimate * scale, p.se * scale) for p in steps]
    assert classify(scaled, margin * scale).shape is classify(steps, margin).shape


def test_an_interval_at_a_look_is_read_at_the_looks_level() -> None:
    nominal = CurveStep(clicks=1, estimate=0.0, ci_low=-0.01, ci_high=0.01, level=0.9)
    at_look = nominal.model_copy(update={"look": 1})
    observable = CurveObservable(
        metric="corner_time_s",
        phase="corner",
        regime="all",
        source="verdict",
        steps=[nominal, at_look],
    )
    read = points(observable)
    assert abs(read[0].se - standard_error(-0.01, 0.01, 0.9)) < 1e-15
    assert read[1].se < read[0].se  # the same width at 98 % means a smaller standard error


def test_units_come_from_the_configuration_or_the_name() -> None:
    assert unit_of("rear_slip_deg") == "deg"
    assert unit_of("corner_time_s") == "s"
    assert unit_of("rear_slip_peak_deg") == "deg"
    assert unit_of("brake_dist_m") == "m"
    assert unit_of("mystery") == ""


def test_the_endpoint_classifies_each_observable() -> None:
    def observable(metric: str, slope: float, margin: float | None) -> CurveObservable:
        steps = [
            CurveStep(clicks=c, estimate=y, ci_low=y - 0.03, ci_high=y + 0.03, level=0.9)
            for c, y in ((c, slope * c) for c in STEPS)
        ]
        return CurveObservable(metric=metric, phase="mid", regime="slow", source="response",
                               expected="direct", margin=margin, steps=steps)  # fmt: skip

    request = DiscoveryCurveRequest(
        parameter="ARB_REAR",
        observables=[observable("rear_slip_deg", 0.1, 0.05), observable("yaw_gain", 0.0, 0.05)],
    )
    single = request.model_copy(deep=True)
    single.observables[0].steps = single.observables[0].steps[:1]
    client = TestClient(create_app())

    reply = client.post("/discovery/curve", json=request.model_dump(mode="json", exclude_none=True))
    one = client.post("/discovery/curve", json=single.model_dump(mode="json", exclude_none=True))

    assert reply.status_code == 200, reply.text
    slip, yaw = reply.json()["curves"]
    assert (slip["shape"], slip["unit"], slip["expected"]) == ("linear", "deg", "direct")
    assert abs(slip["slope"]["estimate"] - 0.1) < 1e-9 and slip["clicks"] == [-1, 1, 2, 3]
    assert yaw["shape"] == "negligible"
    assert one.json()["curves"][0]["shape"] == "insufficient"
