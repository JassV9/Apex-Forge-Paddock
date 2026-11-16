import type {
  ControlsCheck,
  LauncherContent,
  LauncherLayout,
  LauncherStatus,
  LauncherTrack,
} from "../../api/launcher";
import { timeOf } from "../../format";

/** How to start the launcher, shown when the dashboard cannot reach it. */
export const START_LAUNCHER = "make launcher";
export const START_LAUNCHER_BY_HAND = "cd core && go run ./cmd/paddock-launcher";

/** A track and layout as one picker value; "|" cannot occur in a Windows folder name. */
export function layoutKey(trackId: string, layout: string): string {
  return `${trackId}|${layout}`;
}

export function parseLayoutKey(key: string): { trackId: string; layout: string } {
  const [trackId = "", layout = ""] = key.split("|");
  return { trackId, layout };
}

export function findLayout(
  content: LauncherContent,
  key: string,
): { track: LauncherTrack; layout: LauncherLayout } | undefined {
  const { trackId, layout } = parseLayoutKey(key);
  const track = content.tracks.find((t) => t.id === trackId);
  const found = track?.layouts.find((l) => l.layout === layout);
  return track === undefined || found === undefined ? undefined : { track, layout: found };
}

export interface LayoutHint {
  text: string;
  /** The command that makes a test copy, when one is missing. */
  command?: string;
  aiCanDrive: boolean;
}

/** Whether the AI test driver may drive on a layout, and what to do when it may not. */
export function layoutHint(track: LauncherTrack, layout: LauncherLayout): LayoutHint {
  if (layout.ai_can_drive) return { text: "AI can drive here.", aiCanDrive: true };
  if (layout.test_copy !== "") {
    return {
      text: `AI cannot drive here; its test copy ${layout.test_copy} can.`,
      aiCanDrive: false,
    };
  }
  if (layout.layout === "") {
    return {
      text: "AI cannot drive here: this track has no layout folder to make a test copy of.",
      aiCanDrive: false,
    };
  }
  return {
    text: "AI cannot drive here: run",
    command: `python tools/prepare_test_layout.py ${track.id} ${layout.layout}`,
    aiCanDrive: false,
  };
}

/** A layout's line in the picker. */
export function layoutLabel(layout: LauncherLayout): string {
  return layout.ai_can_drive ? `${layout.name} · AI can drive` : layout.name;
}

function controlsText(check: ControlsCheck): string {
  const at = timeOf(check.at);
  switch (check.state) {
    case "watching":
      return "Wheel bindings backed up; they are checked when the game exits.";
    case "unchanged":
      return `Wheel bindings unchanged after the game (${at}).`;
    case "restored":
      return `Wheel bindings restored after the game changed them (${at}): ${check.detail ?? ""}`;
    case "restore_failed":
      return `Wheel bindings changed and could not be restored (${at}): ${check.detail ?? ""}. The backup is in Documents/Assetto Corsa/paddock/controls_backup.`;
  }
}

/** The launcher's state in words, newest first. */
export function statusLines(status: LauncherStatus, carName: (id: string) => string): string[] {
  const lines: string[] = [];
  if (status.game_running) lines.push("Assetto Corsa is running.");
  const last = status.last_launch;
  if (last !== undefined) {
    const where = last.layout === "" ? last.track_id : `${last.track_id} · ${last.layout}`;
    lines.push(`Last start: ${carName(last.car_id)} at ${where}, ${timeOf(last.at)}.`);
  }
  if (status.controls !== undefined) lines.push(controlsText(status.controls));
  return lines;
}

/** Why Start driving is greyed out, or undefined when it can be pressed. */
export function blockedReason(status: LauncherStatus): string | undefined {
  if (status.game_running) return "Leave the game to start another session.";
  if (status.controls?.state === "watching") {
    return "Waiting for the last start: the game has not opened and closed yet.";
  }
  return undefined;
}
