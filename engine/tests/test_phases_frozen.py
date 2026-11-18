"""Distance alignment, events, frozen phase cuts and joined passes (ADR 0031)."""

from itertools import pairwise

import numpy as np
import pytest
from hypothesis import given, settings
from hypothesis import strategies as st
from response_data import Shape, analytic_time, sampled

from paddock_engine.phases.align import align, delta_t, unwrap_distance
from paddock_engine.phases.cuts import MIN_PHASE_M, PHASES, freeze_cuts
from paddock_engine.phases.events import CornerWindow, PassEvents, pass_events
from paddock_engine.phases.join import follows, joined, zones_touch
from paddock_engine.phases.reference import corner_reference
from paddock_engine.response.metrics import phase_slice, phase_time

WINDOW = CornerWindow(180.0, 300.0)
ZONE = 400.0


def passes(shapes: list[Shape], seed: int = 1) -> list:  # type: ignore[type-arg]
    rng = np.random.default_rng(seed)
    return [align(sampled(shape, rng), 0.0, 5000.0, ZONE) for shape in shapes]


def test_distance_unwraps_across_the_start_line_and_drops_backward_samples() -> None:
    d = np.array([4990.0, 4995.0, 4994.9, 2.0, 7.0])

    x = unwrap_distance(d, 4980.0, 5000.0)
    aligned = align(sampled(Shape(), np.random.default_rng(3), 4800.0), 4800.0, 5000.0)

    assert x.tolist() == pytest.approx([10.0, 15.0, 14.9, 22.0, 27.0])
    assert aligned.x[0] >= 0 and aligned.x[-1] == pytest.approx(ZONE, abs=1.0)
    assert np.all(np.diff(aligned.x) == 1.0) and np.all(np.diff(aligned.t) > 0)


def test_delta_t_recovers_a_known_time_loss_whatever_the_sampling() -> None:
    base, slow = Shape(), Shape(slow_stretch=(250.0, 320.0, 0.95))
    reference, other = passes([base, slow])
    expected = analytic_time(slow, 0, 399) - analytic_time(base, 0, 399)

    trace = delta_t(other, reference.x, reference.t, 0.0)

    assert expected > 0.05
    assert trace[-2] == pytest.approx(expected, abs=0.002)
    assert abs(trace[int(240 - reference.x[0])]) < 0.002  # nothing lost before the slow stretch


def test_braking_point_is_stable_when_the_apex_jitters() -> None:
    rng = np.random.default_rng(5)
    shapes = [Shape(apex_m=230 + float(rng.uniform(-20, 20))) for _ in range(24)]
    events = [pass_events(p, WINDOW) for p in passes(shapes)]

    braking = [e.braking_m for e in events if e.braking_m is not None]
    apex_relative = [e.apex_m - b for e, b in zip(events, braking, strict=True)]

    assert np.ptp(braking) <= 2.0  # absolute: the point does not move (1 m grid)
    assert np.ptp(apex_relative) >= 30.0  # measured from the apex it moved tens of metres


@settings(max_examples=25, deadline=None)
@given(st.integers(min_value=0, max_value=10_000))
def test_frozen_boundaries_barely_move_when_one_pass_apex_jumps(seed: int) -> None:
    rng = np.random.default_rng(seed)
    shapes = [Shape(apex_m=230 + float(rng.uniform(-15, 15))) for _ in range(11)]
    aligned = passes(shapes, seed)
    before, *_ = corner_reference(1, aligned, WINDOW, ZONE)
    jumped = passes([Shape(apex_m=float(rng.choice([205.0, 255.0])))], seed + 1)

    after, *_ = corner_reference(1, aligned + jumped, WINDOW, ZONE)

    assert after.braking_m == pytest.approx(before.braking_m, abs=1.0)
    assert after.turn_in_m == pytest.approx(before.turn_in_m, abs=1.0)
    assert after.apex_m == pytest.approx(before.apex_m, abs=6.0)


optional_metres = st.one_of(st.none(), st.floats(min_value=0, max_value=ZONE))


@settings(max_examples=200, deadline=None)
@given(
    st.lists(
        st.tuples(optional_metres, st.floats(0, ZONE), st.floats(0, ZONE), optional_metres),
        min_size=1,
        max_size=8,
    ),
    st.floats(min_value=50, max_value=900),
)
def test_phases_are_ordered_and_never_silently_empty(
    raw: list[tuple[float | None, float, float, float | None]], reach: float
) -> None:
    events = [PassEvents(b, ti, a, None, e, False, 100.0) for b, ti, a, e in raw]
    x = np.arange(0.0, 901.0)

    cuts = freeze_cuts(1, events, x, 20.0 + 0.01 * (x - 200) ** 2, ZONE, reach)

    assert [phase.name for phase in cuts.phases] == list(PHASES)
    edges = [p.start_m for p in cuts.phases] + [cuts.phases[-1].end_m]
    assert all(b >= a - 1e-9 for a, b in pairwise(edges))
    for phase in cuts.phases:
        if phase.status == "ok":
            assert phase.end_m - phase.start_m >= MIN_PHASE_M - 1e-9
        else:
            assert phase.note  # a phase that is not cut always says why
    assert cuts.span("mid").status == "ok"
    measured = cuts.measured()
    assert all(a.end_m == b.start_m for a, b in pairwise(measured))


def test_phase_times_sum_to_the_corner_time() -> None:
    aligned = passes([Shape(apex_m=225.0), Shape(apex_m=238.0), Shape()])
    cuts, *_ = corner_reference(1, aligned, WINDOW, ZONE)

    for p in aligned:
        pieces = [phase_slice(p, s.start_m, s.end_m) for s in cuts.measured()]
        total = p.time_at(cuts.measured()[-1].end_m) - p.time_at(cuts.measured()[0].start_m)
        assert sum(phase_time(piece) for piece in pieces if piece) == pytest.approx(total)


def test_a_corner_without_braking_has_an_absent_braking_phase_that_says_so() -> None:
    cuts, *_ = corner_reference(1, passes([Shape(braking_m=None)] * 3), WINDOW, ZONE)

    assert cuts.span("braking").status == "absent"
    assert cuts.span("braking").note == "no braking in the baseline"


def test_joining_the_next_pass_gives_the_traction_phase_its_150_m() -> None:
    rng = np.random.default_rng(9)
    own, after = sampled(Shape(), rng), sampled(Shape(), rng, zone_start_m=ZONE)
    offset = float(own.t[-1]) + 0.02
    alone = align(own, 0.0, 5000.0, ZONE)
    together = align(joined(own, after, offset), 0.0, 5000.0, ZONE)

    short, *_ = corner_reference(1, [alone], WINDOW, ZONE)
    full, *_ = corner_reference(1, [together], WINDOW, ZONE)

    assert zones_touch(ZONE, ZONE + 1.0, 5000.0) and not zones_touch(ZONE, ZONE + 50, 5000.0)
    assert follows(0.0, offset, offset + 0.3) and not follows(0.0, offset, offset + 5.0)
    traction = full.span("traction")
    assert traction.status == "ok" and traction.end_m - traction.start_m == pytest.approx(150)
    assert short.span("traction").end_m - short.span("traction").start_m < 150
