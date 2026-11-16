import type { CornerSummary, PassRecord } from "../../api/client";

/** Laps down, corners across: the first pass of each corner in each lap. */
export interface LapRow {
  lap: number | undefined;
  byCorner: Map<number, PassRecord>;
}

export function lapGrid(passes: PassRecord[]): LapRow[] {
  const rows = new Map<number | undefined, LapRow>();
  for (const pass of passes) {
    const row = rows.get(pass.lap) ?? { lap: pass.lap, byCorner: new Map() };
    if (!row.byCorner.has(pass.corner)) row.byCorner.set(pass.corner, pass);
    rows.set(pass.lap, row);
  }
  return [...rows.values()].sort((a, b) => (a.lap ?? Infinity) - (b.lap ?? Infinity));
}

export type Tone = "best" | "near" | "off" | "far" | "invalid";

/** Shares of the corner's best time that the colours stand for (legend in OverviewPanel). */
export const NEAR = 0.01;
export const OFF = 0.03;

/** A cell's colour on the timing screen: purple for the best, then by how far off it is. */
export function toneOf(pass: PassRecord, summary: CornerSummary | undefined): Tone {
  if (!pass.valid) return "invalid";
  if (summary?.best_s === undefined) return "off";
  if (pass.seq === summary.best_seq) return "best";
  const share = (pass.duration_s - summary.best_s) / summary.best_s;
  if (share <= NEAR) return "near";
  return share <= OFF ? "off" : "far";
}

/** First and last value of what was recorded, in the order driven. */
export interface Span {
  first: number;
  last: number;
}

export interface SessionFacts {
  passes: number;
  valid: number;
  laps: number;
  fuel: Span | undefined;
  road: Span | undefined;
  air: Span | undefined;
  grip: Span | undefined;
}

function spanOf(values: number[]): Span | undefined {
  const first = values[0];
  const last = values.at(-1);
  return first === undefined || last === undefined ? undefined : { first, last };
}

/** What the session's conditions were at its first and last recorded pass. */
export function sessionFacts(passes: PassRecord[]): SessionFacts {
  const recorded = passes.flatMap((pass) =>
    pass.conditions === undefined ? [] : [pass.conditions],
  );
  const laps = new Set(passes.flatMap((pass) => (pass.lap === undefined ? [] : [pass.lap])));
  return {
    passes: passes.length,
    valid: passes.filter((pass) => pass.valid).length,
    laps: laps.size,
    fuel: spanOf(recorded.map((c) => c.fuel_l)),
    road: spanOf(recorded.map((c) => c.road_c)),
    air: spanOf(recorded.map((c) => c.air_c)),
    grip: spanOf(recorded.map((c) => c.grip)),
  };
}
