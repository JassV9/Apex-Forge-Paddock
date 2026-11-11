"""A synthetic closed track for tests that must run without an Assetto Corsa install.

The line is a polygon with rounded vertices: straights joined by arcs of chosen radius, both
left and right turns, from a tight hairpin to a fast sweeper. The start line is in the middle
of the first straight.
"""

import itertools
import math
from dataclasses import dataclass

Point = tuple[float, float]

# (x, z, fillet radius in metres)
VERTICES: list[tuple[float, float, float]] = [
    (0.0, 0.0, 45.0),
    (900.0, 0.0, 60.0),
    (980.0, 420.0, 150.0),
    (560.0, 560.0, 30.0),
    (470.0, 300.0, 80.0),
    (200.0, 520.0, 45.0),
    (-150.0, 330.0, 110.0),
]
SPACING_M = 4.0


@dataclass(frozen=True)
class Track:
    points: list[Point]
    length: float


def _unit(a: Point, b: Point) -> Point:
    dx, dz = b[0] - a[0], b[1] - a[1]
    size = math.hypot(dx, dz)
    return dx / size, dz / size


def _fillet(prev: Point, vertex: Point, nxt: Point, radius: float) -> list[Point]:
    """Points along the arc that rounds `vertex`, from the incoming to the outgoing edge."""
    u_in, u_out = _unit(prev, vertex), _unit(vertex, nxt)
    turn = math.atan2(
        u_in[0] * u_out[1] - u_in[1] * u_out[0], u_in[0] * u_out[0] + u_in[1] * u_out[1]
    )
    tangent = radius * math.tan(abs(turn) / 2)
    start = (vertex[0] - u_in[0] * tangent, vertex[1] - u_in[1] * tangent)
    side = 1.0 if turn > 0 else -1.0
    centre = (start[0] - u_in[1] * radius * side, start[1] + u_in[0] * radius * side)
    angle0 = math.atan2(start[1] - centre[1], start[0] - centre[0])
    steps = max(8, int(abs(turn) * radius))
    return [
        (
            centre[0] + radius * math.cos(angle0 + turn * step / steps),
            centre[1] + radius * math.sin(angle0 + turn * step / steps),
        )
        for step in range(steps + 1)
    ]


def _outline() -> list[Point]:
    corners = [(x, z) for x, z, _ in VERTICES]
    count = len(corners)
    start = ((corners[0][0] + corners[1][0]) / 2, (corners[0][1] + corners[1][1]) / 2)
    outline = [start]
    for offset in range(1, count + 1):
        index = offset % count
        arc = _fillet(
            corners[index - 1], corners[index], corners[(index + 1) % count], VERTICES[index][2]
        )
        outline.extend(arc)
    return outline


def _resample(outline: list[Point]) -> Track:
    closed = [*outline, outline[0]]
    cumulative = [0.0]
    for a, b in itertools.pairwise(closed):
        cumulative.append(cumulative[-1] + math.hypot(b[0] - a[0], b[1] - a[1]))
    length = cumulative[-1]
    count = int(length / SPACING_M)
    points: list[Point] = []
    segment = 0
    for i in range(count):
        distance = length * i / count
        while cumulative[segment + 1] <= distance:
            segment += 1
        a, b = closed[segment], closed[segment + 1]
        span = cumulative[segment + 1] - cumulative[segment]
        share = 0.0 if span <= 0 else (distance - cumulative[segment]) / span
        points.append((a[0] + (b[0] - a[0]) * share, a[1] + (b[1] - a[1]) * share))
    return Track(points=points, length=length)


def synthetic_track() -> Track:
    return _resample(_outline())
