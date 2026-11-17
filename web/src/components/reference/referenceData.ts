import type { CornerReference, FeatureDifference } from "../../api/client";

/** Corners by time lost to the AI on the typical pass, largest first; no reference last. */
export function byTimeLost(corners: CornerReference[]): CornerReference[] {
  const lost = (corner: CornerReference) => corner.delta_median_s ?? -Infinity;
  return [...corners].sort((a, b) => lost(b) - lost(a) || a.corner - b.corner);
}

export const CLASS_LABEL: Record<CornerReference["class"], string> = {
  driver: "Practise",
  car: "Car / setup",
  no_reference: "No reference",
};

/** What each technique metric of the engine is, in words. */
const METRIC_LABEL: Record<string, string> = {
  brake_dist_m: "Brake point (m before apex)",
  brake_peak: "Peak brake",
  v_entry_kmh: "Entry speed (km/h)",
  trail_share: "Trail-braking share",
  coast_s: "Coasting (s)",
  v_min_kmh: "Minimum speed (km/h)",
  steer_corrections_per_s: "Steering corrections per s",
  throttle_pickup_m: "Throttle pickup (m from apex)",
  throttle_to_full_s: "To full throttle (s)",
};

export function metricLabel(metric: string): string {
  return METRIC_LABEL[metric] ?? metric;
}

function value(number: number): string {
  return Math.abs(number) >= 10 ? number.toFixed(0) : number.toFixed(2);
}

/** "Brake point (m before apex): 120 vs AI 100 (3 / 5 passes)". */
export function differenceText(difference: FeatureDifference): string {
  const counts = `${String(difference.human_n)} / ${String(difference.ai_n)} passes`;
  return `${metricLabel(difference.metric)}: ${value(difference.human_median)} vs AI ${value(
    difference.ai_median,
  )} (${counts})`;
}

/** Which AI passes a corner was compared with, when that is not simply the same setup. */
export function matchNote(corner: CornerReference): string {
  if (corner.match === "same") return "";
  if (corner.match === "none") return corner.reason;
  const items = corner.setup_changes ?? 0;
  return `AI on the nearest setup: ${String(items)} item${items === 1 ? "" : "s"} differ`;
}

/** The suffix of the layout copy the AI drives (tools/prepare_test_layout.py). */
const TEST_COPY_SUFFIX = "_paddock";

/** Why a session has no AI to be compared with, or "" when the AI has driven its layout. */
export function noAiNote(layout: string, aiPasses: number): string {
  if (aiPasses > 0) return "";
  if (!layout.endsWith(TEST_COPY_SUFFIX)) {
    const name = layout === "" ? "" : ` (${layout})`;
    return `You drove the original layout${name}. The AI drives only the "(Paddock test)" copy of a layout, so it has no passes here to be compared with. Drive the test copy: the Drive panel marks it "compared with the AI".`;
  }
  return "The AI has not driven this car on this layout yet. In the game's Paddock window, calibrate the AI and let it drive laps; then this table fills in.";
}
