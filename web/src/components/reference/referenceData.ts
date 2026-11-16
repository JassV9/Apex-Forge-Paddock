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
