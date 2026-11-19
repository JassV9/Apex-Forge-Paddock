"""Off-track passes as a guardrail (ADR 0029).

Core leaves off-track passes out of the comparison: their corner time is not a lap anyone would
keep. Dropped silently, a setup that makes the AI run wide would lose its worst passes and look
better for it. So core counts them per run block and the engine compares the share of each
block's push-lap passes that went off, variant minus baseline, with the same block-level t as
every effect (degrees of freedom: blocks - 2). Worse is higher; the tolerance is the
experiment's own guardrail on `offtrack_rate`, or the configured margin.
"""

from collections.abc import Sequence

import numpy as np

from paddock_engine.contracts.evaluate_request_schema import Arm, OfftrackBlock
from paddock_engine.stats.block_level import arm_error
from paddock_engine.stats.effects import t_quantile
from paddock_engine.stats.model import NotEstimableError

OFFTRACK = "offtrack_rate"


def offtrack_effect(
    blocks: Sequence[OfftrackBlock], level: float
) -> tuple[float, float, float, int] | None:
    """Estimate, interval and passes of the variant's change in off-track share; None when the
    blocks cannot tell (an arm missing, or fewer than 3 blocks with passes)."""
    driven = [block for block in blocks if block.passes > 0]
    if not driven:
        return None
    rates = np.array([block.offtrack / block.passes for block in driven])
    on_variant = np.array([block.arm is Arm.variant for block in driven], dtype=np.float64)
    try:
        estimate, error, degrees = arm_error(
            rates, np.column_stack([np.ones(len(driven)), on_variant])
        )
    except NotEstimableError:
        return None
    half_width = t_quantile(level, degrees) * error
    passes = sum(block.passes for block in driven)
    return estimate, estimate - half_width, estimate + half_width, passes
