"""Explain: template text without a model, and the gate a model's reply must pass."""

import json
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import pytest
from diagnosis_data import car, cota_request, passes_of
from fastapi.testclient import TestClient

from paddock_engine.app import create_app
from paddock_engine.contracts.diagnose_request_schema import DiagnoseRequest
from paddock_engine.contracts.diagnosis_schema import Diagnosis, Provider
from paddock_engine.contracts.explain_reply_schema import ExplainReply, RankedReply
from paddock_engine.diagnosis.diagnose import diagnose
from paddock_engine.explain import config
from paddock_engine.explain.config import Choice, from_env
from paddock_engine.explain.explain import explain
from paddock_engine.explain.pack import EvidencePack
from paddock_engine.explain.provider import NullProvider, ProviderError
from paddock_engine.explain.remote import GeminiProvider, OpenRouterProvider
from paddock_engine.explain.validate import validate

SECRET = "test-key-not-a-real-one"


@pytest.fixture(scope="module")
def pack() -> EvidencePack:
    evidence, hypotheses = diagnose(cota_request(car()))
    return EvidencePack(evidence=evidence, hypotheses=hypotheses)


def reply(summary: str = "Understeer in slow corners.", **ranked: Any) -> ExplainReply:
    fields = {"hypothesis": "mid_understeer.slow", "why": "The front slips more.", "cites": []}
    return ExplainReply(summary=summary, ranking=[RankedReply(**{**fields, **ranked})])


class Scripted:
    """A provider that answers what the test tells it to."""

    name = Provider.gemini

    def __init__(self, answer: ExplainReply | ProviderError) -> None:
        self.answer = answer

    def complete(self, pack: EvidencePack) -> ExplainReply:
        if isinstance(self.answer, ProviderError):
            raise self.answer
        return self.answer


def test_the_template_text_passes_its_own_gate(pack: EvidencePack) -> None:
    template = NullProvider().complete(pack)

    assert validate(template, pack) == ""
    assert template.summary.startswith("Strongest reading: Understeer")
    assert [item.hypothesis for item in template.ranking] == [h.key for h in pack.hypotheses]
    slow = next(item for item in template.ranking if item.hypothesis == "mid_understeer.slow")
    assert "front saturation" in slow.why
    assert "front_slip_mid_deg.fast" in slow.cites


def test_no_symptom_is_said_in_words_too() -> None:
    passes = passes_of("slow", front_slip_mid_deg=4.0, rear_slip_mid_deg=4.0)
    evidence, hypotheses = diagnose(DiagnoseRequest(passes=passes, items=[]))

    shown = explain(EvidencePack(evidence, hypotheses), NullProvider())

    assert shown.provider is Provider.none
    assert shown.summary.startswith("No symptom is beyond its threshold")
    assert shown.ranking == []
    assert shown.rejected == ""


@pytest.mark.parametrize(
    ("made_up", "reason"),
    [
        (
            reply(cites=["brake_temp_front.slow"]),
            "it cites evidence that is not in the pack: brake_temp_front.slow",
        ),
        (
            reply(why="The brake_temp_front channel shows fade."),
            "it names a metric or item that is not in the pack: brake_temp_front",
        ),
        (
            reply(summary="Tyre wear, see tyre_wear_front.slow, is the cause."),
            "it names a metric or item that is not in the pack: tyre_wear_front.slow",
        ),
        (
            reply(why="Softening the bar will gain 0.35 seconds."),
            "it states a number that is not in the pack: 0.35",
        ),
        (
            reply(summary="The front slips 134 percent more than the rear."),
            "it states a number that is not in the pack: 134",
        ),
        (
            reply(hypothesis="aero_stall.fast"),
            "it ranks a hypothesis that is not in the pack: aero_stall.fast",
        ),
    ],
)
def test_acceptance_a_reply_that_invents_a_metric_or_a_number_is_rejected(
    pack: EvidencePack, made_up: ExplainReply, reason: str
) -> None:
    """Phase 4 acceptance: nothing a model makes up reaches the dashboard."""
    assert validate(made_up, pack) == reason

    shown = explain(pack, Scripted(made_up))

    assert shown.provider is Provider.none
    assert shown.rejected == f"The reply was rejected: {reason}"
    assert shown.summary == NullProvider().complete(pack).summary


def test_a_reply_that_stays_inside_the_pack_is_shown(pack: EvidencePack) -> None:
    honest = reply(
        summary="In slow corners the front slips 7.7 degrees and the rear 3.3, over 32 passes.",
        why="front_slip_mid_deg.slow is 7.67; in fast corners it is 3.7. Try ARB_FRONT by 1 click.",
        cites=["front_slip_mid_deg.slow", "front_slip_mid_deg.fast"],
    )

    shown = explain(pack, Scripted(honest))

    assert shown.provider is Provider.gemini
    assert shown.rejected == ""
    assert shown.summary == honest.summary
    assert shown.ranking[0].cites == ["front_slip_mid_deg.slow", "front_slip_mid_deg.fast"]


def test_a_number_is_checked_at_the_precision_it_is_written(pack: EvidencePack) -> None:
    assert validate(reply(why="Front slip is 7.667 degrees."), pack) == ""
    assert validate(reply(why="Front slip is 7.6 degrees."), pack).endswith(": 7.6")
    assert validate(reply(why="Corner T12 is the worst."), pack) == ""
    assert validate(reply(why="Between 3.3-2.9 at the rear."), pack) == ""


def test_ranking_a_hypothesis_twice_is_rejected(pack: EvidencePack) -> None:
    twice = reply()
    twice.ranking.append(twice.ranking[0])

    assert validate(twice, pack) == "it ranks a hypothesis twice: mid_understeer.slow"


def test_a_provider_that_fails_leaves_the_template_and_says_why(pack: EvidencePack) -> None:
    shown = explain(pack, Scripted(ProviderError("HTTP 429")))

    assert shown.provider is Provider.none
    assert shown.rejected == "gemini gave no reply: HTTP 429"
    assert shown.ranking != []


class Wire:
    """Stands in for the network: records the request and answers with a canned body."""

    def __init__(self, answer: object) -> None:
        self.answer = answer
        self.url = ""
        self.headers: Mapping[str, str] = {}
        self.body: Any = None

    def __call__(self, url: str, headers: Mapping[str, str], body: bytes) -> bytes:
        self.url, self.headers, self.body = url, headers, json.loads(body)
        return json.dumps(self.answer).encode("utf-8")


def test_gemini_is_sent_the_pack_and_its_answer_is_parsed(pack: EvidencePack) -> None:
    text = reply().model_dump_json()
    wire = Wire({"candidates": [{"content": {"parts": [{"text": text}]}}]})

    answer = GeminiProvider(SECRET, "some-model", wire).complete(pack)

    assert answer == reply()
    assert wire.url.endswith("/models/some-model:generateContent")
    assert SECRET not in wire.url
    assert wire.headers["x-goog-api-key"] == SECRET
    assert wire.body["generationConfig"]["responseMimeType"] == "application/json"
    sent = json.loads(wire.body["contents"][0]["parts"][0]["text"])
    assert [h["key"] for h in sent["hypotheses"]] == [h.key for h in pack.hypotheses]
    assert "score" not in sent["hypotheses"][0]
    assert SECRET not in json.dumps(wire.body)


def test_openrouter_is_sent_the_pack_and_its_answer_is_parsed(pack: EvidencePack) -> None:
    wire = Wire({"choices": [{"message": {"content": reply().model_dump_json()}}]})

    answer = OpenRouterProvider(SECRET, "", wire).complete(pack)

    assert answer == reply()
    assert wire.headers["Authorization"] == f"Bearer {SECRET}"
    assert wire.body["messages"][1]["content"] == pack.as_json()
    assert SECRET not in json.dumps(wire.body)


@pytest.mark.parametrize(
    "answer",
    [
        {"candidates": []},
        {"candidates": [{"content": {"parts": [{"text": "not json"}]}}]},
        {"candidates": [{"content": {"parts": [{"text": '{"summary": "x", "gain_s": 0.3}'}]}}]},
    ],
)
def test_an_answer_outside_the_reply_schema_is_a_provider_error(
    pack: EvidencePack, answer: object
) -> None:
    provider = GeminiProvider(SECRET, "", Wire(answer))

    with pytest.raises(ProviderError) as failure:
        provider.complete(pack)

    assert SECRET not in str(failure.value)
    assert explain(pack, provider).provider is Provider.none


@pytest.mark.parametrize(
    ("env", "provider", "note"),
    [
        ({}, Provider.none, ""),
        ({"GEMINI_API_KEY": SECRET}, Provider.gemini, ""),
        ({"PADDOCK_LLM_PROVIDER": "none", "GEMINI_API_KEY": SECRET}, Provider.none, ""),
        (
            {"PADDOCK_LLM_PROVIDER": "gemini"},
            Provider.none,
            "no key for gemini: set GEMINI_API_KEY",
        ),
        (
            {"PADDOCK_LLM_PROVIDER": "openrouter", "OPENROUTER_API_KEY": SECRET},
            Provider.openrouter,
            "",
        ),
        ({"PADDOCK_LLM_PROVIDER": "clippy"}, Provider.none, "unknown PADDOCK_LLM_PROVIDER: clippy"),
    ],
)
def test_the_environment_chooses_the_provider(
    monkeypatch: pytest.MonkeyPatch, env: dict[str, str], provider: Provider, note: str
) -> None:
    monkeypatch.setattr(config, "_from_keyring", lambda name: "")

    choice = from_env(env)

    assert choice.provider.name is provider
    assert choice.note == note
    assert SECRET not in choice.note


def test_a_key_in_the_os_keyring_is_used_when_the_environment_has_none(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        config, "_from_keyring", lambda name: SECRET if name == "GEMINI_API_KEY" else ""
    )

    assert from_env({}).provider.name is Provider.gemini


def test_diagnose_endpoint_runs_with_no_model_and_matches_the_contract() -> None:
    """Phase 4 acceptance: the system runs fully with NullProvider."""
    client = TestClient(create_app(Choice(NullProvider(), "")))
    schema_path = Path(__file__).resolve().parents[2] / "contracts" / "diagnosis.schema.json"

    response = client.post("/diagnose", json=cota_request(car()).model_dump(mode="json"))

    assert response.status_code == 200
    body = response.json()
    diagnosis = Diagnosis.model_validate(body)
    assert set(body) == set(json.loads(schema_path.read_text(encoding="utf-8"))["required"])
    assert diagnosis.explanation.provider is Provider.none
    assert diagnosis.explanation.rejected == ""
    assert diagnosis.hypotheses[0].candidates != []
    assert {cite for item in diagnosis.explanation.ranking for cite in item.cites} <= {
        e.id for e in diagnosis.evidence
    }


def test_a_requested_provider_without_a_key_is_reported_not_hidden() -> None:
    client = TestClient(create_app(Choice(NullProvider(), "no key for gemini: set GEMINI_API_KEY")))

    body = client.post("/diagnose", json={"passes": [], "items": []}).json()

    assert body["explanation"]["rejected"] == "no key for gemini: set GEMINI_API_KEY"
    assert body["hypotheses"] == []
