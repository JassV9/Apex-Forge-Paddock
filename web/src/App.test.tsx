import { render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SessionSummary } from "./api/client";
import { App } from "./App";
import type { Route } from "./hooks/useRoute";
import { Workspace } from "./components/workspace/Workspace";
import { cotaPasses, cotaSession, traceOf } from "./test-passes";
import { cotaReference } from "./test-reference";

vi.mock("./components/charts/UPlotChart", () => ({
  UPlotChart: ({ label }: { label: string }) => <div role="img" aria-label={label} />,
}));

const older: SessionSummary = {
  ...cotaSession,
  id: 1,
  car_name: "Mazda MX-5 Cup",
  started_at: "2026-11-12T10:00:00Z",
  passes: 12,
  corners: 7,
};

const sessionApi = {
  passes: () => Promise.resolve(cotaPasses),
  trace: (_: number, seq: number) => Promise.resolve(traceOf(seq)),
  reference: () => Promise.resolve(cotaReference),
};

function open(route: Route, readOnly = false) {
  render(
    <Workspace
      route={route}
      readOnly={readOnly}
      loadSessions={() => Promise.resolve([cotaSession, older])}
      sessionApi={sessionApi}
    />,
  );
}

function pending(): Promise<Response> {
  return new Promise(() => undefined);
}

describe("Workspace", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn(pending)); // core status, diagnosis, ledger
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    window.location.hash = "";
  });

  it("lists the sessions and opens the newest one on its overview", async () => {
    open({ tab: "overview" });

    const rail = await screen.findByRole("navigation", { name: "Sessions" });
    const current = within(rail).getByRole("link", { current: "page" });
    expect(current).toHaveTextContent("Formula Hybrid 2026");
    expect(current).toHaveTextContent("5 passes");
    expect(within(rail).getByRole("link", { name: /Mazda MX-5 Cup/ })).toHaveAttribute(
      "href",
      "#/sessions/1",
    );
    expect(screen.getByRole("heading", { level: 1, name: "Formula Hybrid 2026" })).toBeVisible();
    expect(await screen.findByRole("region", { name: "Zone times by lap" })).toBeVisible();
  });

  it("keeps the view when switching sessions, and marks the view on screen", async () => {
    open({ sessionId: 2, tab: "telemetry" });

    const rail = await screen.findByRole("navigation", { name: "Sessions" });
    expect(within(rail).getByRole("link", { name: /Mazda/ })).toHaveAttribute(
      "href",
      "#/sessions/1/telemetry",
    );
    const views = screen.getByRole("navigation", { name: "Views" });
    expect(within(views).getByRole("link", { current: "page" })).toHaveTextContent("Telemetry");
    expect(within(views).getAllByRole("link")).toHaveLength(6);
    expect(await screen.findByRole("navigation", { name: "Corners" })).toBeVisible();
  });

  it("says so when the session in the link does not exist", async () => {
    open({ sessionId: 9, tab: "overview" });

    expect(await screen.findByRole("alert")).toHaveTextContent("Session #9 does not exist.");
  });

  it("offers proposing an experiment, inside the experiments view", async () => {
    open({ sessionId: 2, tab: "experiments", propose: "blank" });

    expect(await screen.findByRole("region", { name: "Propose an experiment" })).toBeVisible();
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("says it is a read-only demo and offers nothing a visitor cannot do", async () => {
    open({ sessionId: 2, tab: "experiments", propose: "blank" }, true);

    expect(await screen.findByRole("note")).toHaveTextContent(
      "Read-only demo on recorded sessions",
    );
    expect(screen.queryByRole("region", { name: "Propose an experiment" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Propose an experiment" })).toBeNull();
    expect(screen.getByText(/cannot be proposed in the read-only demo/)).toBeVisible();
  });
});

describe("App", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("is the workspace, waiting for core's sessions", () => {
    vi.stubGlobal("fetch", vi.fn(pending));

    render(<App readOnly={false} />);

    expect(screen.getByText("Loading sessions…")).toBeVisible();
    expect(screen.getByRole("button", { name: "Sessions" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });
});
