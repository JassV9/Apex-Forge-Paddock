import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { SessionSummary } from "../../api/client";
import { SessionRail } from "./SessionRail";

function session(
  id: number,
  passes: number,
  source: SessionSummary["source"] = "human",
): SessionSummary {
  return {
    id,
    spool_id: `s${String(id)}`,
    source,
    car_name: `Car ${String(id)}`,
    track_name: "COTA",
    layout: "",
    started_at: "2026-11-15T22:00:00Z",
    passes,
    corners: passes > 0 ? 17 : 0,
  };
}

describe("SessionRail", () => {
  it("folds away sessions without passes until asked", () => {
    render(
      <SessionRail
        sessions={[session(3, 40, "ai"), session(2, 0), session(1, 12)]}
        current={3}
        tab="overview"
      />,
    );

    expect(screen.getByText("Car 3")).toBeVisible();
    expect(screen.getByText("Car 1")).toBeVisible();
    expect(screen.queryByText("Car 2")).toBeNull();
    expect(screen.getByText("Game AI")).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Show 1 empty session" }));
    expect(screen.getByText("Car 2")).toBeVisible();
    expect(screen.getByRole("button", { name: "Hide 1 empty session" })).toBeVisible();
  });

  it("keeps the session on screen even when it is empty", () => {
    render(<SessionRail sessions={[session(5, 0), session(4, 9)]} current={5} tab="overview" />);

    expect(screen.getByText("Car 5")).toBeVisible();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
