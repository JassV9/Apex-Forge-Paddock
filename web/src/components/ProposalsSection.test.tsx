import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Proposal, Proposals } from "../api/client";
import { parseRoute } from "../hooks/useRoute";
import { diagnosis } from "../test-data";
import { ProposalsSection } from "./ProposalsSection";

const measured: Proposal = {
  parameter: "ARB_FRONT",
  label: "Front ARB",
  clicks: -1,
  why: "a softer front bar gives more front grip mid-corner",
  hypotheses: ["mid_understeer.slow"],
  basis: "measured",
  prior_source: "",
  expected_gain: 0.0161,
  gain_low: 0.0164,
  gain_high: 0.0258,
  laps_needed: 16,
  value_per_lap: 0.001,
};

const borrowed: Proposal = {
  ...measured,
  parameter: "ARB_REAR",
  label: "",
  clicks: 1,
  why: "a stiffer rear bar helps the car rotate",
  basis: "prior",
  prior_source: "Formula Alpha 2025",
  expected_gain: 0.004,
  gain_low: -0.014,
  gain_high: 0.03,
  laps_needed: 20,
};

const unknown: Proposal = { ...borrowed, parameter: "CAMBER_F", basis: "none", prior_source: "" };

const ranking: Proposals = {
  stage: "learning",
  ranked: [measured, borrowed, unknown],
  skipped: [
    {
      parameter: "WING_REAR",
      label: "Rear wing",
      clicks: 1,
      reason:
        "Measured on this car as negligible on corner_time_s: -0.003 to +0.002 over 168 passes.",
    },
  ],
};

function show(proposals: Proposals | null) {
  render(
    <ProposalsSection
      sessionId={3}
      diagnosis={diagnosis}
      load={() => Promise.resolve(proposals)}
    />,
  );
}

describe("ProposalsSection", () => {
  it("lists the experiments in the engine's order, each with what the expectation rests on", async () => {
    show(ranking);

    const section = await screen.findByRole("region", { name: "What to test next" });
    const [first, second, third] = within(section).getAllByRole("listitem");

    expect(first).toHaveTextContent("Front ARB −1 click: expected gain 0.016 s per corner pass");
    expect(first).toHaveTextContent("between +0.016 and +0.026");
    expect(first).toHaveTextContent("about 16 laps to decide. Basis: measured on this car.");
    expect(second).toHaveTextContent(
      "ARB_REAR +1 click: expected gain 0.004 s per corner pass (between −0.014 and +0.030)",
    );
    expect(second).toHaveTextContent(
      "Basis: a prior from Formula Alpha 2025, not measured on this car.",
    );
    expect(third).toHaveTextContent("Basis: nothing is known about it yet.");
    expect(section).toHaveTextContent("5 to 19 judged experiments on this car");
  });

  it("hands a measured gain to the propose form as the prediction, and no gain for a prior", async () => {
    show(ranking);

    const [first, second] = await screen.findAllByRole("link", { name: "Propose this" });

    expect(parseRoute(first?.getAttribute("href") ?? "")).toEqual({
      page: "propose",
      prefill: {
        sessionId: 3,
        parameter: "ARB_FRONT",
        clicks: -1,
        hypothesisKey: "mid_understeer.slow",
        statement: `${diagnosis.hypotheses[0]?.statement ?? ""} Test: a softer front bar gives more front grip mid-corner.`,
        gain: { low: 0.016, high: 0.026 },
      },
    });
    const prior = parseRoute(second?.getAttribute("href") ?? "");
    expect(prior.page === "propose" && prior.prefill?.gain).toBeUndefined();
  });

  it("says why a change the car already answered is left out", async () => {
    show(ranking);

    const section = await screen.findByRole("region", { name: "What to test next" });

    expect(within(section).getByText("Already answered on this car (1)")).toBeInTheDocument();
    expect(
      within(section).getByText(/Rear wing \+1 click: Measured on this car as negligible/),
    ).toBeInTheDocument();
  });

  it("shows nothing without a ranking, and reports a failed load", async () => {
    const { container, unmount } = render(
      <ProposalsSection sessionId={3} diagnosis={diagnosis} load={() => Promise.resolve(null)} />,
    );
    await Promise.resolve();
    expect(container).toBeEmptyDOMElement();
    unmount();

    render(
      <ProposalsSection
        sessionId={3}
        diagnosis={diagnosis}
        load={() => Promise.reject(new Error("502"))}
      />,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent("The ranking could not be loaded.");
  });
});
