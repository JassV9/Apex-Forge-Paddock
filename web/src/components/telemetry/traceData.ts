import type { PassTrace } from "../../api/client";

export type XAxis = "distance" | "time";

/** A channel as the charts show it: core's samples in display units, nothing derived here. */
export interface Channel {
  key: string;
  title: string;
  unit: string;
  value: (trace: PassTrace, index: number) => number;
}

export const CHANNELS: Channel[] = [
  { key: "speed", title: "Speed", unit: "km/h", value: (t, i) => (t.columns.v[i] ?? 0) * 3.6 },
  { key: "gas", title: "Throttle", unit: "%", value: (t, i) => (t.columns.gas[i] ?? 0) * 100 },
  { key: "brake", title: "Brake", unit: "%", value: (t, i) => (t.columns.brake[i] ?? 0) * 100 },
  { key: "steer", title: "Steering", unit: "°", value: (t, i) => t.columns.steer[i] ?? 0 },
  {
    key: "front",
    title: "Front slip",
    unit: "°",
    value: (t, i) => t.axles.front_slip_deg[i] ?? 0,
  },
  {
    key: "rear",
    title: "Rear slip",
    unit: "°",
    value: (t, i) => t.axles.rear_slip_deg[i] ?? 0,
  },
];

/**
 * Distance from the start line, continued past it: the game restarts distance at the line,
 * so a pass through the last corner would jump back by a lap.
 */
export function continuousDistance(d: number[], trackLength: number): number[] {
  let offset = 0;
  return d.map((value, i) => {
    const previous = d[i - 1];
    if (previous !== undefined && value < previous - trackLength / 2) offset += trackLength;
    return value + offset;
  });
}

export function xOf(trace: PassTrace, axis: XAxis, trackLength: number): number[] {
  return axis === "time" ? trace.columns.t : continuousDistance(trace.columns.d, trackLength);
}

/**
 * The passes on one shared x scale, as uPlot needs: the sorted union of every pass's x, and
 * for each pass where each of its samples sits on it.
 */
export interface Frame {
  x: number[];
  positions: number[][];
  xs: number[][];
}

export function frameOf(traces: PassTrace[], axis: XAxis, trackLength: number): Frame {
  const xs = traces.map((trace) => xOf(trace, axis, trackLength));
  const x = [...new Set(xs.flat())].sort((a, b) => a - b);
  const at = new Map(x.map((value, index) => [value, index]));
  const positions = xs.map((values) => values.map((value) => at.get(value) ?? 0));
  return { x, positions, xs };
}

/** One channel of every pass on the frame; null where a pass has no sample. */
export function seriesOf(frame: Frame, traces: PassTrace[], channel: Channel): (number | null)[][] {
  return traces.map((trace, pass) => {
    const series = new Array<number | null>(frame.x.length).fill(null);
    (frame.positions[pass] ?? []).forEach((position, index) => {
      series[position] = channel.value(trace, index);
    });
    return series;
  });
}

/** The index of the sample of `xs` (ascending) closest to `x`. */
export function nearestIndex(xs: number[], x: number): number {
  let low = 0;
  let high = xs.length - 1;
  while (high - low > 1) {
    const middle = (low + high) >> 1;
    if ((xs[middle] ?? 0) < x) low = middle;
    else high = middle;
  }
  const below = Math.abs((xs[low] ?? 0) - x);
  const above = Math.abs((xs[high] ?? 0) - x);
  return above < below ? high : low;
}
