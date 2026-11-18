import type { CornerReference, FeatureDifference, SessionReference } from "../../api/client";

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

/**
 * Where the AI's passes come from, when some are on a variant of the session's layout: the
 * same track on another track row (docs/adr/0027-layout-variants.md); "" when all are on the
 * session's own layout.
 */
export function aiSourceText(data: SessionReference): string {
  const variants = data.ai_tracks.filter((t) => t.variant && t.ai_passes > 0).map((t) => t.name);
  if (variants.length === 0) return "";
  const own = data.ai_tracks.some((t) => !t.variant && t.ai_passes > 0);
  return `AI on ${own ? "this layout and on " : ""}${variants.join(" and ")}, the same track`;
}

/** Why a session has no AI to be compared with, or "" when there are AI passes. */
export function noAiNote(data: SessionReference): string {
  if (data.ai_passes > 0) return "";
  if (data.variant_note !== "") return data.variant_note;
  if (data.layout === "") {
    return 'This track has a single layout. The AI drives only the "(Paddock test)" copy of a layout, and a track without a layout folder cannot have one, so there is no AI to compare with.';
  }
  if (!data.layout.endsWith(TEST_COPY_SUFFIX)) {
    return `You drove the original layout (${data.layout}). Laps on an original layout are compared here, in the web app, with the AI's laps on its "(Paddock test)" copy, the same track, and are never shown in the game. The AI has not driven this car on that copy yet: drive the copy with the AI from the game's Paddock window, then this table fills in.`;
  }
  return "The AI has not driven this car on this layout yet. In the game's Paddock window, calibrate the AI and let it drive laps; then this table fills in.";
}
