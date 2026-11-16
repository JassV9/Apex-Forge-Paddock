import type { CornerSummary, PassRecord, SessionPasses } from "../../api/client";
import { lapLabel } from "../../format";
import { INK, PALETTE } from "../charts/chartTheme";

/** The corner to open on: the asked one if it has passes, else the first with a valid pass. */
export function pickCorner(data: SessionPasses, asked: number | undefined): number | undefined {
  if (data.corners.some((corner) => corner.corner === asked)) return asked;
  return (data.corners.find((corner) => corner.best_seq !== undefined) ?? data.corners[0])?.corner;
}

/** The pass to look at: the asked one, else the newest valid one that is not the best. */
export function pickPass(
  passes: PassRecord[],
  summary: CornerSummary | undefined,
  asked: number | undefined,
): number | undefined {
  if (passes.some((pass) => pass.seq === asked)) return asked;
  const others = passes.filter((pass) => pass.valid && pass.seq !== summary?.best_seq);
  return (others.at(-1) ?? passes.at(-1))?.seq;
}

/** The best pass is purple and the picked one cyan, as everywhere; the rest share a palette. */
export function colorsOf(seqs: number[], best: number | undefined, picked: number | undefined) {
  const colors = new Map<number, string>();
  let next = 0;
  for (const seq of seqs) {
    if (seq === best) colors.set(seq, INK.best);
    else if (seq === picked) colors.set(seq, INK.picked);
    else colors.set(seq, PALETTE[next++ % PALETTE.length] ?? INK.axis);
  }
  return colors;
}

/** How a pass is named in the legend: its lap, its number, and whether it is the best. */
export function passLabel(pass: PassRecord, best: number | undefined): string {
  const name = `${lapLabel(pass.lap)} #${String(pass.seq)}`;
  return pass.seq === best ? `${name} best` : name;
}
