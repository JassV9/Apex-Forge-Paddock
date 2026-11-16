import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { tradeoff } from "../test-data";
import { VerdictSection } from "./ExperimentDetails";

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
