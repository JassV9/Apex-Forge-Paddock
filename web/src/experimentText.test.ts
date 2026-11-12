import { describe, expect, it } from "vitest";

import type { Experiment } from "./api/client";
import { availableSteps, changeText, effectText, isBlind, type State } from "./experimentText";
import { running, tradeoff } from "./test-data";

function inState(state: State, change: Partial<Experiment> = {}): Experiment {
  return { ...running, ...change, state };
}

describe("availableSteps", () => {
  const cases: [State, string[]][] = [
    ["proposed", ["Plan", "Discard"]],
    ["planned", ["Start", "Discard"]],
    ["running", ["Evaluate", "Abort"]],
    ["evaluating", []],
    ["tradeoff", ["Keep the variant", "Back to the baseline"]],
    ["inconclusive", ["Extend", "Back to the baseline"]],
    ["accepted", ["Roll back"]],
    ["rejected", []],
    ["reverted", []],
    ["aborted", []],
    ["discarded", []],
  ];

  it.each(cases)("offers what core allows from %s", (state, labels) => {
    expect(availableSteps(inState(state)).map((step) => step.label)).toEqual(labels);
  });

  it("does not offer a third extension", () => {
    const steps = availableSteps(inState("inconclusive", { extensions: 2 }));

    expect(steps.map((step) => step.label)).toEqual(["Back to the baseline"]);
  });

  it("sends a trade-off to the decision route and everything else to an action", () => {
    expect(availableSteps(tradeoff).map((step) => step.request)).toEqual([
      { accept: true },
      { accept: false },
    ]);
    expect(availableSteps(running)[0]?.request).toEqual({ action: "evaluate" });
  });
});

describe("experiment text", () => {
  it("names the change by the item's label and its clicks", () => {
    expect(changeText(running)).toBe("Front ARB −1 click");
    expect(changeText({ ...running, clicks: 2, parameter_label: "" })).toBe("ARB_FRONT +2 clicks");
    expect(changeText({ ...running, kind: "recheck" })).toMatch(/^Re-check/);
  });

  it("writes an effect with its interval and signs", () => {
    const [time, slip] = tradeoff.results;
    expect(time && effectText(time)).toBe("−0.045 [−0.060, −0.030]");
    expect(slip && effectText(slip)).toBe("+1.490 [+1.470, +1.510]");
  });

  it("keeps the setups blind until there is a verdict", () => {
    expect(isBlind(running)).toBe(true);
    expect(isBlind(inState("evaluating"))).toBe(true);
    expect(isBlind(tradeoff)).toBe(false);
    expect(isBlind(inState("reverted"))).toBe(false);
  });
});
