import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { LauncherContent, LauncherStatus } from "../../api/launcher";
import { DrivePanel } from "./DrivePanel";

const content: LauncherContent = {
  ac_root: "C:/ac",
  cars: [{ id: "ks_ferrari_488_gt3", name: "Ferrari 488 GT3" }],
  tracks: [
    {
      id: "fn_cota",
      name: "Circuit of The Americas",
      layouts: [
        { layout: "gp", name: "COTA GP", ai_can_drive: false, test_copy: "" },
        { layout: "f1", name: "COTA F1", ai_can_drive: false, test_copy: "f1_paddock" },
        { layout: "f1_paddock", name: "COTA F1 (Paddock test)", ai_can_drive: true, test_copy: "" },
      ],
    },
  ],
};

const idle: LauncherStatus = { service: "launcher", version: "dev", game_running: false };

interface Reply {
  status: number;
  body: unknown;
}

/** Stubs fetch with the launcher's answers by method and path; records POST bodies. */
function stubLauncher(routes: Record<string, Reply | (() => Reply)>) {
  const posted: unknown[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const key = `${request.method} ${new URL(request.url).pathname}`;
    if (request.method === "POST") posted.push(await request.json());
    const route = routes[key];
    if (route === undefined) throw new TypeError("Failed to fetch");
    const reply = typeof route === "function" ? route() : route;
    return new Response(JSON.stringify(reply.body), {
      status: reply.status,
      headers: { "Content-Type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  return { posted, fetchMock };
}

function ok(body: unknown): Reply {
  return { status: 200, body };
}

describe("DrivePanel", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("explains how to start the launcher when it cannot be reached", async () => {
    stubLauncher({});
    render(<DrivePanel pollMs={60_000} />);

    expect(await screen.findByText(/The launcher is not running/)).toBeVisible();
    expect(screen.getByText("make launcher")).toBeVisible();
    expect(screen.getByText("cd core && go run ./cmd/paddock-launcher")).toBeVisible();
  });

  it("says where the AI can drive, and how to make a test copy", async () => {
    stubLauncher({ "GET /status": ok(idle), "GET /content": ok(content) });
    render(<DrivePanel pollMs={60_000} />);

    const track = await screen.findByLabelText("Track");
    expect(
      screen.getByRole("option", { name: "COTA F1 (Paddock test) · AI can drive" }),
    ).toBeTruthy();

    fireEvent.change(track, { target: { value: "fn_cota|f1_paddock" } });
    expect(screen.getByText("AI can drive here.")).toBeVisible();

    fireEvent.change(track, { target: { value: "fn_cota|f1" } });
    expect(screen.getByText("AI cannot drive here; its test copy f1_paddock can.")).toBeVisible();

    fireEvent.change(track, { target: { value: "fn_cota|gp" } });
    expect(screen.getByText("python tools/prepare_test_layout.py fn_cota gp")).toBeVisible();
  });

  it("starts driving with the picked car and layout and shows the launcher's answer", async () => {
    const watching: LauncherStatus = {
      ...idle,
      last_launch: {
        car_id: "ks_ferrari_488_gt3",
        track_id: "fn_cota",
        layout: "f1_paddock",
        at: "2026-11-16T18:30:00Z",
      },
      controls: { state: "watching", at: "2026-11-16T18:30:00Z", backup_sha256: "ab" },
    };
    const { posted } = stubLauncher({
      "GET /status": ok(idle),
      "GET /content": ok(content),
      "POST /launch": { status: 202, body: watching },
    });
    render(<DrivePanel pollMs={60_000} />);

    const car = await screen.findByLabelText("Car");
    const start = screen.getByRole("button", { name: "Start driving" });
    expect(start).toBeDisabled();
    fireEvent.change(car, { target: { value: "ks_ferrari_488_gt3" } });
    fireEvent.change(screen.getByLabelText("Track"), { target: { value: "fn_cota|f1_paddock" } });
    fireEvent.click(start);

    expect(await screen.findByText(/Wheel bindings backed up/)).toBeVisible();
    expect(posted).toEqual([
      { car_id: "ks_ferrari_488_gt3", track_id: "fn_cota", layout: "f1_paddock" },
    ]);
    expect(screen.getByText(/Last start: Ferrari 488 GT3 at fn_cota · f1_paddock/)).toBeVisible();
    expect(screen.getByRole("button", { name: "Start driving" })).toBeDisabled();
  });

  it("shows what the launcher refused", async () => {
    const last = {
      car_id: "ks_ferrari_488_gt3",
      track_id: "fn_cota",
      layout: "gp",
      at: "2026-11-16T18:00:00Z",
    };
    stubLauncher({
      "GET /status": ok({ ...idle, last_launch: last }),
      "GET /content": ok(content),
      "POST /launch": { status: 409, body: { error: "Assetto Corsa is already running" } },
    });
    render(<DrivePanel pollMs={60_000} />);

    fireEvent.click(await screen.findByRole("button", { name: "Start driving" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Assetto Corsa is already running");
  });

  it("reports bindings restored after the game, and a running game blocks a start", async () => {
    stubLauncher({
      "GET /status": ok({
        ...idle,
        game_running: true,
        controls: {
          state: "restored",
          at: "2026-11-16T19:00:00Z",
          backup_sha256: "ab",
          detail: "CONTROLLERS/CON1",
        },
      }),
      "GET /content": ok(content),
    });
    render(<DrivePanel pollMs={60_000} />);

    expect(await screen.findByText(/restored after the game changed them/)).toHaveTextContent(
      "CONTROLLERS/CON1",
    );
    expect(screen.getByText("Assetto Corsa is running.")).toBeVisible();
    expect(screen.getByText("Leave the game to start another session.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Start driving" })).toBeDisabled();
  });
});
