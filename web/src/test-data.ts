import type { Experiment } from "./api/client";

/** An experiment that is being driven: a softer front anti-roll bar, two of four blocks done. */
export const running: Experiment = {
  id: 4,
  kind: "ofat",
  state: "running",
  statement: "A softer front bar gives the front more grip.",
  car_name: "Mazda MX-5 Cup",
  track_name: "Test Ring",
  parameter: "ARB_FRONT",
  parameter_label: "Front ARB",
  clicks: -1,
  success_test: { channel: "corner_time_s", rope: 0.005, level: 0.9, better: "lower" },
  guardrails: [{ channel: "rear_slip_peak_deg", tolerance: 0.5, worse: "higher" }],
  predictions: [{ channel: "corner_time_s", direction: "decrease", low: 0.01, high: 0.3 }],
  arms: [
    { label: "Setup K", is_baseline: true, setup_version_id: 1, value: 3 },
    { label: "Setup R", is_baseline: false, setup_version_id: 2, value: 2 },
  ],
  plan: {
    sigma: 0.028,
    sigma_source: "assumed",
    passes_per_arm: 6,
    mde_detect: 0.02,
    mde_decide: 0.03,
  },
  blocks: [
    { order: 1, label: "Setup K", laps_target: 3, laps_done: 3, passes: 28 },
    { order: 2, label: "Setup R", laps_target: 3, laps_done: 3, passes: 28 },
    { order: 3, label: "Setup K", laps_target: 3, laps_done: 1, passes: 14 },
    { order: 4, label: "Setup R", laps_target: 3, laps_done: 0, passes: 0 },
  ],
  results: [],
  events: [
    {
      from: "",
      to: "proposed",
      reason: "A softer front bar gives the front more grip.",
      at: "2026-11-12T10:00:00Z",
    },
    { from: "proposed", to: "planned", reason: "4 blocks of 3 laps.", at: "2026-11-12T10:01:00Z" },
    {
      from: "planned",
      to: "running",
      reason: "Run plan and setups written for the agent.",
      at: "2026-11-12T10:02:00Z",
    },
  ],
  extensions: 0,
  created_at: "2026-11-12T10:00:00Z",
};

/** The same experiment after the engine's verdict: faster, but the rear slides more. */
export const tradeoff: Experiment = {
  ...running,
  state: "tradeoff",
  results: [
    {
      channel: "corner_time_s",
      role: "primary",
      corner_class: "all",
      estimate: -0.045,
      ci_low: -0.06,
      ci_high: -0.03,
      n_passes: 84,
      effect_class: "negative",
    },
    {
      channel: "rear_slip_peak_deg",
      role: "guardrail",
      corner_class: "all",
      estimate: 1.49,
      ci_low: 1.47,
      ci_high: 1.51,
      n_passes: 84,
      effect_class: "positive",
    },
  ],
  verdict: {
    decision: "tradeoff",
    reason: "Faster on corner_time_s, but rear_slip_peak_deg got worse.",
    decided_by: "engine",
    decided_at: "2026-11-12T10:30:00Z",
    prediction_hits: 1,
    prediction_total: 1,
  },
  events: [
    ...running.events,
    {
      from: "running",
      to: "evaluating",
      reason: "84 passes judged, 0 left out.",
      at: "2026-11-12T10:29:59Z",
    },
    {
      from: "evaluating",
      to: "tradeoff",
      reason: "Faster on corner_time_s, but rear_slip_peak_deg got worse.",
      at: "2026-11-12T10:30:00Z",
    },
  ],
};
