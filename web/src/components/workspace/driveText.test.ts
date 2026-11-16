import { describe, expect, it } from "vitest";

import type { LauncherStatus } from "../../api/launcher";
import { blockedReason, layoutHint, layoutKey, parseLayoutKey } from "./driveText";

const idle: LauncherStatus = { service: "launcher", version: "dev", game_running: false };

describe("driveText", () => {
  it.each([
    ["fn_cota", "gp_paddock"],
    ["magione", ""],
  ])("round-trips %s / %s through the picker value", (trackId, layout) => {
    expect(parseLayoutKey(layoutKey(trackId, layout))).toEqual({ trackId, layout });
  });

  it("says a single-layout track cannot get a test copy", () => {
    const hint = layoutHint(
      { id: "magione", name: "Magione", layouts: [] },
      { layout: "", name: "Magione", ai_can_drive: false, test_copy: "" },
    );
    expect(hint.aiCanDrive).toBe(false);
    expect(hint.command).toBeUndefined();
    expect(hint.text).toMatch(/no layout folder/);
  });

  it.each<[string, LauncherStatus, string | undefined]>([
    ["idle", idle, undefined],
    ["game running", { ...idle, game_running: true }, "Leave the game to start another session."],
    [
      "last start not finished",
      { ...idle, controls: { state: "watching", at: "2026-11-16T18:00:00Z", backup_sha256: "a" } },
      "Waiting for the last start: the game has not opened and closed yet.",
    ],
    [
      "bindings checked",
      { ...idle, controls: { state: "unchanged", at: "2026-11-16T18:00:00Z", backup_sha256: "a" } },
      undefined,
    ],
  ])("blocks a start when %s", (_, status, reason) => {
    expect(blockedReason(status)).toBe(reason);
  });
});
