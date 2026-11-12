"""The rule table: which knobs are worth testing for a symptom.

Ported from Apex Forge v0 `core/advisor.lua` (docs/audit-v0.md: "refactor into a hypothesis
generator that emits ranked candidates, not answers"). In v0 the first knob of a row was
applied. Here every row is a list of experiments to run; each `why` is a rule of thumb, shown
as a prior and never as a result.

sign: +1 means more of what the knob does (stiffer, more wing, more front brake, more
locking); -1 means less. For camber, -1 is more negative.
"""

from dataclasses import dataclass


@dataclass(frozen=True)
class Knob:
    role: str
    sign: int
    why: str
    axle: str = ""  # F or R; empty when the role has no axle
    unless: str = ""  # symptom that rules this knob out


RULES: dict[str, tuple[Knob, ...]] = {
    "entry_us": (
        Knob("bias", -1, "brake bias rearward helps the car rotate on entry", unless="lock_r"),
        Knob("arb", -1, "a softer front bar gives the front more grip on turn-in", "F"),
        Knob("diffcoast", -1, "less coast locking frees rotation off the brakes"),
        Knob("preload", -1, "less preload frees rotation on entry"),
        Knob("arb", 1, "a stiffer rear bar adds rotation", "R"),
    ),
    "entry_os": (
        Knob("bias", 1, "brake bias forward settles the rear on entry", unless="lock_f"),
        Knob("diffcoast", 1, "more coast locking stabilises the rear off the brakes"),
        Knob("preload", 1, "more preload stabilises entry"),
        Knob("arb", -1, "a softer rear bar gives the rear more grip", "R"),
    ),
    "mid_us_low": (
        Knob("arb", -1, "a softer front bar gives more front grip mid-corner", "F"),
        Knob("arb", 1, "a stiffer rear bar moves the balance rearward", "R"),
        Knob("camber", -1, "more negative front camber adds front grip mid-corner", "F"),
        Knob("spring", -1, "softer front springs add front mechanical grip", "F"),
    ),
    "mid_us_high": (
        Knob("wing", 1, "more front wing moves the aero balance forward", "F"),
        Knob("wing", -1, "less rear wing moves the aero balance forward", "R"),
        Knob("arb", -1, "a softer front bar gives more front grip", "F"),
    ),
    "mid_os_low": (
        Knob("arb", -1, "a softer rear bar gives more rear grip mid-corner", "R"),
        Knob("arb", 1, "a stiffer front bar moves the balance forward", "F"),
        Knob("spring", -1, "softer rear springs add rear mechanical grip", "R"),
    ),
    "mid_os_high": (
        Knob("wing", 1, "more rear wing moves the aero balance rearward", "R"),
        Knob("wing", -1, "less front wing moves the aero balance rearward", "F"),
        Knob("arb", -1, "a softer rear bar gives more rear grip", "R"),
    ),
    # Diff power locking is left out, as in v0: sources disagree on which way it goes.
    "exit_us": (
        Knob("arb", 1, "a stiffer rear bar helps rotation on throttle", "R"),
        Knob("arb", -1, "a softer front bar helps the front hold the line on exit", "F"),
    ),
    "exit_os": (
        Knob("arb", -1, "a softer rear bar gives the rear more traction", "R"),
        Knob("spring", -1, "softer rear springs help traction", "R"),
        Knob("wing", 1, "more rear wing helps high-speed exits", "R"),
    ),
    "lock_f": (
        Knob("bias", -1, "the front tyres lock first: move brake bias rearward"),
        Knob("brakepower", -1, "less brake force makes the fronts harder to lock"),
    ),
    "lock_r": (
        Knob("bias", 1, "the rear tyres lock first: move brake bias forward"),
        Knob("diffcoast", 1, "more coast locking steadies the rears under braking"),
    ),
    "spin": (
        Knob("arb", -1, "a softer rear bar puts power down better", "R"),
        Knob("spring", -1, "softer rear springs put power down better", "R"),
    ),
}

# Roles whose raw value has a known direction: a higher value is more of the role's effect,
# unless the car stores the value inverted (negative shown_scale). From v0 `catalog.role`.
DIRECTED_ROLES = frozenset(
    {
        "arb",
        "bias",
        "brakepower",
        "diffcoast",
        "preload",
        "camber",
        "pressure",
        "heave",
        "spring",
        "wing",
    }
)

# v0 rows that need channels Paddock does not compute yet: hot tyre pressures against the
# tyre's ideal, bottoming, and straight-line speed. They come back with those channels.
