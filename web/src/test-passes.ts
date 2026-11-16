import type { PassRecord, PassTrace, SessionPasses, SessionSummary } from "./api/client";

/** A recorded session as the rail lists it. */
export const cotaSession: SessionSummary = {
  id: 2,
  spool_id: "20261112T201842-rss_formula_hybrid_2026",
  car_name: "Formula Hybrid 2026",
  track_name: "COTA F1 2026",
  layout: "layout_f1_2026",
  started_at: "2026-11-13T01:18:42Z",
  source: "human",
  passes: 5,
  corners: 2,
};

const conditions = (fuel: number, core: number) => ({
  fuel_l: fuel,
  tyre_core_c: [core, core + 1, core + 4, core + 5],
  tyre_surface_c: [80, 80, 82, 82],
  tyre_press_psi: [24, 24, 22, 22],
  tyre_wear: [0, 0, 0, 0],
  grip: 0.97,
  air_c: 26,
  road_c: 36,
});

function pass(seq: number, corner: number, lap: number, seconds: number, valid = true): PassRecord {
  return {
    seq,
    corner,
    lap,
    t_start: "2026-11-13T01:20:00Z",
    duration_s: seconds,
    valid,
    ...(valid ? {} : { invalid_reason: "off track" }),
    n_samples: 4,
    conditions: conditions(50 - seq, 70 + seq),
    flags: [],
    metrics: {},
  };
}

/** Two laps through T1 and T2, and a third T1 that went off track. */
export const cotaPasses: SessionPasses = {
  session_id: 2,
  track_length_m: 5428.8,
  corners: [
    {
      corner: 1,
      name: "T1",
      passes: 3,
      valid_passes: 2,
      best_s: 9.164,
      best_seq: 3,
      median_s: 9.2,
      spread_s: 0.05,
    },
    {
      corner: 2,
      name: "T2",
      passes: 2,
      valid_passes: 2,
      best_s: 4.378,
      best_seq: 2,
      median_s: 4.4,
    },
  ],
  passes: [
    pass(1, 1, 0, 9.236),
    pass(2, 2, 0, 4.378),
    pass(3, 1, 1, 9.164),
    pass(4, 2, 1, 4.6),
    pass(5, 1, 2, 9.01, false),
  ],
};

/** A pass's samples: four of them, from `start` metres on. */
export function traceOf(seq: number, start = 600): PassTrace {
  const four = (a: number, b: number, c: number, d: number) => [a, b, c, d];
  return {
    session_id: 2,
    seq,
    corner: 1,
    n_recorded: 4,
    n: 4,
    step: 1,
    columns: {
      t: four(0, 0.1, 0.2, 0.3),
      d: four(start, start + 5, start + 10, start + 15),
      v: four(80, 70, 60, 65),
      gas: four(1, 0, 0, 0.5),
      brake: four(0, 1, 0.5, 0),
      steer: four(0, -10, -40, -20),
      lat_g: four(0, 1, 2, 1),
      lon_g: four(0, -2, -1, 0),
      yaw: four(0, 0, 0, 0),
      sa_fl: four(0, 1, 4, 2),
      sa_fr: four(0, 1, 4, 2),
      sa_rl: four(0, 1, 2, 1),
      sa_rr: four(0, 1, 2, 1),
      sr_fl: four(0, 0, 0, 0),
      sr_fr: four(0, 0, 0, 0),
      sr_rl: four(0, 0, 0, 0),
      sr_rr: four(0, 0, 0, 0),
      load_fl: four(3000, 3000, 3000, 3000),
      load_fr: four(3000, 3000, 3000, 3000),
      load_rl: four(3000, 3000, 3000, 3000),
      load_rr: four(3000, 3000, 3000, 3000),
      out: four(0, 0, 0, 0),
      rh_f: four(0, 0, 0, 0),
      rh_r: four(0, 0, 0, 0),
    },
    axles: { front_slip_deg: four(0, 1, 4, 2), rear_slip_deg: four(0, 1, 2, 1) },
  };
}
