"""Margins of practical equivalence per metric and setup item (margins.toml, ADR 0029).

Which margin a channel is judged against, in order:

1. the setup item's own, when `[parameters.<item>]` names the channel;
2. the one the experiment froze before driving (the success test's band, a guardrail's
   tolerance), when it is above 0;
3. the metric's default in `[metrics]`;
4. none: the channel gets no neutral or confirmed finding, never a band of 0.
"""

import tomllib
from dataclasses import dataclass, field
from functools import cache
from importlib.resources import files
from typing import Any


class MarginsError(ValueError):
    """The configuration names a margin that is not a positive number."""


@dataclass(frozen=True)
class Margins:
    metrics: dict[str, float]
    parameters: dict[str, dict[str, float]] = field(default_factory=dict)

    def margin(self, channel: str, parameter: str | None, frozen: float) -> float | None:
        """The margin `channel` is judged against; `frozen` is what the experiment carries."""
        own = self.parameters.get(parameter or "", {}).get(channel)
        if own is not None:
            return own
        if frozen > 0:
            return frozen
        return self.metrics.get(channel)


def _positive(table: dict[str, Any], where: str) -> dict[str, float]:
    out = {}
    for channel, value in table.items():
        if isinstance(value, bool) or not isinstance(value, int | float) or value <= 0:
            raise MarginsError(f"{where}.{channel} must be a positive number, not {value!r}")
        out[channel] = float(value)
    return out


def parse(text: str) -> Margins:
    """Margins from TOML text. Raises MarginsError on a margin that is not positive."""
    content = tomllib.loads(text)
    parameters = content.get("parameters", {})
    return Margins(
        metrics=_positive(content.get("metrics", {}), "metrics"),
        parameters={
            item: _positive(table, f"parameters.{item}") for item, table in parameters.items()
        },
    )


@cache
def margins() -> Margins:
    """The engine's configured margins (read once)."""
    return parse(files("paddock_engine.config").joinpath("margins.toml").read_text("utf-8"))
