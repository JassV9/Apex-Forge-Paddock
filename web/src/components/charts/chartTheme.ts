import type uPlot from "uplot";

/** The chart colours of styles.css, for canvas drawing, which cannot read CSS variables. */
export const INK = {
  axis: "#7d8a99",
  grid: "rgba(125, 138, 153, 0.12)",
  tick: "rgba(125, 138, 153, 0.3)",
  /** The session's best pass of a corner: purple, as on a timing screen. */
  best: "#c77dff",
  /** The pass picked for a closer look. */
  picked: "#3ad0ff",
  /** AC's AI on the same car and setup, the reference driver (docs/adr/0019). */
  ai: "#2ee6a6",
};

/** Colours of the other passes overlaid, and of the four tyres FL, FR, RL, RR. */
export const PALETTE = ["#ffb547", "#7bd88f", "#ff6b8b", "#f2e36c", "#8ea8ff", "#e68f5c"];
export const TYRES = ["#3ad0ff", "#7bd88f", "#ffb547", "#ff6b8b"];

const FONT = '11px ui-sans-serif, system-ui, "Segoe UI", sans-serif';

/** An axis in the dashboard's style; `side` 1 puts it on the right. */
export function axis(label: string, extra: Partial<uPlot.Axis> = {}): uPlot.Axis {
  return {
    label,
    stroke: INK.axis,
    font: FONT,
    labelFont: FONT,
    labelSize: 16,
    size: 44,
    grid: { stroke: INK.grid, width: 1 },
    ticks: { stroke: INK.tick, width: 1, size: 4 },
    ...extra,
  };
}

/** Options every chart shares. Charts with the same `sync` key move one cursor together. */
export function baseOptions(sync?: string): Pick<uPlot.Options, "legend" | "cursor" | "padding"> {
  return {
    legend: { show: false },
    padding: [8, 8, 0, 0],
    cursor: {
      points: { size: 5 },
      drag: { x: false, y: false },
      ...(sync === undefined ? {} : { sync: { key: sync } }),
    },
  };
}
