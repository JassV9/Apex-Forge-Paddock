import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { BaseSetupDriver, BaseSetupReply } from "../api/baseSetup";
import { cotaSession } from "../test-passes";
import { BaseSetupPanel } from "./BaseSetupPanel";
import { millis } from "./BaseSetupResult";

const built: BaseSetupReply = {
  car_name: "Ferrari 488 GT3",
  experiment_id: 14,
  base_setup: {
    channel: "corner_time_s",
    changes: [
      {
        parameter: "ARB_FRONT",
        label: "ARB Front",
        clicks: -1,
        from_value: 3,
        to_value: 2,
        estimate: -0.0042,
        ci_low: -0.0068,
        ci_high: -0.0016,
        n_passes: 84,
        effect_class: "inconclusive",
        experiment_ids: [3],
      },
    ],
    left_out: [
      {
        parameter: "WING",
        label: "Wing",
        clicks: 1,
        reason: "Flagged non-linear: 1 and -1 do not cancel",
      },
    ],
    measured: [],
    prediction: {
      channel: "corner_time_s",
      estimate: -0.0042,
      ci_low: -0.0068,
      ci_high: -0.0016,
      level: 0.9,
      note: "Sum of 1 effects measured one item at a time.",
    },
  },
};

const refused: BaseSetupReply = {
  car_name: "Ferrari 488 GT3",
  error: "No measured change qualifies. Train the AI on this car (free laps) first.",
  base_setup: {
    channel: "corner_time_s",
    changes: [],
    left_out: [],
    measured: [
      {
        parameter: "ARB_REAR",
        label: "ARB Rear",
        clicks: 1,
        estimate: 0.001,
        ci_low: -0.002,
        ci_high: 0.004,
        n_passes: 84,
        effect_class: "negligible",
      },
    ],
  },
};

function show(
  build: (sessionId: number, driver: BaseSetupDriver) => Promise<BaseSetupReply>,
  readOnly = false,
) {
  render(<BaseSetupPanel session={cotaSession} readOnly={readOnly} build={build} />);
}

describe("BaseSetupPanel", () => {
  it("builds with the chosen driver and shows each change with its evidence and the prediction", async () => {
    const build = vi.fn(() => Promise.resolve(built));
    show(build);

    fireEvent.click(screen.getByRole("radio", { name: "I drive" }));
    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));

    const changes = await screen.findByRole("list", { name: "Changes" });
    expect(build).toHaveBeenCalledWith(cotaSession.id, "human");
    expect(within(changes).getByRole("listitem")).toHaveTextContent(
      "ARB Front −1 click: 3 → 2. Measured −4.2 ms [−6.8 ms, −1.6 ms] per corner pass over 84 passes (inconclusive), experiments 3.",
    );
    expect(screen.getByText(/Prediction, written down before a lap is driven/)).toHaveTextContent(
      "at 90 %. Sum of 1 effects",
    );
    expect(screen.getByRole("link", { name: "Open experiment 14" })).toHaveAttribute(
      "href",
      expect.stringContaining("/14"),
    );
    expect(screen.getByText(/Flagged non-linear/)).toBeInTheDocument();
  });

  it("proposes that the person drives in their own session, and the AI in the AI's", () => {
    const build = vi.fn(() => Promise.resolve(built));
    show(build);
    expect(screen.getByRole("radio", { name: "I drive" })).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));
    expect(build).toHaveBeenLastCalledWith(cotaSession.id, "human");

    cleanup();
    render(
      <BaseSetupPanel session={{ ...cotaSession, source: "ai" }} readOnly={false} build={build} />,
    );
    expect(screen.getByRole("radio", { name: "AI drives" })).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));
    expect(build).toHaveBeenLastCalledWith(cotaSession.id, "ai");
  });

  it("says what to do next, for the person and for the AI", async () => {
    show(() => Promise.resolve(built));
    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));
    expect(await screen.findByText(/stop in the pits/)).toHaveTextContent("Load button");
    expect(screen.getByText(/only proposed so far/)).toHaveTextContent("press Plan and then Start");

    cleanup();
    show(() => Promise.resolve(built));
    fireEvent.click(screen.getByRole("radio", { name: "AI drives" }));
    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));
    expect(await screen.findByText(/takes the car at once/)).toHaveTextContent(
      "Let the AI drive this plan",
    );
  });

  it("explains a refusal with what the car measured", async () => {
    show(() => Promise.resolve(refused));

    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));

    expect(await screen.findByText(/Train the AI on this car/)).toBeInTheDocument();
    const measured = screen.getByRole("list", { name: "Measured on this car" });
    expect(measured).toHaveTextContent("ARB Rear +1 click: negligible, +1.0 ms [−2.0 ms, +4.0 ms]");
    expect(screen.queryByRole("link", { name: /Open experiment/ })).not.toBeInTheDocument();
  });

  it("says when core could not build it", async () => {
    show(() => Promise.reject(new Error("the analysis engine did not answer")));

    fireEvent.click(screen.getByRole("button", { name: "Build a base setup" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "The base setup could not be built. the analysis engine did not answer",
    );
  });

  it("offers nothing to build in the read-only demo", () => {
    show(() => Promise.resolve(built), true);

    expect(screen.queryByRole("button", { name: "Build a base setup" })).not.toBeInTheDocument();
    expect(screen.getByText(/cannot be built in the read-only demo/)).toBeInTheDocument();
  });

  it("writes milliseconds with their sign", () => {
    expect(millis(-0.0042)).toBe("−4.2 ms");
    expect(millis(0.01)).toBe("+10.0 ms");
    expect(millis(0)).toBe("0.0 ms");
  });
});
