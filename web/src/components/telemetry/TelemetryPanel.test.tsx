import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { cotaPasses, traceOf } from "../../test-passes";
import { TelemetryPanel } from "./TelemetryPanel";

vi.mock("../charts/UPlotChart", () => ({
  UPlotChart: ({ label, data }: { label: string; data: unknown[][] }) => (
    <div role="img" aria-label={label} data-series={data.length - 1} />
  ),
}));

function show(props: { corner?: number; pass?: number } = {}) {
  const load = vi.fn((seq: number) => Promise.resolve(traceOf(seq, 600 + seq)));
  render(<TelemetryPanel sessionId={2} data={cotaPasses} loadTrace={load} {...props} />);
  return load;
}

describe("TelemetryPanel", () => {
  it("overlays the corner's best pass and the newest other valid one", async () => {
    const load = show();

    const speed = await screen.findByRole("img", { name: /^Speed in km\/h against distance/ });

    expect(load.mock.calls.map(([seq]) => seq).sort()).toEqual([1, 3]);
    expect(speed).toHaveAccessibleName("Speed in km/h against distance: L1 #1, L2 #3 best");
    expect(speed).toHaveAttribute("data-series", "2");
    expect(screen.getAllByRole("img")).toHaveLength(6);
  });

  it("lists the corner's passes with their gap to the best, and the reason one does not count", () => {
    show({ pass: 5 });

    const picker = screen.getByRole("table", { name: "Passes through this corner" });
    const rows = within(picker).getAllByRole("row");

    expect(rows).toHaveLength(4);
    expect(rows[1]).toHaveTextContent("L19.236+0.072");
    expect(rows[2]).toHaveTextContent("L29.164best");
    expect(rows[3]).toHaveTextContent("L39.010off track");
    expect(within(rows[3] as HTMLElement).getByRole("link", { name: "L3" })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("loads another pass when it is ticked, and drops it when unticked", async () => {
    const load = show({ pass: 3 });
    await screen.findAllByRole("img");

    fireEvent.click(screen.getByRole("checkbox", { name: "Overlay pass 5" }));

    await vi.waitFor(() => {
      expect(screen.getByRole("img", { name: /^Brake/ })).toHaveAttribute("data-series", "2");
    });
    expect(load).toHaveBeenCalledWith(5);
    fireEvent.click(screen.getByRole("checkbox", { name: "Overlay pass 5" }));
    expect(screen.getByRole("img", { name: /^Brake/ })).toHaveAttribute("data-series", "1");
  });

  it("switches the x axis to time and offers every corner", async () => {
    show({ corner: 2 });
    await screen.findAllByRole("img");

    fireEvent.click(screen.getByRole("radio", { name: "Time" }));

    expect(screen.getByRole("img", { name: /^Steering in ° against time/ })).toBeVisible();
    const corners = screen.getByRole("navigation", { name: "Corners" });
    expect(within(corners).getByRole("link", { name: "T2" })).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(within(corners).getByRole("link", { name: "T1" })).toHaveAttribute(
      "href",
      "#/sessions/2/telemetry?corner=1",
    );
  });

  it("reads the values of every pass at the cursor", async () => {
    show();

    const readout = await screen.findByRole("table", { name: "Values at the cursor" });

    expect(within(readout).getAllByRole("row")).toHaveLength(3);
    expect(screen.getByText("Move over a chart to read the values.")).toBeVisible();
  });

  it("says when samples could not be loaded", async () => {
    render(
      <TelemetryPanel
        sessionId={2}
        data={cotaPasses}
        loadTrace={() => Promise.reject(new Error("404"))}
      />,
    );

    expect(await screen.findByRole("alert")).toHaveTextContent("could not be loaded");
  });
});
