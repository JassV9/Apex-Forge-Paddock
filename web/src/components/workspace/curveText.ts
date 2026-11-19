import type { CurveResult } from "../../api/client";

/** Plain names of the observables a sweep classifies; anything else is its id in words. */
const NAMES: Record<string, string> = {
  corner_time_s: "corner time",
  phase_time_s: "phase time",
  rear_slip_deg: "rear slip",
  front_slip_deg: "front slip",
  slip_balance_deg: "slip balance (front minus rear)",
  rear_slip_peak_deg: "peak rear slip",
  countersteer_s: "countersteer time",
  yaw_gain: "yaw gain",
  steer_per_g: "steering per g",
  rear_slip_ratio_traction: "rear wheelspin on the throttle",
  front_slip_ratio_braking: "front slip under braking",
  rear_slip_ratio_braking: "rear slip under braking",
  steer_reversals_per_s: "steering corrections",
  v_min_kmh: "minimum speed",
  v_max_kmh: "top speed",
  brake_point_m: "braking point",
  brake_dist_m: "braking distance",
  throttle_pickup_m: "throttle pick-up",
};

const REGIMES: Record<string, string> = {
  all: "all corners",
  slow: "slow corners",
  medium: "medium corners",
  fast: "fast corners",
};

export function observableName(metric: string): string {
  return NAMES[metric] ?? metric.replace(/_(s|deg|m|kmh|mm|g|per_s)$/, "").replaceAll("_", " ");
}

/** Where the observable was measured: "mid, slow corners"; the whole corner is not named. */
export function context(curve: Pick<CurveResult, "phase" | "regime">): string {
  const regime = REGIMES[curve.regime] ?? curve.regime;
  return curve.phase === "corner" ? regime : `${curve.phase}, ${regime}`;
}

/** A size in the unit the driver reads: seconds as milliseconds, two significant digits. */
export function amount(value: number, unit: string): string {
  const [scaled, shown] = unit === "s" ? [value * 1000, "ms"] : [value, unit];
  const digits = Number(Math.abs(scaled).toPrecision(2)).toString();
  return shown === "" ? digits : `${digits} ${shown}`;
}

/** What a curve says in one plain sentence, e.g. "rear slip rises with each click, about 0.1 deg
 * per click, linear". */
export function curveSentence(curve: CurveResult): string {
  const name = observableName(curve.metric);
  const slope = curve.slope?.estimate ?? 0;
  const way = slope >= 0 ? "rises" : "falls";
  switch (curve.shape) {
    case "linear":
      return `${name} ${way} with each click, about ${amount(slope, curve.unit)} per click, linear`;
    case "nonlinear":
      return `${name} ${way}, more at some steps than others: curved, not a straight line`;
    case "negligible":
      return `${name} barely moves: every step within ±${amount(curve.margin ?? 0, curve.unit)}`;
    case "direction_change": {
      const shape =
        (curve.curvature?.estimate ?? 0) < 0 ? "rises, then falls" : "falls, then rises";
      const at = curve.turn_clicks ?? 0;
      return `${name} ${shape}: it turns at ${at > 0 ? "+" : ""}${String(at)} clicks`;
    }
    case "insufficient":
      return `${name}: not enough to say (${curve.reason.replace(/\.$/, "").toLowerCase()})`;
  }
}
