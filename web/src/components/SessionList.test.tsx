import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { SessionSummary } from "../api/client";
import { SessionList } from "./SessionList";

const demo: SessionSummary = {
  id: 1,
  spool_id: "demo-fa26-cota",
  car_name: "VRC Formula Alpha 2026",
  track_name: "Circuit of the Americas",
  layout: "layout_f1_2025",
  started_at: "2026-10-26T15:10:03Z",
  source: "demo",
  passes: 136,
  corners: 17,
};

describe("SessionList", () => {
  it("shows one row per session with what was recorded", async () => {
    const driven: SessionSummary = { ...demo, id: 2, source: "human", layout: "", passes: 40 };
    render(<SessionList loadSessions={() => Promise.resolve([driven, demo])} />);

    const rows = await screen.findAllByRole("row");

    expect(rows).toHaveLength(3); // header + two sessions
    const [, first, second] = rows as [HTMLElement, HTMLElement, HTMLElement];
    expect(within(first).getByText("Driven")).toBeInTheDocument();
    expect(within(first).getByText("Circuit of the Americas")).toBeInTheDocument();
    expect(within(first).getByText("40")).toBeInTheDocument();
    expect(within(second).getByText("Demo data")).toBeInTheDocument();
    expect(
      within(second).getByText("Circuit of the Americas (layout_f1_2025)"),
    ).toBeInTheDocument();
    expect(within(second).getByText("136")).toBeInTheDocument();
    expect(within(second).getByText("17")).toBeInTheDocument();
  });

  it("says how to get data when nothing is recorded yet", async () => {
    render(<SessionList loadSessions={() => Promise.resolve([])} />);

    expect(await screen.findByText(/No sessions yet/)).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("reports a failed load", async () => {
    render(<SessionList loadSessions={() => Promise.reject(new Error("502"))} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("Sessions could not be loaded.");
  });
});
