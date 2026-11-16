import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { cotaReference } from "../../test-reference";
import { ReferencePanel } from "./ReferencePanel";

describe("ReferencePanel", () => {
  it("lists the corners by time lost with the AI, the verdict and what differs", async () => {
    render(<ReferencePanel sessionId={2} load={() => Promise.resolve(cotaReference)} />);

    const table = await screen.findByRole("table", { name: /time lost to the AI/ });
    const rows = within(table).getAllByRole("row");

    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("T19.1649.2009.050 ±0.003+0.150Practise");
    expect(rows[1]).toHaveTextContent("Brake point (m before apex): 120 vs AI 100 (2 / 9 passes)");
    expect(rows[2]).toHaveTextContent("Car / setup");
    expect(rows[2]).toHaveTextContent("AI on the nearest setup: 2 items differ");
    expect(within(rows[1] as HTMLElement).getByRole("link", { name: "T1" })).toHaveAttribute(
      "href",
      "#/sessions/2/telemetry?corner=1",
    );
    expect(screen.getByText(/40 AI passes on this car and track/)).toBeVisible();
  });

  it("compares another setup of the session when it is picked", async () => {
    const load = vi.fn(() => Promise.resolve(cotaReference));
    render(<ReferencePanel sessionId={2} load={load} />);

    fireEvent.change(await screen.findByRole("combobox"), { target: { value: "s2" } });

    await vi.waitFor(() => {
      expect(load).toHaveBeenLastCalledWith(2, "s2");
    });
    expect(load).toHaveBeenNthCalledWith(1, 2, undefined);
  });

  it("says when the comparison could not be loaded", async () => {
    render(<ReferencePanel sessionId={2} load={() => Promise.reject(new Error("500"))} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be loaded");
  });
});
