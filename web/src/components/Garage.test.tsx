import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { SensitivityEffect, SensitivityProfile } from "../api/client";
import { Garage } from "./Garage";

const measured: SensitivityEffect = {
  parameter: "ARB_FRONT",
  clicks: -1,
  channel: "corner_time_s",
  corner_class: "all",
  estimate: -0.021,
  ci_low: -0.026,
  ci_high: -0.016,
  n_passes: 168,
  effect_class: "negative",
  nonlinear: false,
  note: "",
  is_prior: false,
  prior_source: "",
  experiment_ids: [3, 8],
};

const wing: SensitivityEffect = {
  ...measured,
  parameter: "WING_REAR",
  clicks: 1,
  estimate: 0.015,
  ci_low: 0.011,
  ci_high: 0.019,
  n_passes: 84,
  effect_class: "positive",
  nonlinear: true,
  note: "+1 and -1 clicks are not mirror images of each other.",
  experiment_ids: [9],
};

const borrowed: SensitivityEffect = {
  ...measured,
  parameter: "ARB_REAR",
  clicks: 1,
  estimate: -0.008,
  ci_low: -0.03,
  ci_high: 0.014,
  n_passes: 0,
  effect_class: "inconclusive",
  is_prior: true,
  prior_source: "Formula Alpha 2025",
  note: "Prior from Formula Alpha 2025, not measured on this car.",
  experiment_ids: [],
};

const mazda: SensitivityProfile = {
  car_id: 1,
  car_name: "Mazda MX-5 Cup",
  experiments: 3,
  model_version: "inverse-variance-pool-1",
  labels: { ARB_FRONT: "Front ARB", ARB_REAR: "Rear ARB" },
  effects: [measured, wing, borrowed],
};

const untested: SensitivityProfile = {
  car_id: 2,
  car_name: "Ferrari F2026",
  experiments: 0,
  model_version: "inverse-variance-pool-1",
  labels: {},
  effects: [],
};

function rowOf(text: RegExp): HTMLElement {
  return screen.getByText(text).closest("tr") as HTMLElement;
}

describe("Garage", () => {
  it("shows a measured effect with its interval and what it rests on", async () => {
    render(<Garage loadProfiles={() => Promise.resolve([mazda, untested])} />);

    const car = await screen.findByRole("region", { name: "Mazda MX-5 Cup" });

    expect(within(car).getByText(/3 judged experiments/)).toBeVisible();
    expect(within(car).getByText("inverse-variance-pool-1")).toBeVisible();
    const row = rowOf(/Front ARB −1 click/);
    expect(within(row).getByText("−0.021 [−0.026, −0.016]")).toBeVisible();
    expect(within(row).getByText("Lowers it")).toBeVisible();
    expect(within(row).getByText("Measured: 2 experiments, 168 passes")).toBeVisible();
    expect(row).not.toHaveClass("prior");
  });

  it("words a borrowed effect as a prior, never as a measurement", async () => {
    render(<Garage loadProfiles={() => Promise.resolve([mazda])} />);

    await screen.findByRole("region", { name: "Mazda MX-5 Cup" });
    const row = rowOf(/Rear ARB \+1 click/);

    expect(row).toHaveClass("prior");
    expect(
      within(row).getByText("Prior from Formula Alpha 2025, not measured on this car"),
    ).toBeVisible();
    expect(within(row).queryByText(/Measured/)).toBeNull();
    expect(within(row).getByText("Inconclusive")).toBeVisible();
    expect(row.querySelector("svg")).toHaveClass("interval-prior");
  });

  it("flags a response that is not a straight line, and falls back to the item id", async () => {
    render(<Garage loadProfiles={() => Promise.resolve([mazda])} />);

    await screen.findByRole("region", { name: "Mazda MX-5 Cup" });
    const row = rowOf(/WING_REAR \+1 click/);

    expect(within(row).getByText("non-linear")).toBeVisible();
    expect(within(row).getByText(/not mirror images/)).toBeVisible();
  });

  it("says so when a car has nothing measured", async () => {
    render(<Garage loadProfiles={() => Promise.resolve([untested])} />);

    const car = await screen.findByRole("region", { name: "Ferrari F2026" });

    expect(within(car).getByText(/Nothing measured yet/)).toBeVisible();
    expect(within(car).queryByRole("table")).toBeNull();
  });

  it("says what to do without cars, and reports a failed load", async () => {
    const { unmount } = render(<Garage loadProfiles={() => Promise.resolve([])} />);
    expect(await screen.findByText(/No cars yet/)).toBeVisible();
    unmount();

    render(<Garage loadProfiles={() => Promise.reject(new Error("502"))} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("The garage could not be loaded.");
  });
});
