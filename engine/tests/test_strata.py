"""Strata are never mixed, invalid experiments never pooled, stale ones marked (ADR 0033)."""

from typing import Any

import pytest
from sensitivity_data import request_of, synthetic

from paddock_engine.contracts.sensitivity_response_schema import (
    KnowledgeLevel,
    SensitivityEffect,
    SensitivityResponse,
)
from paddock_engine.sensitivity.profile import update

COTA = {"track": "fn_cota/layout_f1_2026", "track_name": "COTA F1 2026"}


def rows(profile: SensitivityResponse, **match: Any) -> list[SensitivityEffect]:
    return [e for e in profile.effects if all(getattr(e, k) == v for k, v in match.items())]


def test_different_ai_hints_are_never_pooled_into_one_row() -> None:
    low = [synthetic(i, "ARB_R", 1, -0.010, 0.002, hint_bucket="0.89") for i in (1, 2, 3)]
    high = [synthetic(i, "ARB_R", 1, +0.010, 0.002, hint_bucket="0.97") for i in (4, 5, 6)]

    profile = update(request_of(low + high))

    by_hint = {e.hint_bucket: e for e in profile.effects}
    assert set(by_hint) == {"0.89", "0.97"}
    assert by_hint["0.89"].experiment_ids == [1, 2, 3] and by_hint["0.89"].estimate < 0
    assert by_hint["0.97"].experiment_ids == [4, 5, 6] and by_hint["0.97"].estimate > 0


def test_different_tracks_get_their_own_rows_and_a_separate_all_track_row() -> None:
    hungaroring = [synthetic(i, "ARB_R", 1, -0.010, 0.002) for i in (1, 2)]
    cota = [synthetic(i, "ARB_R", 1, +0.004, 0.002, **COTA) for i in (3, 4)]

    profile = update(request_of(hungaroring + cota))

    assert {e.track: e.experiment_ids for e in profile.effects} == {
        "fn_hungaroring/layout_f1_2026": [1, 2],
        "fn_cota/layout_f1_2026": [3, 4],
        "all": [1, 2, 3, 4],
    }
    (everywhere,) = rows(profile, track="all")
    assert everywhere.track_name == "All tracks" and everywhere.method.value == "hksj"


def test_other_strata_split_too_baseline_value_click_size_driver_and_phase() -> None:
    base = synthetic(1, "ARB_R", 1, -0.01, 0.002)
    variants = [
        base,
        synthetic(2, "ARB_R", 1, -0.01, 0.002, baseline_value=5.0),
        synthetic(3, "ARB_R", 1, -0.01, 0.002, click_step=0.5),
        synthetic(4, "ARB_R", 1, -0.01, 0.002, driver="human", hint_bucket="none"),
        synthetic(5, "ARB_R", 1, -0.01, 0.002, phase="exit"),
    ]

    profile = update(request_of(variants))

    assert sorted(e.experiment_ids[0] for e in profile.effects) == [1, 2, 3, 4, 5]
    assert all(e.experiments == 1 for e in profile.effects)


def test_an_invalid_experiment_is_never_pooled_and_is_listed() -> None:
    good = [synthetic(i, "ARB_R", 1, -0.010, 0.002) for i in (1, 2, 3)]
    broken = synthetic(4, "ARB_R", 1, -0.500, 0.001, finding_class="invalid")

    profile = update(request_of([*good, broken]))

    (row,) = profile.effects
    assert row.experiment_ids == [1, 2, 3]
    assert [(e.experiment_id, e.kind.value) for e in profile.excluded] == [(4, "invalid")]
    only_broken = update(request_of([broken]))
    assert only_broken.effects == [] and only_broken.excluded[0].experiment_id == 4


def test_a_stale_measurement_is_left_out_beside_rederived_ones_and_alone_is_insufficient() -> None:
    current = [synthetic(i, "ARB_R", 1, -0.010, 0.002) for i in (11, 16, 19)]
    stale = synthetic(7, "ARB_R", 1, -0.01575, 0.0, stale=True)
    stale |= {"ci_low": -0.0532, "ci_high": 0.0217, "note": "the passes are gone"}
    stale.pop("standard_error"), stale.pop("df"), stale.pop("engine_version")

    mixed = update(request_of([*current, stale]))

    (row,) = mixed.effects
    assert row.experiment_ids == [11, 16, 19] and not row.stale
    assert [(e.experiment_id, e.kind.value) for e in mixed.excluded] == [(7, "stale")]
    (alone,) = update(request_of([stale])).effects
    assert alone.stale and alone.knowledge_level is KnowledgeLevel.insufficient
    assert alone.engine_versions == [] and "Not re-derived" in alone.note
    # one experiment keeps the ledger's interval (read back with z: its df was never stored)
    assert (alone.ci_low, alone.ci_high) == pytest.approx((-0.0532, 0.0217))


def test_a_four_block_experiment_is_left_out_beside_longer_ones_and_kept_alone() -> None:
    long = [synthetic(i, "ARB_R", 1, -0.004, 0.0025, df=6) for i in (16, 19)]
    short = synthetic(8, "ARB_R", 1, 0.0067, 0.00006, df=1)  # an error that is noise

    mixed = update(request_of([*long, short]))

    (row,) = mixed.effects
    assert row.experiment_ids == [16, 19]
    assert [(e.experiment_id, e.kind.value) for e in mixed.excluded] == [(8, "few_blocks")]
    (alone,) = update(request_of([short])).effects
    assert alone.experiment_ids == [8] and alone.method.value == "single"
