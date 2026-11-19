import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { SensitivityEffect, SensitivityProfile } from "../api/client";
import { Garage } from "./Garage";
import type { SensitivityPrior } from "./GarageRows";

const measured: SensitivityEffect = {
  parameter: "ARB_FRONT",
  clicks: -1,
  channel: "corner_time_s",
  corner_class: "all",
  phase: "corner",
  track: "fn_hungaroring/layout_f1_2026",
  track_name: "Hungaroring F1 2026",
  driver: "ai",
  hint_bucket: "0.94",
  baseline_value: 3,
  click_step: 1,
  estimate: -0.021,
  ci_low: -0.026,
  ci_high: -0.016,
  standard_error: 0.002,
  p_increase: 0.999,
  tau: 0.0012,
  method: "hksj",
  experiments: 3,
  knowledge_level: "confirmed",
  effect_class: "negative",
  margin: 0.005,
  n_passes: 168,
  nonlinear: false,
  note: "",
  stale: false,
  experiment_ids: [3, 8, 11],
  engine_versions: ["0.2.0"],
};

const wing: SensitivityEffect = {
  ...measured,
  parameter: "WING_REAR",
  clicks: 1,
  estimate: 0.015,
  ci_low: 0.011,
  ci_high: 0.019,
  n_passes: 84,
  method: "single",
  experiments: 1,
  tau: undefined,
  knowledge_level: "preliminary",
  effect_class: "positive",
  nonlinear: true,
  note: "+1 and -1 clicks are not mirror images of each other.",
  experiment_ids: [9],
};

const stale: SensitivityEffect = {
  ...measured,
  parameter: "ARB_REAR",
  clicks: 1,
  driver: "human",
  hint_bucket: "none",
  method: "single",
  experiments: 1,
  knowledge_level: "insufficient",
  effect_class: "inconclusive",
  stale: true,
  note: "Not re-derived by the current engine (the ledger's numbers): never more than insufficient.",
  experiment_ids: [7],
  engine_versions: [],
};

const borrowed: SensitivityPrior = {
  parameter: "CAMBER_F",
  clicks: 1,
  channel: "corner_time_s",
  corner_class: "all",
  phase: "corner",
  estimate: -0.008,
  ci_low: -0.03,
  ci_high: 0.014,
  standard_error: 0.013,
  source_car: "Formula Alpha 2025",
  source_track: "fn_cota/layout_f1_2026",
  source_experiments: 2,
  weight: 0.5,
  note: "Prior from Formula Alpha 2025, not measured on this car.",
};

const mazda: SensitivityProfile = {
  car_id: 1,
  car_name: "Mazda MX-5 Cup",
  experiments: 5,
  model_version: "random-effects-hksj-1",
  labels: { ARB_FRONT: "Front ARB", ARB_REAR: "Rear ARB", CAMBER_F: "Front camber" },
  effects: [measured, wing, stale],
  priors: [borrowed],
  excluded: [{ experiment_id: 4, kind: "invalid", reason: "Its finding is invalid." }],
};

const untested: SensitivityProfile = {
  car_id: 2,
  car_name: "Ferrari F2026",
  experiments: 0,
  model_version: "random-effects-hksj-1",
  labels: {},
  effects: [],
  priors: [],
  excluded: [],
};

function rowOf(text: RegExp): HTMLElement {
  return screen.getByText(text).closest("tr") as HTMLElement;
}

async function show(profiles: SensitivityProfile[], car = "Mazda MX-5 Cup"): Promise<HTMLElement> {
  render(<Garage carName={car} loadProfiles={() => Promise.resolve(profiles)} />);
  return screen.findByRole("region", { name: car });
}

describe("Garage", () => {
  it("shows a measured row with its knowledge level, stratum and provenance", async () => {
    const car = await show([mazda, untested]);

    expect(within(car).getByText(/5 judged experiments/)).toBeVisible();
    expect(within(car).getByText("random-effects-hksj-1")).toBeVisible();
    const row = rowOf(/Front ARB −1 click/);
    expect(within(row).getByText("−0.021 [−0.026, −0.016]")).toBeVisible();
    expect(within(row).getByText("Confirmed on this car")).toBeVisible();
    expect(within(row).getByText("Hungaroring F1 2026, AI, hint 0.94, from 3")).toBeVisible();
    expect(
      within(row).getByText(
        "3 experiments (3, 8, 11), random effects, HKSJ, tau 0.0012; engine 0.2.0; 168 passes",
      ),
    ).toBeVisible();
    expect(row).not.toHaveClass("prior");
  });

  it("lists priors apart and labelled, never among the measurements", async () => {
    const car = await show([mazda]);

    const priors = within(car).getByRole("region", { name: "Priors from other cars" });
    expect(within(priors).getByText(/not measured on this car/, { selector: "h4" })).toBeVisible();
    const row = within(priors)
      .getByText(/Front camber \+1 click/)
      .closest("tr") as HTMLElement;
    expect(row).toHaveClass("prior");
    expect(
      within(row).getByText(/Prior from Formula Alpha 2025, not measured on this car/),
    ).toBeVisible();
    expect(row.querySelector("svg")).toHaveClass("interval-prior");
    expect(within(row).queryByText(/Confirmed|Preliminary|Neutral|Insufficient/)).toBeNull();
    const [measuredTable] = within(car).getAllByRole("table") as [HTMLElement, ...HTMLElement[]];
    expect(within(measuredTable).queryByText(/Front camber/)).toBeNull();
  });

  it("marks a stale row, flags a curve, and names the experiments left out", async () => {
    const car = await show([mazda]);

    const old = rowOf(/Rear ARB \+1 click/);
    expect(within(old).getByText("stale")).toBeVisible();
    expect(within(old).getByText("Insufficient evidence")).toBeVisible();
    expect(within(old).getByText(/ledger numbers, no engine version/)).toBeVisible();
    const bent = rowOf(/WING_REAR \+1 click/);
    expect(within(bent).getByText("non-linear")).toBeVisible();
    expect(within(bent).getByText(/not mirror images/)).toBeVisible();
    expect(within(car).getByText(/Experiment 4 not pooled \(invalid\)/)).toBeVisible();
  });

  it("says so when a car has nothing measured", async () => {
    const car = await show([untested], "Ferrari F2026");

    expect(within(car).getByText(/Nothing measured yet/)).toBeVisible();
    expect(within(car).queryByRole("table")).toBeNull();
  });

  it("shows only the session car, says when it has no profile, and reports a failed load", async () => {
    const { unmount } = render(
      <Garage carName="Mazda MX-5 Cup" loadProfiles={() => Promise.resolve([untested])} />,
    );
    expect(await screen.findByText(/no profile of this car/)).toBeVisible();
    expect(screen.queryByRole("region", { name: "Ferrari F2026" })).toBeNull();
    unmount();

    render(
      <Garage carName="Mazda MX-5 Cup" loadProfiles={() => Promise.reject(new Error("502"))} />,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("The garage could not be loaded.");
  });
});
