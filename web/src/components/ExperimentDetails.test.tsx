import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { running, tradeoff } from "../test-data";
import { PlanSection, VerdictSection } from "./ExperimentDetails";

describe("VerdictSection", () => {
  it("names an AI training program as the one that kept the baseline", () => {
    const verdict = tradeoff.verdict;
    if (verdict === undefined) throw new Error("the fixture has a verdict");
    const reverted = {
      ...tradeoff,
      state: "reverted" as const,
      verdict: {
        ...verdict,
        decision: "reverted" as const,
        decided_by: "training" as const,
        reason: "Training program 1 kept the baseline: still unclear after 1 extension(s).",
      },
    };

    render(<VerdictSection experiment={reverted} />);

    const section = screen.getByRole("region", { name: "Verdict" });
    expect(section).toHaveTextContent("decided by the AI training program");
    expect(section).toHaveTextContent("Training program 1 kept the baseline");
  });
});

describe("PlanSection", () => {
  it("says how much whole laps scatter and when the longest plan falls short", () => {
    const plan = {
      sigma: 0.0385,
      sigma_source: "measured" as const,
      passes_per_arm: 12,
      mde_detect: 0.025,
      mde_decide: 0.0328,
      sigma_lap: 0.0243,
      sigma_lap_source: "free" as const,
      mde_target: 0.025,
      target_met: false,
    };

    render(<PlanSection experiment={{ ...running, plan }} />);

    const section = screen.getByRole("region", { name: "Run plan" });
    expect(section).toHaveTextContent("Whole laps scatter 0.024 (free laps).");
    expect(section).toHaveTextContent("Short of the 0.025 it was sized for");
  });

  it("says nothing about laps for a plan made before they were measured", () => {
    render(<PlanSection experiment={running} />);

    expect(screen.getByRole("region", { name: "Run plan" })).not.toHaveTextContent("Whole laps");
  });
});
