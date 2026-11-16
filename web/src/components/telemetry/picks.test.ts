import { describe, expect, it, vi } from "vitest";

import { cotaPasses, traceOf } from "../../test-passes";
import { INK } from "../charts/chartTheme";
import { colorsOf, passLabel, pickCorner, pickPass } from "./picks";
import { cachedLoader } from "./useTraces";

const t1Passes = cotaPasses.passes.filter((pass) => pass.corner === 1);
const t1 = cotaPasses.corners[0];

describe("pickCorner", () => {
  it("opens the corner asked for, or the first with a valid pass", () => {
    expect(pickCorner(cotaPasses, 2)).toBe(2);
    expect(pickCorner(cotaPasses, 99)).toBe(1);
    expect(pickCorner({ ...cotaPasses, corners: [] }, 1)).toBeUndefined();
  });
});

describe("pickPass", () => {
  it("picks the pass asked for, else the newest valid one that is not the best", () => {
    expect(pickPass(t1Passes, t1, 5)).toBe(5);
    expect(pickPass(t1Passes, t1, undefined)).toBe(1);
    expect(pickPass(t1Passes, t1, 2)).toBe(1); // pass 2 is through another corner
  });
});

describe("colorsOf and passLabel", () => {
  it("draws the best purple and the picked cyan, as everywhere", () => {
    const colors = colorsOf([1, 3, 5], 3, 1);

    expect(colors.get(3)).toBe(INK.best);
    expect(colors.get(1)).toBe(INK.picked);
    expect(colors.get(5)).not.toBe(INK.best);
  });

  it("names a pass by lap and number", () => {
    const [first, , last] = t1Passes;
    if (first === undefined || last === undefined) throw new Error("fixture");

    expect(passLabel(first, 3)).toBe("L1 #1");
    expect(passLabel(last, 3)).toBe("L3 #5");
    expect(passLabel(t1Passes[1] ?? first, 3)).toBe("L2 #3 best");
  });
});

describe("cachedLoader", () => {
  it("asks core once per pass, and again after a failure", async () => {
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error("502"))
      .mockImplementation((seq: number) => Promise.resolve(traceOf(seq)));
    const load = cachedLoader(fetch);

    await expect(load(3)).rejects.toThrow("502");
    await load(3);
    await load(3);

    expect(fetch).toHaveBeenCalledTimes(2);
  });
});
