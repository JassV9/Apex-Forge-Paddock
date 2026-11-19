import { describe, expect, it } from "vitest";

import type { CurveResult } from "../../api/client";
import { amount, context, curveSentence, observableName } from "./curveText";

function curve(over: Partial<CurveResult>): CurveResult {
  return {
    metric: "rear_slip_deg",
    phase: "mid",
    regime: "slow",
    source: "response",
    unit: "deg",
    shape: "linear",
    clicks: [-1, 1, 2],
    slope: { estimate: 0.104, ci_low: 0.08, ci_high: 0.12 },
    curvature: { estimate: 0.002, ci_low: -0.01, ci_high: 0.014 },
    reason: "The slope is clear of zero; the quadratic term is not.",
    ...over,
  };
}

describe("curveSentence", () => {
  it("says a linear response with its size per click", () => {
    expect(curveSentence(curve({}))).toBe(
      "rear slip rises with each click, about 0.1 deg per click, linear",
    );
  });

  it("shows time in milliseconds and says which way it goes", () => {
    const time = curve({
      metric: "corner_time_s",
      unit: "s",
      slope: { estimate: -0.0021, ci_low: -0.003, ci_high: -0.001 },
    });
    expect(curveSentence(time)).toBe(
      "corner time falls with each click, about 2.1 ms per click, linear",
    );
  });

  it("names the other shapes plainly", () => {
    expect(curveSentence(curve({ shape: "nonlinear" }))).toContain("curved");
    expect(curveSentence(curve({ shape: "negligible", margin: 0.05 }))).toBe(
      "rear slip barely moves: every step within ±0.05 deg",
    );
    expect(
      curveSentence(
        curve({
          shape: "direction_change",
          turn_clicks: 1,
          curvature: { estimate: -0.1, ci_low: -0.2, ci_high: -0.05 },
        }),
      ),
    ).toBe("rear slip rises, then falls: it turns at +1 clicks");
    expect(
      curveSentence(curve({ shape: "insufficient", reason: "One step is a point, not a curve." })),
    ).toBe("rear slip: not enough to say (one step is a point, not a curve)");
  });
});

describe("names", () => {
  it("reads an unknown metric from its id and leaves the whole corner unnamed", () => {
    expect(observableName("ride_height_rear_mm")).toBe("ride height rear");
    expect(context({ phase: "corner", regime: "all" })).toBe("all corners");
    expect(context({ phase: "exit", regime: "fast" })).toBe("exit, fast corners");
    expect(amount(0.5, "")).toBe("0.5");
  });
});
