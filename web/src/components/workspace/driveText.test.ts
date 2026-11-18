import { describe, expect, it } from "vitest";

import type { Experiment, TrainingProgram } from "../../api/client";
import type { LauncherStatus } from "../../api/launcher";
import { blockedReason, layoutHint, layoutKey, openWorkNotes, parseLayoutKey } from "./driveText";

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
      expect.stringContaining("Waiting for the last start: the game has not opened and closed yet"),
    ],
    [
      "bindings checked",
      { ...idle, controls: { state: "unchanged", at: "2026-11-16T18:00:00Z", backup_sha256: "a" } },
      undefined,
    ],
  ])("blocks a start when %s", (_, status, reason) => {
    expect(blockedReason(status)).toEqual(reason);
  });

  it("tells a driver on an original layout: compared in the web app, never shown in the game", () => {
    const track = { id: "fn_cota", name: "COTA", layouts: [] };
    const original = { layout: "gp", name: "GP", ai_can_drive: false, test_copy: "" };

    const hint = layoutHint(track, original);

    expect(hint.aiCanDrive).toBe(false);
    expect(hint.text).toContain("no AI times in the game");
    expect(hint.text).toContain(
      "In the web app your laps are compared with the AI's on its test copy",
    );
    expect(hint.text).not.toContain("not compared");
    expect(hint.command).toBe("python tools/prepare_test_layout.py fn_cota gp");
    expect(hint.testCopy).toBeUndefined();
    expect(layoutHint(track, { ...original, test_copy: "gp_paddock" }).testCopy).toBe("gp_paddock");
  });
});

const experiment = (fields: Partial<Experiment>) =>
  ({ id: 1, car_name: "Ferrari 488 GT3", driver: "ai", state: "running", ...fields }) as Experiment;
const program = (fields: Partial<TrainingProgram>) =>
  ({ id: 1, car_name: "Ferrari 488 GT3", state: "running", ...fields }) as TrainingProgram;

describe("openWorkNotes", () => {
  it("names an open AI experiment, a human one and a training program of the car", () => {
    const notes = openWorkNotes(
      [experiment({ id: 3 }), experiment({ id: 4, driver: "human", state: "inconclusive" })],
      [program({ state: "waiting" })],
      "ferrari 488 gt3 ",
    );

    expect(notes).toHaveLength(3);
    expect(notes[0]).toContain("#3 is open on this car for the AI to drive");
    expect(notes[1]).toContain("#4 is open on this car for you to drive");
    expect(notes[2]).toContain("training program is waiting");
  });

  it.each(["proposed", "planned", "accepted", "rejected", "reverted", "aborted", "discarded"])(
    "ignores an experiment that is %s: nothing of it is on the car",
    (state) => {
      expect(
        openWorkNotes([experiment({ state: state as Experiment["state"] })], [], "Ferrari 488 GT3"),
      ).toEqual([]);
    },
  );

  it("ignores other cars and finished programs", () => {
    expect(
      openWorkNotes(
        [experiment({ car_name: "Mazda MX-5" })],
        [program({ state: "done" })],
        "Ferrari 488 GT3",
      ),
    ).toEqual([]);
  });
});
