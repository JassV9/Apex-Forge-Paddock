import { describe, expect, it } from "vitest";

import { cotaPasses } from "../../test-passes";
import { lapGrid, sessionFacts, toneOf } from "./overviewData";

const [t1] = cotaPasses.corners;

describe("lapGrid", () => {
  it("puts each lap on a row and each corner's pass in its column", () => {
    const rows = lapGrid(cotaPasses.passes);

    expect(rows.map((row) => row.lap)).toEqual([0, 1, 2]);
    expect(rows[1]?.byCorner.get(1)?.seq).toBe(3);
    expect(rows[1]?.byCorner.get(2)?.seq).toBe(4);
    expect(rows[2]?.byCorner.has(2)).toBe(false);
  });

  it("keeps the first pass when a corner was passed twice in a lap, and puts unknown laps last", () => {
    const [first, , third] = cotaPasses.passes;
    if (first === undefined || third === undefined) throw new Error("fixture");
    const unknown = { ...third, lap: undefined };

    const rows = lapGrid([unknown, first, { ...first, seq: 9 }]);

    expect(rows.map((row) => row.lap)).toEqual([0, undefined]);
    expect(rows[0]?.byCorner.get(1)?.seq).toBe(1);
  });
});

describe("toneOf", () => {
  const base = cotaPasses.passes[0];
  if (base === undefined) throw new Error("fixture");

  it.each([
    ["the best pass", { ...base, seq: 3, duration_s: 9.164 }, "best"],
    ["within 1 % of the best", { ...base, duration_s: 9.25 }, "near"],
    ["within 3 %", { ...base, duration_s: 9.4 }, "off"],
    ["further off", { ...base, duration_s: 9.6 }, "far"],
    ["not counted, however quick", { ...base, valid: false, duration_s: 9 }, "invalid"],
  ] as const)("colours %s", (_, pass, tone) => {
    expect(toneOf(pass, t1)).toBe(tone);
  });

  it("does not grade a corner without a valid time", () => {
    const untimed = { corner: 2, name: "T2", passes: 1, valid_passes: 0 };

    expect(toneOf(base, untimed)).toBe("off");
  });
});

describe("sessionFacts", () => {
  it("counts passes and laps and spans the conditions from first to last pass", () => {
    const facts = sessionFacts(cotaPasses.passes);

    expect(facts).toMatchObject({ passes: 5, valid: 4, laps: 3 });
    expect(facts.fuel).toEqual({ first: 49, last: 45 });
    expect(facts.road).toEqual({ first: 36, last: 36 });
  });

  it("says nothing about conditions that were not recorded", () => {
    const bare = cotaPasses.passes.map((pass) => ({ ...pass, conditions: undefined }));

    expect(sessionFacts(bare).fuel).toBeUndefined();
  });
});
