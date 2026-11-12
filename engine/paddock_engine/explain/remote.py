"""Hosted language models: Gemini (the default when a key is present) and OpenRouter.

Both get the same instructions and the same pack, and must answer with JSON that validates
against contracts/explain_reply.schema.json. What they answer is still checked against the
pack (explain/validate.py) before anyone sees it.

Only the standard library is used for HTTP: one POST does not justify a dependency.
"""

import json
import urllib.error
import urllib.request
from collections.abc import Callable, Mapping
from typing import Any

from pydantic import ValidationError

from paddock_engine.contracts.diagnosis_schema import Provider
from paddock_engine.contracts.explain_reply_schema import ExplainReply
from paddock_engine.explain.pack import EvidencePack
from paddock_engine.explain.provider import ProviderError

GEMINI_URL = "https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent"
GEMINI_MODEL = "gemini-2.5-flash"
OPENROUTER_URL = "https://openrouter.ai/api/v1/chat/completions"
OPENROUTER_MODEL = "google/gemini-2.5-flash"
TIMEOUT_S = 30.0
TEMPERATURE = 0.2

INSTRUCTIONS = """You explain a race car diagnosis to its driver. You are given an evidence \
pack as JSON: computed evidence (id, value, passes) and hypotheses (key, statement, \
alternatives, candidates), strongest first.

Answer with JSON only, in this shape:
{"summary": "<two or three plain sentences, at most 1200 characters>",
 "ranking": [{"hypothesis": "<key>", "why": "<at most 600 characters>", "cites": ["<id>"]}]}

Rules:
- Use only hypothesis keys and evidence ids that are in the pack.
- Every number you write must be a value from the pack, written as it is in the pack. Do not
  compute differences, ratios or percentages, and write no other digits.
- When an alternative is marked supported, say so before any setup change is discussed.
- Candidates are experiments to run, not fixes. Do not predict the size of any effect.
- Name no channel, metric or setup item that is not in the pack."""

# (url, headers, body) -> response body. Injected in tests; the real one is `post`.
Transport = Callable[[str, Mapping[str, str], bytes], bytes]


def post(url: str, headers: Mapping[str, str], body: bytes) -> bytes:
    request = urllib.request.Request(url, data=body, headers=dict(headers), method="POST")
    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT_S) as response:
            content: bytes = response.read()
            return content
    except urllib.error.HTTPError as error:
        raise ProviderError(f"HTTP {error.code}") from error
    except (urllib.error.URLError, TimeoutError) as error:
        raise ProviderError("no connection") from error


def _reply(text: object) -> ExplainReply:
    if not isinstance(text, str):
        raise ProviderError("the answer holds no text")
    try:
        return ExplainReply.model_validate_json(text)
    except ValidationError as error:
        raise ProviderError(
            f"the reply does not match the schema ({error.error_count()} errors)"
        ) from error


def _call(transport: Transport, url: str, headers: Mapping[str, str], body: Any) -> Any:
    sent = {"Content-Type": "application/json", **headers}
    answer = transport(url, sent, json.dumps(body).encode("utf-8"))
    try:
        return json.loads(answer)
    except ValueError as error:
        raise ProviderError("the answer is not JSON") from error


class GeminiProvider:
    name = Provider.gemini

    def __init__(self, api_key: str, model: str = "", transport: Transport = post) -> None:
        self._key = api_key
        self._model = model or GEMINI_MODEL
        self._transport = transport

    def complete(self, pack: EvidencePack) -> ExplainReply:
        body = {
            "systemInstruction": {"parts": [{"text": INSTRUCTIONS}]},
            "contents": [{"role": "user", "parts": [{"text": pack.as_json()}]}],
            "generationConfig": {
                "responseMimeType": "application/json",
                "temperature": TEMPERATURE,
            },
        }
        url = GEMINI_URL.format(model=self._model)
        answer = _call(self._transport, url, {"x-goog-api-key": self._key}, body)
        try:
            text = answer["candidates"][0]["content"]["parts"][0]["text"]
        except (KeyError, IndexError, TypeError) as error:
            raise ProviderError("the answer holds no text") from error
        return _reply(text)


class OpenRouterProvider:
    name = Provider.openrouter

    def __init__(self, api_key: str, model: str = "", transport: Transport = post) -> None:
        self._key = api_key
        self._model = model or OPENROUTER_MODEL
        self._transport = transport

    def complete(self, pack: EvidencePack) -> ExplainReply:
        body = {
            "model": self._model,
            "temperature": TEMPERATURE,
            "response_format": {"type": "json_object"},
            "messages": [
                {"role": "system", "content": INSTRUCTIONS},
                {"role": "user", "content": pack.as_json()},
            ],
        }
        headers = {"Authorization": f"Bearer {self._key}"}
        answer = _call(self._transport, OPENROUTER_URL, headers, body)
        try:
            text = answer["choices"][0]["message"]["content"]
        except (KeyError, IndexError, TypeError) as error:
            raise ProviderError("the answer holds no text") from error
        return _reply(text)
