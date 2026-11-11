import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { Health } from "../api/client";
import { CoreStatus } from "./CoreStatus";

const neverAnswers = () => new Promise<Health>(() => undefined);

describe("CoreStatus", () => {
  it("says it is checking until core answers", () => {
    render(<CoreStatus fetchHealth={neverAnswers} />);

    expect(screen.getByRole("status")).toHaveTextContent("Checking core…");
  });

  it.each<{ health: Health; text: string }>([
    {
      health: { service: "core", version: "1.2.3", status: "ok" },
      text: "Core 1.2.3 is running.",
    },
    {
      health: { service: "core", version: "1.2.3", status: "degraded" },
      text: "Core 1.2.3 is running, but its database is unavailable.",
    },
  ])("reports core as $health.status", async ({ health, text }) => {
    render(<CoreStatus fetchHealth={() => Promise.resolve(health)} />);

    expect(await screen.findByText(text)).toBeInTheDocument();
  });

  it("explains what to do when core cannot be reached", async () => {
    render(<CoreStatus fetchHealth={() => Promise.reject(new Error("connection refused"))} />);

    expect(await screen.findByText(/Core is not reachable/)).toBeInTheDocument();
  });
});
