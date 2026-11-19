import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SessionSummary, SetupItem, Sweep } from "../../api/client";
import { DiscoverPanel, parseSteps } from "./DiscoverPanel";

const session: SessionSummary = {
  id: 105,
  spool_id: "20261118T101500-rss_formula_hybrid_2026-ai",
  car_name: "Formula Hybrid 2026",
  track_name: "Hungaroring",
  layout: "",
  started_at: "2026-11-18T10:15:00Z",
  passes: 336,
  corners: 14,
  source: "ai",
};

const items: SetupItem[] = [
  { name: "ARB_REAR", label: "Rear ARB", min: 0, max: 5, step: 1, value: 3 },
];

function sweep(over: Partial<Sweep>): Sweep {
  return {
    id: 2,
    session_id: 105,
    car_name: "Formula Hybrid 2026",
    track_name: "Hungaroring",
    parameter: "ARB_REAR",
    parameter_label: "Rear ARB",
    baseline_version_id: 9,
    ai_hint_share: 0.93,
    requested: [-1, 1, 2, 3],
    planned: [-1, 1, 2],
    skipped: ["+3 would be 6, beyond the maximum 5: clipped to +2, already a step; skipped"],
    state: "running",
    message: "The AI is driving step +1 of [-1 1 2] (experiment 41, block 3 of 8).",
    created_at: "2026-11-18T18:00:00Z",
    steps: [
      { experiment_id: 40, clicks: -1, state: "reverted", finding_class: "neutral" },
      { experiment_id: 41, clicks: 1, state: "running" },
    ],
    ...over,
  };
}

function fakeApi(sweeps: Sweep[]) {
  return {
    list: vi.fn(() => Promise.resolve(sweeps)),
    items: vi.fn(() => Promise.resolve(items)),
    start: vi.fn(() => Promise.resolve(sweep({}))),
    stop: vi.fn(() => Promise.resolve(sweep({ state: "stopped" }))),
  };
}

describe("DiscoverPanel", () => {
  it("starts a sweep only with a pinned hint", async () => {
    const api = fakeApi([]);
    render(<DiscoverPanel session={session} readOnly={false} api={api} />);

    await screen.findByRole("option", { name: /Rear ARB/ });
    fireEvent.change(screen.getByLabelText(/^Setting/), { target: { value: "ARB_REAR" } });
    const start = screen.getByRole("button", { name: "Start sweep" });
    expect(start).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Pinned AI hint share"), { target: { value: "0.93" } });
    fireEvent.click(start);

    expect(api.start).toHaveBeenCalledWith(105, {
      parameter: "ARB_REAR",
      steps: [-1, 1, 2, 3],
      ai_hint_share: 0.93,
    });
  });

  it("shows an open sweep's progress and what clipping skipped, and stops it", async () => {
    const api = fakeApi([sweep({})]);
    render(<DiscoverPanel session={session} readOnly={false} api={api} />);

    expect(await screen.findByText(/2 of 3 steps/)).toBeVisible();
    expect(screen.getByText(/clipped to \+2/)).toBeVisible();
    expect(screen.getByText("-1: #40 reverted (neutral)")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Start sweep" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Stop sweep" }));
    expect(api.stop).toHaveBeenCalledWith(2);
  });

  it("says each curve in plain words when done", async () => {
    const done = sweep({
      state: "done",
      message: "Done.",
      curve: {
        parameter: "ARB_REAR",
        level: 0.9,
        engine_version: "0.2.0",
        curves: [
          {
            metric: "rear_slip_deg",
            phase: "exit",
            regime: "slow",
            source: "response",
            unit: "deg",
            shape: "linear",
            clicks: [-1, 1, 2],
            slope: { estimate: 0.11, ci_low: 0.08, ci_high: 0.14 },
            reason: "The slope is clear of zero; the quadratic term is not.",
          },
        ],
      },
    });
    render(<DiscoverPanel session={session} readOnly api={fakeApi([done])} />);

    expect(
      await screen.findByText(/rear slip rises with each click, about 0.11 deg per click, linear/),
    ).toBeVisible();
    expect(screen.getByText("(exit, slow corners)")).toBeVisible();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("says why core refused to start", async () => {
    const api = fakeApi([]);
    api.start.mockImplementation(() =>
      Promise.reject(new Error("training program 3 is open on this car")),
    );
    render(<DiscoverPanel session={session} readOnly={false} api={api} />);

    await screen.findByRole("option", { name: /Rear ARB/ });
    fireEvent.change(screen.getByLabelText(/^Setting/), { target: { value: "ARB_REAR" } });
    fireEvent.change(screen.getByLabelText("Pinned AI hint share"), { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: "Start sweep" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("training program 3");
  });
});

describe("parseSteps", () => {
  it("reads whole clicks and refuses anything else", () => {
    expect(parseSteps("-1, 1 +2,3")).toEqual([-1, 1, 2, 3]);
    expect(parseSteps("1.5")).toBeNull();
    expect(parseSteps("  ")).toBeNull();
  });
});
