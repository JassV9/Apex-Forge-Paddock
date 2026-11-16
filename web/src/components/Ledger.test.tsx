import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { running, tradeoff } from "../test-data";
import { Ledger } from "./Ledger";

describe("Ledger", () => {
  it("lists the car's experiments with their state and why they are there", async () => {
    const judged = { ...tradeoff, id: 5 };
    const otherCar = { ...running, id: 6, car_name: "Ferrari F2026" };
    render(
      <Ledger
        sessionId={3}
        carName="Mazda MX-5 Cup"
        selected={5}
        loadExperiments={() => Promise.resolve([judged, running, otherCar])}
      />,
    );

    const rows = await screen.findAllByRole("row");

    expect(rows).toHaveLength(3); // header + two experiments
    const [, first, second] = rows as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(first).getByRole("link", { name: "#5" })).toHaveAttribute(
      "href",
      "#/sessions/3/experiments/5",
    );
    expect(within(first).getByRole("link", { name: "#5" })).toHaveAttribute("aria-current", "true");
    expect(within(first).getByText("Trade-off")).toBeInTheDocument();
    expect(within(first).getByText(/rear_slip_peak_deg got worse/)).toBeInTheDocument();
    expect(within(first).getByText("1 of 1")).toBeInTheDocument();
    expect(within(second).getByText("Running")).toBeInTheDocument();
    expect(within(second).getByText("Front ARB −1 click")).toBeInTheDocument();
    expect(within(second).getByText("Run plan and setups written for the agent.")).toBeVisible();
  });

  it("says so when the car has no experiment", async () => {
    render(
      <Ledger
        sessionId={3}
        carName="Mazda MX-5 Cup"
        loadExperiments={() => Promise.resolve([{ ...running, car_name: "Other" }])}
      />,
    );

    expect(await screen.findByText(/No experiments on this car yet/)).toBeInTheDocument();
  });

  it("reports a failed load", async () => {
    render(
      <Ledger
        sessionId={3}
        carName="Mazda MX-5 Cup"
        loadExperiments={() => Promise.reject(new Error("502"))}
      />,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("The ledger could not be loaded.");
  });
});
