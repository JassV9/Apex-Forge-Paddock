import { describe, expect, it } from "vitest";

import type { CornerReference, FeatureDifference, SessionReference } from "../../api/client";
import { cotaReference } from "../../test-reference";
import {
  aiSourceText,
  byTimeLost,
  differenceText,
  matchNote,
  metricLabel,
  noAiNote,
} from "./referenceData";

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
    ["layout_f1", 0, "", "original layout (layout_f1)"],
    ["layout_f1", 0, "", "compared here, in the web app"],
    ["layout_f1", 0, "", "never shown in the game"],
    ["", 0, "", "single layout"],
    ["layout_f1_paddock", 0, "", "has not driven this car on this layout"],
    [
      "layout_f1",
      0,
      "The AI's laps on F1 (Paddock test) are not compared with: x.",
      "not compared with: x",
    ],
    ["layout_f1", 5, "", ""],
    ["layout_f1_paddock", 5, "", ""],
  ])(
    "explains no AI on layout %j with %j AI passes (note %j)",
    (layout, passes, variant, words) => {
      const data = { ...cotaReference, layout, ai_passes: passes, variant_note: variant };
      const note = noAiNote(data);
      if (words === "") expect(note).toBe("");
      else expect(note).toContain(words);
    },
  );

  it("names the variant the AI drove, the same track, and nothing on the own layout", () => {
    const original: SessionReference = {
      ...cotaReference,
      layout: "layout_f1_2026",
      ai_tracks: [
        { layout: "layout_f1_2026", name: "COTA F1 2026", variant: false, ai_passes: 0 },
        {
          layout: "layout_f1_2026_paddock",
          name: "COTA F1 2026 (Paddock test)",
          variant: true,
          ai_passes: 40,
        },
      ],
    };
    expect(aiSourceText(original)).toBe("AI on COTA F1 2026 (Paddock test), the same track");
    expect(aiSourceText(cotaReference)).toBe("");
    const both = {
      ...original,
      ai_tracks: original.ai_tracks.map((t) => ({ ...t, ai_passes: 3 })),
    };
    expect(aiSourceText(both)).toBe(
      "AI on this layout and on COTA F1 2026 (Paddock test), the same track",
    );
  });

  it("says when the AI drove another setup, and why there is no reference", () => {
    expect(matchNote(t1)).toBe("");
    expect(matchNote(t2)).toBe("AI on the nearest setup: 2 items differ");
    expect(matchNote(none)).toContain("3 are needed");
  });
});
