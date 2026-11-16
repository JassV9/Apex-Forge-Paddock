import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { cotaPasses, traceOf } from "../../test-passes";
import { cotaReference } from "../../test-reference";
import type { AiPassSource } from "./AiOverlay";
import { TelemetryPanel } from "./TelemetryPanel";

vi.mock("../charts/UPlotChart", () => ({
  UPlotChart: ({ label, data }: { label: string; data: unknown[][] }) => (
    <div role="img" aria-label={label} data-series={data.length - 1} />
  ),
}));

function show(corner: number) {
  const loadAi = vi.fn((sessionId: number, seq: number) =>
    Promise.resolve({ ...traceOf(seq, 590), session_id: sessionId }),
  );
  const ai: AiPassSource = {
    corner: (n) => cotaReference.corners.find((c) => c.corner === n),
    load: loadAi,
  };
  const load = (seq: number) => Promise.resolve(traceOf(seq));
  render(
    <TelemetryPanel sessionId={2} data={cotaPasses} loadTrace={load} corner={corner} ai={ai} />,
  );
  return loadAi;
}

describe("the AI's pass in the telemetry", () => {
  it("overlays the AI's best pass of the corner on the same setup when ticked", async () => {
    const loadAi = show(1);
    await screen.findAllByRole("img");

    fireEvent.click(screen.getByRole("checkbox", { name: /Overlay the AI's best pass/ }));

    await vi.waitFor(() => {
      expect(screen.getByRole("img", { name: /^Speed/ })).toHaveAttribute("data-series", "3");
    });
    expect(loadAi).toHaveBeenCalledWith(57, 30);
    expect(screen.getByRole("img", { name: /^Speed/ })).toHaveAccessibleName(/AI best 9\.045/);
  });

  it("is not offered when the AI drove another setup there", async () => {
    show(2);
    await screen.findAllByRole("img");

    expect(screen.queryByRole("checkbox", { name: /Overlay the AI/ })).toBeNull();
  });
});
