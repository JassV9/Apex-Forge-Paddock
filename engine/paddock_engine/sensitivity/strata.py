"""Which measurements may be pooled together (ADR 0033).

A stratum is everything that may change what one click does: the item, the clicks, the channel,
the corner class and phase, the track (a layout and its `_paddock` test copy are one track, ADR
0027), the driver (AI or human), the AI's planning hint (ADR 0028), the item's value on the
baseline and the size of one click. Measurements of different strata are never pooled into one
row. A row pooled over every track ("all") is made beside the per-track rows when an otherwise
identical stratum was measured on more than one track; it is labelled and never replaces them.

Never pooled:

- an experiment whose finding is **invalid** (ADR 0029): listed as excluded;
- a measurement with **fewer than 3 block degrees of freedom** (a 4-block experiment: 1 or 2) in a
  stratum that has better ones: listed as excluded. Its standard error rests on one or two block
  contrasts, so weighting by it is weighting by noise (experiment 8 on the FH26: 0.06 ms on 1
  degree of freedom). A stratum with only such experiments is pooled from them;
- a **stale** measurement (the ledger's numbers, typically from an engine before ADR 0029, which
  core could not re-derive) in a stratum that has re-derived ones: listed as excluded. A stratum
  with only stale measurements is pooled and marked stale, which caps it at insufficient.
"""

from collections import defaultdict
from collections.abc import Iterable, Sequence
from dataclasses import dataclass, replace

from paddock_engine.contracts.sensitivity_request_schema import FindingClass, Measurement
from paddock_engine.contracts.sensitivity_response_schema import ExcludedMeasurement, Kind
from paddock_engine.sensitivity.pooling import Estimate, from_interval

ALL_TRACKS = "all"
ALL_TRACKS_NAME = "All tracks"
MIN_POOLED_DF = 3
FEW_BLOCKS_REASON = (
    "Fewer than 3 block degrees of freedom; its stratum has experiments with more, "
    "and its error is too uncertain to weight it."
)
STALE_REASON = (
    "Not re-derived by the current engine (the ledger's numbers); "
    "its stratum has re-derived experiments."
)


@dataclass(frozen=True)
class Stratum:
    parameter: str
    clicks: int
    channel: str
    corner_class: str
    phase: str
    track: str
    driver: str
    hint_bucket: str
    baseline_value: float | None
    click_step: float | None

    def change(self) -> tuple[str, int, str, str, str]:
        """The change regardless of where it was measured: what a prior stands in for."""
        return (self.parameter, self.clicks, self.channel, self.corner_class, self.phase)

    def order(self) -> tuple[object, ...]:
        unknown = float("-inf")
        values = (self.baseline_value, self.click_step)
        return (
            *self.change(),
            self.track,
            self.driver,
            self.hint_bucket,
            *(unknown if v is None else v for v in values),
        )

    def response(self) -> "Stratum":
        """The stratum with clicks set aside: the item's response curve (sensitivity/shape.py)."""
        return replace(self, clicks=0)


@dataclass(frozen=True)
class Group:
    stratum: Stratum
    track_name: str
    measurements: list[Measurement]
    stale: bool


def stratum_of(item: Measurement, track: str | None = None) -> Stratum:
    return Stratum(
        parameter=item.parameter,
        clicks=item.clicks,
        channel=item.channel,
        corner_class=item.corner_class.value,
        phase=item.phase,
        track=item.track if track is None else track,
        driver=item.driver.value,
        hint_bucket=item.hint_bucket,
        baseline_value=item.baseline_value,
        click_step=item.click_step,
    )


def estimate_of(item: Measurement, level: float) -> Estimate:
    """The re-derived block-level error; for a stale row the ledger's interval read back with
    a normal quantile (its degrees of freedom were never stored), which overstates the error of
    a t interval: conservative."""
    if item.standard_error is not None and not item.stale:
        return Estimate(mean=item.estimate, se=max(item.standard_error, 1e-12), df=item.df)
    if item.ci_low is None or item.ci_high is None:
        raise ValueError(f"experiment {item.experiment_id}: no standard error and no interval")
    return from_interval(item.estimate, item.ci_low, item.ci_high, level)


def _excluded(items: Iterable[Measurement], kind: Kind, reason: str) -> list[ExcludedMeasurement]:
    ids = sorted({item.experiment_id for item in items})
    return [ExcludedMeasurement(experiment_id=i, kind=kind, reason=reason) for i in ids]


def _tier(item: Measurement) -> int:
    """0: re-derived on enough blocks; 1: on fewer than 3 degrees of freedom; 2: stale."""
    if item.stale:
        return 2
    return 0 if item.df is None or item.df >= MIN_POOLED_DF else 1


def _split(
    stratum: Stratum, name: str, items: Sequence[Measurement]
) -> tuple[Group, list[Measurement]]:
    """The best tier of the stratum is pooled; the others are left out (and listed)."""
    best = min(_tier(item) for item in items)
    kept = [item for item in items if _tier(item) == best]
    left = [item for item in items if _tier(item) != best]
    return Group(stratum, name, kept, stale=best == 2), left


def _across_tracks(
    by_stratum: dict[Stratum, list[Measurement]],
) -> dict[Stratum, list[Measurement]]:
    pooled: dict[Stratum, list[Measurement]] = defaultdict(list)
    tracks: dict[Stratum, set[str]] = defaultdict(set)
    for stratum, items in by_stratum.items():
        everywhere = replace(stratum, track=ALL_TRACKS)
        pooled[everywhere] += items
        tracks[everywhere].add(stratum.track)
    return {stratum: items for stratum, items in pooled.items() if len(tracks[stratum]) > 1}


def group(measurements: Sequence[Measurement]) -> tuple[list[Group], list[ExcludedMeasurement]]:
    """The strata to pool, in order, and every experiment left out with the reason."""
    usable = [m for m in measurements if m.clicks != 0]
    invalid = [m for m in usable if m.finding_class is FindingClass.invalid]
    excluded = _excluded(invalid, Kind.invalid, "Its finding is invalid (ADR 0029); never pooled.")
    by_stratum: dict[Stratum, list[Measurement]] = defaultdict(list)
    names: dict[str, str] = {ALL_TRACKS: ALL_TRACKS_NAME}
    for item in usable:
        if item.finding_class is not FindingClass.invalid:
            by_stratum[stratum_of(item)].append(item)
            names.setdefault(item.track, item.track_name)
    groups, left = [], []
    every = {**by_stratum, **_across_tracks(by_stratum)}
    for stratum in sorted(every, key=Stratum.order):
        items = every[stratum]
        kept, out = _split(stratum, names[stratum.track], items)
        groups.append(kept)
        left += out
    excluded += _excluded([m for m in left if m.stale], Kind.stale, STALE_REASON)
    excluded += _excluded([m for m in left if not m.stale], Kind.few_blocks, FEW_BLOCKS_REASON)
    return groups, sorted(excluded, key=lambda e: (e.experiment_id, e.kind.value))
