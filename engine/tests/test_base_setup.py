"""A base setup from the car's own measurements (docs/adr/0018-base-setup.md)."""

from typing import Any

import pytest
from fastapi.testclient import TestClient
from hypothesis import given, settings
from hypothesis import strategies as st

from paddock_engine.app import create_app
from paddock_engine.contracts.base_setup_request_schema import BaseSetupRequest
from paddock_engine.contracts.base_setup_schema import BaseSetup
from paddock_engine.optimize.base_setup import base_setup, predict
from paddock_engine.sensitivity.pooling import quantile
from paddock_engine.stats.rope import classify

TIME, REAR_SLIP = "corner_time_s", "rear_slip_peak_deg"
ROPE, LEVEL = 0.005, 0.90


def effect(
    parameter: str, clicks: int, estimate: float, low: float, high: float, **changes: Any
) -> dict[str, Any]:
    channel = changes.pop("channel", TIME)
    rope = 0.5 if channel == REAR_SLIP else ROPE
    row = {
        "parameter": parameter,
        "clicks": clicks,
        "channel": channel,
        "corner_class": "all",
        "estimate": estimate,
        "ci_low": low,
        "ci_high": high,
        "n_passes": 84,
        "effect_class": classify(low, high, rope).value,
        "nonlinear": False,
        "note": "",
        "is_prior": False,
        "prior_source": "",
        "experiment_ids": [7],
    }
    return row | changes


def item(
    name: str, value: float = 3, low: float = 0, high: float = 6, **changes: Any
) -> dict[str, Any]:
    fields = {"name": name, "label": name.title(), "min": low, "max": high, "step": 1}
    return fields | {"value": value, "paired": False} | changes


def build(effects: list[dict[str, Any]], items: list[dict[str, Any]] | None = None) -> BaseSetup:
    request = BaseSetupRequest.model_validate(
        {
            "effects": effects,
            "items": items
            if items is not None
            else [item("ARB_FRONT"), item("ARB_REAR"), item("WING")],
            "channel": TIME,
            "better": "lower",
            "rope": ROPE,
            "level": LEVEL,
            "guardrails": [{"channel": REAR_SLIP, "tolerance": 0.5, "worse": "higher"}],
        }
    )
    return base_setup(request)


def reasons(result: BaseSetup) -> dict[tuple[str, int], str]:
    return {(entry.parameter, entry.clicks): entry.reason for entry in result.left_out}


def test_a_change_measured_faster_goes_in_even_inside_the_band() -> None:
    """The real case on the 488: -4.2 ms [-6.8, -1.6], inconclusive against 5 ms, qualifies."""
    result = build([effect("ARB_FRONT", -1, -0.0042, -0.0068, -0.0016)])

    (change,) = result.changes
    assert (change.parameter, change.clicks, change.from_value, change.to_value) == (
        "ARB_FRONT",
        -1,
        3,
        2,
    )
    assert change.effect_class.value == "inconclusive"
    assert change.experiment_ids == [7]
    assert result.prediction is not None
    assert result.prediction.estimate == pytest.approx(-0.0042)
    assert result.prediction.ci_high < 0
    assert "interact" in result.prediction.note


def test_what_cannot_go_in_is_left_out_with_the_reason() -> None:
    effects = [
        effect("ARB_FRONT", 1, 0.002, -0.001, 0.005),
        effect(
            "ARB_REAR",
            -1,
            -0.02,
            -0.03,
            -0.01,
            is_prior=True,
            prior_source="Formula Alpha",
            experiment_ids=[],
        ),
        effect("WING", 1, -0.02, -0.03, -0.01, nonlinear=True, note="1 and -1 do not cancel"),
        effect("WING", -1, -0.01, -0.02, -0.005),
        effect("WING", -1, 0.9, 0.7, 1.1, channel=REAR_SLIP),
        effect("CAMBER_LF", -1, -0.02, -0.03, -0.01),
    ]

    result = build(
        effects, [item("ARB_FRONT"), item("ARB_REAR"), item("WING"), item("CAMBER_LF", paired=True)]
    )

    assert result.changes == []
    assert result.prediction is None
    why = reasons(result)
    assert "reaches zero" in why[("ARB_FRONT", 1)]
    assert "Prior from Formula Alpha" in why[("ARB_REAR", -1)]
    assert "non-linear" in why[("WING", 1)]
    assert "guardrail rear_slip_peak_deg" in why[("WING", -1)]
    assert "pair" in why[("CAMBER_LF", -1)]
    assert [m.parameter for m in result.measured] == ["ARB_FRONT", "CAMBER_LF", "WING", "WING"]


def test_one_change_per_item_the_best_measured_click_amount() -> None:
    effects = [
        effect("ARB_FRONT", -1, -0.004, -0.007, -0.001),
        effect("ARB_FRONT", -2, -0.009, -0.013, -0.005),
        effect("ARB_REAR", 1, -0.006, -0.010, -0.002),
    ]

    result = build(effects)

    assert [(c.parameter, c.clicks) for c in result.changes] == [("ARB_FRONT", -2), ("ARB_REAR", 1)]
    assert "-2 clicks of this item measured better" in reasons(result)[("ARB_FRONT", -1)]
    assert result.prediction is not None
    assert result.prediction.estimate == pytest.approx(-0.015)


def test_a_change_that_leaves_the_range_or_the_setup_is_not_taken() -> None:
    effects = [
        effect("ARB_FRONT", -2, -0.01, -0.02, -0.005),
        effect("GONE", 1, -0.01, -0.02, -0.005),
    ]

    result = build(effects, [item("ARB_FRONT", value=1)])

    assert result.changes == []
    assert "outside 0 to 6" in reasons(result)[("ARB_FRONT", -2)]
    assert "Not a changeable item" in reasons(result)[("GONE", 1)]


def test_other_channels_and_corner_classes_do_not_choose_changes() -> None:
    effects = [
        effect("ARB_FRONT", -1, -0.02, -0.03, -0.01, corner_class="slow"),
        effect("ARB_FRONT", -1, -0.2, -0.3, -0.1, channel=REAR_SLIP),
    ]

    result = build(effects)

    assert result.changes == [] and result.left_out == [] and result.measured == []


def test_the_prediction_sums_estimates_and_variances() -> None:
    effects = [
        effect("ARB_FRONT", -1, -0.004, -0.008, 0.0 - 1e-9),
        effect("ARB_REAR", 1, -0.006, -0.010, -0.002),
    ]
    result = build(effects)
    z = quantile(LEVEL)
    se = [(c.ci_high - c.ci_low) / (2 * z) for c in result.changes]

    prediction = predict(result.changes, TIME, LEVEL)

    half = z * (se[0] ** 2 + se[1] ** 2) ** 0.5
    assert prediction.estimate == pytest.approx(-0.010)
    assert prediction.ci_low == pytest.approx(-0.010 - half)
    assert prediction.ci_high == pytest.approx(-0.010 + half)


better_rows = st.tuples(
    st.sampled_from(["ARB_FRONT", "ARB_REAR", "WING"]),
    st.sampled_from([-2, -1, 1, 2]),
    st.floats(-0.05, 0.05),
    st.floats(0.0005, 0.02),
    st.booleans(),
    st.booleans(),
)


@settings(max_examples=200, deadline=None)
@given(st.lists(better_rows, max_size=12, unique_by=lambda r: (r[0], r[1])))
def test_every_row_is_accounted_for_and_every_change_is_measured_faster(rows: list[Any]) -> None:
    effects = [
        effect(
            p,
            c,
            est,
            est - half,
            est + half,
            is_prior=prior,
            nonlinear=bent,
            prior_source="Other" if prior else "",
            experiment_ids=[] if prior else [1],
        )
        for p, c, est, half, prior, bent in rows
    ]

    result = build(effects)

    taken = [(c.parameter, c.clicks) for c in result.changes]
    left = [(entry.parameter, entry.clicks) for entry in result.left_out]
    assert sorted(taken + left) == sorted((r[0], r[1]) for r in rows)
    assert len({p for p, _ in taken}) == len(taken)
    by_key = {(r[0], r[1]): r for r in rows}
    for change in result.changes:
        _, _, _, _, prior, bent = by_key[(change.parameter, change.clicks)]
        assert change.ci_high < 0 and not prior and not bent
        assert 0 <= change.to_value <= 6
    if result.changes:
        assert result.prediction is not None
        assert result.prediction.ci_high < 0  # each interval is below zero, so is their sum's
        widest = max(c.ci_high - c.ci_low for c in result.changes)
        assert result.prediction.ci_high - result.prediction.ci_low >= widest - 1e-12
    else:
        assert result.prediction is None


def test_the_endpoint_answers_with_the_contract() -> None:
    body = {
        "effects": [effect("ARB_FRONT", -1, -0.0042, -0.0068, -0.0016)],
        "items": [item("ARB_FRONT")],
        "channel": TIME,
        "better": "lower",
        "rope": ROPE,
        "level": LEVEL,
        "guardrails": [],
    }

    reply = TestClient(create_app()).post("/base-setup", json=body)

    assert reply.status_code == 200
    answer = BaseSetup.model_validate(reply.json())
    assert [c.to_value for c in answer.changes] == [2]


def test_the_endpoint_leaves_the_prediction_out_when_nothing_qualifies() -> None:
    body = {
        "effects": [],
        "items": [],
        "channel": TIME,
        "better": "lower",
        "rope": ROPE,
        "level": LEVEL,
        "guardrails": [],
    }

    reply = TestClient(create_app()).post("/base-setup", json=body)

    assert reply.status_code == 200 and "prediction" not in reply.json()
