import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SessionSummary, TrainingProgram } from "../../api/client";
import { TrainingPanel } from "./TrainingPanel";

const session: SessionSummary = {
  id: 57,
  spool_id: "20261116T113827-ks_ferrari_488_gt3-ai",
  car_name: "Ferrari 488 GT3",
  track_name: "COTA F1 2026 (Paddock test)",
  layout: "layout_f1_2026_paddock",
  started_at: "2026-11-16T16:38:26Z",
  passes: 42,
  corners: 17,
  source: "ai",
};

function program(over: Partial<TrainingProgram>): TrainingProgram {
  return {
    id: 3,
    session_id: 57,
    car_name: "Ferrari 488 GT3",
    track_name: "COTA",
    target: 4,
    state: "running",
    message: "The AI is driving experiment 4 (block 2 of 4, lap 1 of 3).",
    created_at: "2026-11-16T18:00:00Z",
    steps: [
      { experiment_id: 3, parameter: "ARB_FRONT", clicks: -1, state: "reverted" },
      { experiment_id: 4, parameter: "ARB_REAR", clicks: 1, state: "running" },
    ],
    ...over,
  };
}

function fakeApi(programs: TrainingProgram[]) {
  return {
    list: vi.fn(() => Promise.resolve(programs)),
    start: vi.fn(() => Promise.resolve(program({}))),
    stop: vi.fn(() => Promise.resolve(program({ state: "stopped" }))),
  };
}

describe("TrainingPanel", () => {
  it("starts a program with the number of experiments asked for", async () => {
    const api = fakeApi([]);
    render(<TrainingPanel session={session} readOnly={false} api={api} />);

    const field = await screen.findByLabelText("Experiments");
    fireEvent.change(field, { target: { value: "6" } });
    fireEvent.click(screen.getByRole("button", { name: "Start training" }));

    expect(api.start).toHaveBeenCalledWith(57, 6);
  });

  it("shows the open program of this car with its steps, and stops it", async () => {
    const other = program({ id: 9, car_name: "Mazda MX-5 Cup", message: "another car" });
    const api = fakeApi([other, program({})]);
    render(<TrainingPanel session={session} readOnly={false} api={api} />);

    expect(await screen.findByText(/block 2 of 4/)).toBeVisible();
    expect(screen.getByText("#4 ARB_REAR +1: running")).toBeVisible();
    expect(screen.queryByText(/another car/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Start training" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Stop training" }));
    expect(api.stop).toHaveBeenCalledWith(3);
  });

  it("says why core refused to start", async () => {
    const api = fakeApi([]);
    api.start.mockImplementation(() =>
      Promise.reject(new Error("experiment 5 is on the car with you driving")),
    );
    render(<TrainingPanel session={session} readOnly={false} api={api} />);

    fireEvent.click(await screen.findByRole("button", { name: "Start training" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("with you driving");
  });

  it("offers no buttons in the read-only demo", async () => {
    const api = fakeApi([program({})]);
    render(<TrainingPanel session={session} readOnly api={api} />);

    expect(await screen.findByText(/block 2 of 4/)).toBeVisible();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
