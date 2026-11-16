import { describe, expect, it } from "vitest";

import { traceOf } from "../../test-passes";
import { CHANNELS, continuousDistance, frameOf, nearestIndex, seriesOf, xOf } from "./traceData";

const speed = CHANNELS[0];
if (speed === undefined) throw new Error("no channels");

describe("continuousDistance", () => {
  it("carries on past the start line instead of jumping back a lap", () => {
    expect(continuousDistance([5400, 5420, 10, 30], 5428.8)).toEqual([5400, 5420, 5438.8, 5458.8]);
  });

  it("leaves a pass that does not cross the line alone", () => {
    expect(continuousDistance([600, 605, 604.9, 610], 5428.8)).toEqual([600, 605, 604.9, 610]);
  });
});

describe("frameOf and seriesOf", () => {
  it("puts every pass on one sorted x scale, null where a pass has no sample", () => {
    const traces = [traceOf(1, 600), traceOf(2, 602)];

    const frame = frameOf(traces, "distance", 5428.8);
    const [first, second] = seriesOf(frame, traces, speed);

    expect(frame.x).toEqual([600, 602, 605, 607, 610, 612, 615, 617]);
    expect(frame.positions).toEqual([
      [0, 2, 4, 6],
      [1, 3, 5, 7],
    ]);
    expect(first?.[0]).toBeCloseTo(288);
    expect(first?.[1]).toBeNull();
    expect(second?.[1]).toBeCloseTo(288);
  });

  it("aligns passes on time since zone entry when asked", () => {
    const traces = [traceOf(1, 600), traceOf(2, 900)];

    expect(frameOf(traces, "time", 5428.8).x).toEqual([0, 0.1, 0.2, 0.3]);
    expect(xOf(traces[1] ?? traceOf(2), "time", 5428.8)).toEqual([0, 0.1, 0.2, 0.3]);
  });

  it("shows core's channels in display units, without computing new ones", () => {
    const trace = traceOf(1);
    const values = CHANNELS.map((channel) => channel.value(trace, 1));

    expect(values).toEqual([252, 0, 100, -10, 1, 1]);
  });
});

describe("nearestIndex", () => {
  it.each([
    [599, 0],
    [602, 0],
    [603, 1],
    [611, 2],
    [700, 3],
  ])("finds the sample nearest %d", (x, index) => {
    expect(nearestIndex([600, 605, 610, 615], x)).toBe(index);
  });
});
