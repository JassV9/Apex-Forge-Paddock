import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { running, tradeoff } from "../test-data";
import { Ledger } from "./Ledger";

describe("Ledger", () => {
  it("lists every experiment with its state and why it is there", async () => {
    const judged = { ...tradeoff, id: 5 };
    render(<Ledger loadExperiments={() => Promise.resolve([judged, running])} />);

    const rows = await screen.findAllByRole("row");

    expect(rows).toHaveLength(3); // header + two experiments
    const [, first, second] = rows as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(first).getByRole("link", { name: "#5" })).toHaveAttribute(
      "href",
      "#/experiments/5",
    );
    expect(within(first).getByText("Trade-off")).toBeInTheDocument();
    expect(within(first).getByText(/rear_slip_peak_deg got worse/)).toBeInTheDocument();
    expect(within(first).getByText("1 of 1")).toBeInTheDocument();
    expect(within(second).getByText("Running")).toBeInTheDocument();
    expect(within(second).getByText("Front ARB −1 click")).toBeInTheDocument();
    expect(within(second).getByText("Run plan and setups written for the agent.")).toBeVisible();
  });

  it("says what to do when nothing was proposed yet", async () => {
    render(<Ledger loadExperiments={() => Promise.resolve([])} />);

    expect(await screen.findByText(/No experiments yet/)).toBeInTheDocument();
  });

  it("reports a failed load", async () => {
    render(<Ledger loadExperiments={() => Promise.reject(new Error("502"))} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("The ledger could not be loaded.");
  });
});
