"""Which provider writes explanations, from the environment.

PADDOCK_LLM_PROVIDER: none, gemini or openrouter. Unset means Gemini when a key for it is
there, and no model otherwise, so a fresh checkout runs without any key.
PADDOCK_LLM_MODEL: model id, when the provider's default is not wanted.
Keys: GEMINI_API_KEY and OPENROUTER_API_KEY, from the environment or, when the `keyring`
package is installed, from the OS keyring (service "paddock"). A key is never logged, never
written to disk and never part of an evidence pack.
"""

import importlib
import os
from collections.abc import Mapping
from dataclasses import dataclass

from paddock_engine.explain.provider import LLMProvider, NullProvider
from paddock_engine.explain.remote import GeminiProvider, OpenRouterProvider

KEYRING_SERVICE = "paddock"
KEY_NAMES = {"gemini": "GEMINI_API_KEY", "openrouter": "OPENROUTER_API_KEY"}


@dataclass(frozen=True)
class Choice:
    provider: LLMProvider
    note: str  # why a requested provider is not in use; empty otherwise


def _from_keyring(name: str) -> str:
    try:
        keyring = importlib.import_module("keyring")
        return str(keyring.get_password(KEYRING_SERVICE, name) or "")
    except Exception:  # no keyring, no backend, locked: all mean "no key here"
        return ""


def api_key(name: str, env: Mapping[str, str]) -> str:
    return env.get(name, "").strip() or _from_keyring(name)


def from_env(env: Mapping[str, str] | None = None) -> Choice:
    """The provider the environment asks for, or NullProvider with the reason it is not."""
    env = os.environ if env is None else env
    wanted = env.get("PADDOCK_LLM_PROVIDER", "").strip().lower()
    model = env.get("PADDOCK_LLM_MODEL", "").strip()
    if wanted in ("none", "null", "off"):
        return Choice(NullProvider(), "")
    if wanted not in ("", *KEY_NAMES):
        return Choice(NullProvider(), f"unknown PADDOCK_LLM_PROVIDER: {wanted}")
    name = wanted or "gemini"
    key = api_key(KEY_NAMES[name], env)
    if not key:
        note = f"no key for {name}: set {KEY_NAMES[name]}" if wanted else ""
        return Choice(NullProvider(), note)
    if name == "openrouter":
        return Choice(OpenRouterProvider(key, model), "")
    return Choice(GeminiProvider(key, model), "")
