import { describe, expect, it } from "vitest";

import type { CornerReference, FeatureDifference } from "../../api/client";
import { cotaReference } from "../../test-reference";
import { byTimeLost, differenceText, matchNote, metricLabel, noAiNote } from "./referenceData";

const [t2, t1] = cotaReference.corners as [CornerReference, CornerReference];
const none: CornerReference = {
  ...t2,
  corner: 3,
  match: "none",
  class: "no_reference",
  reason: "1 AI passes of this corner on this setup; 3 are needed",
  delta_median_s: undefined,
};

describe("referenceData", () => {
  it("sorts by time lost on the typical pass, corners without a reference last", () => {
    expect(byTimeLost([none, t2, t1]).map((c) => c.name)).toEqual(["T1", "T2", "T2"]);
    expect(byTimeLost([none, t2, t1]).at(-1)?.corner).toBe(3);
  });

  it("writes a difference with both medians and the passes behind them", () => {
    const [brake] = t1.differences as [FeatureDifference];
    expect(differenceText(brake)).toBe("Brake point (m before apex): 120 vs AI 100 (2 / 9 passes)");
    expect(metricLabel("something_new")).toBe("something_new");
  });

  it.each([
    ["layout_f1", 0, "original layout (layout_f1)"],
    ["", 0, "original layout."],
    ["layout_f1_paddock", 0, "has not driven this car on this layout"],
    ["layout_f1", 5, ""],
    ["layout_f1_paddock", 5, ""],
  ])("explains no AI to compare with on layout %j with %j AI passes", (layout, passes, words) => {
    const note = noAiNote(layout, passes);
    if (words === "") expect(note).toBe("");
    else expect(note).toContain(words);
  });

  it("says when the AI drove another setup, and why there is no reference", () => {
    expect(matchNote(t1)).toBe("");
    expect(matchNote(t2)).toBe("AI on the nearest setup: 2 items differ");
    expect(matchNote(none)).toContain("3 are needed");
  });
});
