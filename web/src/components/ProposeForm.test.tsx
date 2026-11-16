import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { SetupItem } from "../api/client";
import { running } from "../test-data";
import { type ProposeApi, ProposeForm, toRequest } from "./ProposeForm";

const items: SetupItem[] = [
  { name: "ARB_FRONT", label: "Front ARB", min: 0, max: 4, step: 1, value: 3 },
  { name: "ARB_REAR", label: "Rear ARB", min: 0, max: 4, step: 1, value: 1 },
];

function apiWith(change: Partial<ProposeApi> = {}): ProposeApi {
  return {
    items: vi.fn(() => Promise.resolve(items)),
    create: vi.fn(() => Promise.resolve(running)),
    ...change,
  };
}

describe("ProposeForm", () => {
  it("proposes a one-item change against the session, with the prediction", async () => {
    const api = apiWith();
    const onProposed = vi.fn();
    render(<ProposeForm sessionId={3} api={api} onProposed={onProposed} />);

    await screen.findByRole("option", { name: /Rear ARB/ }); // the items are loaded
    fireEvent.change(screen.getByLabelText("Setup item"), { target: { value: "ARB_REAR" } });
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
      driver: "human",
      statement: "A stiffer rear bar helps rotation.",
      predictions: [{ channel: "corner_time_s", direction: "decrease", low: 0.01, high: 0.05 }],
    });
  });

  it("offers the item's current value and range, and starts on the first item", async () => {
    render(<ProposeForm sessionId={3} api={apiWith()} onProposed={vi.fn()} />);

    const option = await screen.findByRole("option", {
      name: "Front ARB [ARB_FRONT]: now 3, 0 to 4",
    });

    expect(option).toBeInTheDocument();
    expect(screen.getByLabelText("Setup item")).toHaveValue("ARB_FRONT");
  });

  it("says what core refused", async () => {
    const refusal = new Error("ARB_FRONT = 5 is above its maximum 4");
    const api = apiWith({ create: () => Promise.reject(refusal) });
    render(<ProposeForm sessionId={3} api={api} onProposed={vi.fn()} />);

    await screen.findByRole("option", { name: /Front ARB/ });
    fireEvent.change(screen.getByLabelText("Hypothesis"), { target: { value: "More bar." } });
    fireEvent.click(screen.getByRole("button", { name: "Propose experiment" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("above its maximum 4");
  });

  it("starts from a diagnosis candidate and names the hypothesis it tests", async () => {
    const api = apiWith();
    const prefill = {
      sessionId: 3,
      parameter: "ARB_REAR",
      clicks: 1,
      statement: "Understeer in slow corners. Test: a stiffer rear bar.",
      hypothesisKey: "mid_understeer.slow",
    };
    render(<ProposeForm sessionId={3} api={api} prefill={prefill} onProposed={vi.fn()} />);

    await screen.findByRole("option", { name: /Rear ARB/ });
    expect(screen.getByLabelText("Setup item")).toHaveValue("ARB_REAR");
    expect(screen.getByLabelText("Hypothesis")).toHaveValue(prefill.statement);
    expect(screen.getByText("mid_understeer.slow")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Propose experiment" }));

    await vi.waitFor(() => {
      expect(api.create).toHaveBeenCalledWith({
        session_id: 3,
        parameter: "ARB_REAR",
        clicks: 1,
        driver: "human",
        statement: prefill.statement,
        predictions: [],
        hypothesis_key: "mid_understeer.slow",
      });
    });
  });

  it("starts the prediction from the gain the car's profile expects", async () => {
    const api = apiWith();
    const prefill = {
      sessionId: 3,
      parameter: "ARB_FRONT",
      clicks: -1,
      statement: "Understeer in slow corners. Test: a softer front bar.",
      hypothesisKey: "mid_understeer.slow",
      gain: { low: 0.016, high: 0.026 },
    };
    render(<ProposeForm sessionId={3} api={api} prefill={prefill} onProposed={vi.fn()} />);

    await screen.findByRole("option", { name: /Front ARB/ });
    expect(screen.getByLabelText(/Predicted gain per corner pass/)).toHaveValue(0.016);
    expect(screen.getByLabelText("to")).toHaveValue(0.026);
    fireEvent.click(screen.getByRole("button", { name: "Propose experiment" }));

    await vi.waitFor(() => {
      expect(api.create).toHaveBeenCalledWith(
        expect.objectContaining({
          predictions: [
            { channel: "corner_time_s", direction: "decrease", low: 0.016, high: 0.026 },
          ],
        }),
      );
    });
  });
});

describe("toRequest", () => {
  const draft = {
    parameter: "ARB_FRONT",
    clicks: -1,
    statement: "Softer.",
    gainLow: "",
    gainHigh: "",
    hypothesisKey: "",
  };

  it("sends no prediction when the gain is left empty or is not a range", () => {
    expect(toRequest(3, draft).predictions).toEqual([]);
    expect(toRequest(3, { ...draft, gainLow: "0.05", gainHigh: "0.01" }).predictions).toEqual([]);
    expect(toRequest(3, { ...draft, gainLow: "0.01" }).predictions).toEqual([]);
  });
});
