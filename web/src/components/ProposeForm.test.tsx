import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SessionSummary, SetupItem } from "../api/client";
import { running } from "../test-data";
import { type ProposeApi, ProposeForm, toRequest } from "./ProposeForm";

const session: SessionSummary = {
  id: 3,
  spool_id: "20261112T100000-ks_mazda_mx5_cup",
  car_name: "Mazda MX-5 Cup",
  track_name: "Test Ring",
  layout: "",
  started_at: "2026-11-12T10:00:00Z",
  source: "human",
  passes: 28,
  corners: 7,
};

const items: SetupItem[] = [
  { name: "ARB_FRONT", label: "Front ARB", min: 0, max: 4, step: 1, value: 3 },
  { name: "ARB_REAR", label: "Rear ARB", min: 0, max: 4, step: 1, value: 1 },
];

function apiWith(change: Partial<ProposeApi> = {}): ProposeApi {
  return {
    sessions: () => Promise.resolve([session]),
    items: vi.fn(() => Promise.resolve(items)),
    create: vi.fn(() => Promise.resolve(running)),
    ...change,
  };
}

describe("ProposeForm", () => {
  it("proposes a one-item change against the session, with the prediction", async () => {
    const api = apiWith();
    const onProposed = vi.fn();
    render(<ProposeForm api={api} onProposed={onProposed} />);

    fireEvent.change(await screen.findByLabelText("Setup item"), { target: { value: "ARB_REAR" } });
    fireEvent.change(screen.getByLabelText(/Clicks/), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Hypothesis"), {
      target: { value: "  A stiffer rear bar helps rotation. " },
    });
    fireEvent.change(screen.getByLabelText(/Predicted gain/), { target: { value: "0.01" } });
    fireEvent.change(screen.getByLabelText("to"), { target: { value: "0.05" } });
    fireEvent.click(screen.getByRole("button", { name: "Propose experiment" }));

    await vi.waitFor(() => {
      expect(onProposed).toHaveBeenCalledWith(running);
    });
    expect(api.items).toHaveBeenCalledWith(3);
    expect(api.create).toHaveBeenCalledWith({
      session_id: 3,
      parameter: "ARB_REAR",
      clicks: 2,
      statement: "A stiffer rear bar helps rotation.",
      predictions: [{ channel: "corner_time_s", direction: "decrease", low: 0.01, high: 0.05 }],
    });
  });

  it("offers the item's current value and range, and starts on the first item", async () => {
    render(<ProposeForm api={apiWith()} onProposed={vi.fn()} />);

    const option = await screen.findByRole("option", {
      name: "Front ARB [ARB_FRONT]: now 3, 0 to 4",
    });

    expect(option).toBeInTheDocument();
    expect(screen.getByLabelText("Setup item")).toHaveValue("ARB_FRONT");
  });

  it("says what core refused", async () => {
    const refusal = new Error("ARB_FRONT = 5 is above its maximum 4");
    const api = apiWith({ create: () => Promise.reject(refusal) });
    render(<ProposeForm api={api} onProposed={vi.fn()} />);

    await screen.findByRole("option", { name: /Front ARB/ });
    fireEvent.change(screen.getByLabelText("Hypothesis"), { target: { value: "More bar." } });
    fireEvent.click(screen.getByRole("button", { name: "Propose experiment" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("above its maximum 4");
  });

  it("asks for a session when none was recorded", async () => {
    render(
      <ProposeForm api={apiWith({ sessions: () => Promise.resolve([]) })} onProposed={vi.fn()} />,
    );

    expect(await screen.findByText(/Record a session first/)).toBeInTheDocument();
  });
});

describe("toRequest", () => {
  const draft = {
    parameter: "ARB_FRONT",
    clicks: -1,
    statement: "Softer.",
    gainLow: "",
    gainHigh: "",
  };

  it("sends no prediction when the gain is left empty or is not a range", () => {
    expect(toRequest(3, draft).predictions).toEqual([]);
    expect(toRequest(3, { ...draft, gainLow: "0.05", gainHigh: "0.01" }).predictions).toEqual([]);
    expect(toRequest(3, { ...draft, gainLow: "0.01" }).predictions).toEqual([]);
  });
});
