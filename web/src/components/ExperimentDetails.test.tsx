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

describe("VerdictSection finding", () => {
  const verdict = tradeoff.verdict;
  if (verdict === undefined) throw new Error("the fixture has a verdict");

  it("shows the finding, the look and the next step in plain words", () => {
    const judged = {
      ...tradeoff,
      verdict: {
        ...verdict,
        decision: "inconclusive" as const,
        finding_class: "context_dependent" as const,
        finding_derived: false,
        recommendation: "investigate_context" as const,
        look: 1,
        flags: [],
        confounders: [
          { kind: "speed_class_interaction" as const, name: "fast vs medium" },
          { kind: "phase_interaction" as const, name: "braking vs exit" },
          { kind: "setup_mismatch" as const, name: "setup mismatch", count: 3 },
        ],
      },
    };

    render(<VerdictSection experiment={judged} />);

    const finding = screen.getByRole("group", { name: "Finding" });
    expect(finding).toHaveTextContent("Depends on the corner: better in some, worse in others");
    expect(finding).toHaveTextContent("Judged at look 1 of 3.");
    expect(finding).toHaveTextContent("Next: Look at the speed classes and corner phases");
    expect(finding).toHaveTextContent("effect differs between: fast vs medium");
    expect(finding).toHaveTextContent("effect differs between the phases: braking vs exit");
    expect(finding).toHaveTextContent("passes on another setup left out: setup mismatch (3)");
  });

  it("says when a finding was read from an old verdict, and shows none for a confirmed one's next step", () => {
    const old = {
      ...tradeoff,
      verdict: {
        ...verdict,
        finding_class: "confirmed_improvement" as const,
        finding_derived: true,
      },
    };

    render(<VerdictSection experiment={old} />);

    const finding = screen.getByRole("group", { name: "Finding" });
    expect(finding).toHaveTextContent("Confirmed improvement");
    expect(finding).toHaveTextContent("judged before findings existed");
    expect(finding).not.toHaveTextContent("Next:");
  });

  it("shows nothing when the verdict has no finding", () => {
    render(<VerdictSection experiment={tradeoff} />);

    expect(screen.queryByRole("group", { name: "Finding" })).toBeNull();
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
