import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { cotaPasses } from "../../test-passes";
import { OverviewPanel } from "./OverviewPanel";

// jsdom has no canvas: the chart is replaced by its accessible name and its point count.
vi.mock("../charts/UPlotChart", () => ({
  UPlotChart: ({ label, data }: { label: string; data: unknown[][] }) => (
    <div role="img" aria-label={label} data-points={data[0]?.length} />
  ),
}));

describe("OverviewPanel", () => {
  it("shows zone times laps down and corners across, each opening the pass", () => {
    render(<OverviewPanel data={cotaPasses} />);

    const grid = screen.getByRole("region", { name: "Zone times by lap" });
    const lap2 = within(grid).getByRole("row", { name: /^L2/ });
    const best = within(lap2).getByRole("link", { name: "9.164" });

    expect(best).toHaveAttribute("href", "#/sessions/2/telemetry?corner=1&pass=3");
    expect(best.closest("td")).toHaveClass("tone-best");
    expect(within(grid).getByRole("columnheader", { name: "T2" })).toBeVisible();
  });

  it("marks a pass that does not count and says why", () => {
    render(<OverviewPanel data={cotaPasses} />);

    const offTrack = screen.getByRole("link", { name: "9.010" });

    expect(offTrack.closest("td")).toHaveClass("tone-invalid");
    expect(offTrack).toHaveAttribute("title", "Pass 5, 9.010 s, not counted: off track");
  });

  it("gives each corner's best, median and spread, and nothing it cannot", () => {
    render(<OverviewPanel data={cotaPasses} />);

    expect(screen.getByRole("row", { name: "Best 9.164 4.378" })).toBeVisible();
    expect(screen.getByRole("row", { name: "Spread σ 0.050 –" })).toBeVisible();
  });

  it("sums up the session and charts its conditions", () => {
    render(<OverviewPanel data={cotaPasses} />);

    expect(screen.getByText("49.0 → 45.0 L")).toBeVisible();
    expect(screen.getByText("4 valid")).toBeVisible();
    const chart = screen.getByRole("img", { name: /Tyre core temperatures/ });
    expect(chart).toHaveAttribute("data-points", "5");
  });

  it("says so when nothing was recorded", () => {
    render(<OverviewPanel data={{ ...cotaPasses, corners: [], passes: [] }} />);

    expect(screen.getByText(/No corner passes/)).toBeVisible();
  });
});
