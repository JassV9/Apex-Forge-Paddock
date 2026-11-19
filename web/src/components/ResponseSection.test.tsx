import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { tradeoff } from "../test-data";
import { ResponseSection } from "./ResponseSection";

const responses = [
  {
    metric: "rear_slip_deg",
    phase: "mid" as const,
    regime: "all" as const,
    expected: "direct" as const,
    finding: "increase" as const,
    estimate: 0.108,
    ci_low: 0.04,
    ci_high: 0.175,
    margin: 0.05,
    n_passes: 336,
  },
  {
    metric: "slip_balance_deg",
    phase: "exit" as const,
    regime: "fast" as const,
    expected: "proxy" as const,
    finding: "likely_decrease" as const,
    estimate: -0.0141,
    ci_low: -0.02,
    ci_high: -0.008,
    margin: 0.05,
    n_passes: 96,
  },
];

describe("ResponseSection", () => {
  it("shows each expected cell with its finding, interval, phase and regime, direct or proxy", () => {
    render(<ResponseSection experiment={{ ...tradeoff, responses }} />);

    const section = screen.getByRole("region", { name: "What the car did" });
    const rows = within(section).getAllByRole("row");
    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent("rear_slip_deg");
    expect(rows[1]).toHaveTextContent("mid");
    expect(rows[1]).toHaveTextContent("direct");
    expect(rows[1]).toHaveTextContent("+0.108 [+0.0400, +0.175]");
    expect(rows[1]).toHaveTextContent("went up");
    expect(rows[2]).toHaveTextContent("fast");
    expect(rows[2]).toHaveTextContent("proxy");
    expect(rows[2]).toHaveTextContent("probably down, not by the margin");
  });

  it("is absent for an experiment judged before responses were part of the verdict", () => {
    render(<ResponseSection experiment={tradeoff} />);

    expect(screen.queryByRole("region", { name: "What the car did" })).toBeNull();
  });
});
