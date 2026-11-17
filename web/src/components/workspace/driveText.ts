import type { Experiment, TrainingProgram } from "../../api/client";
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
  /** The test copy to pick instead, when the layout picked is an original that has one. */
  testCopy?: string;
  /** Your laps here are compared with the AI's. */
  aiCanDrive: boolean;
}

/**
 * What picking a layout means for a person driving it. The AI drives, and so can be compared
 * with, only the Paddock test copy of a layout (docs/adr/0014-ghost-driver.md); the original is
 * driven and recorded all the same.
 */
export function layoutHint(track: LauncherTrack, layout: LauncherLayout): LayoutHint {
  if (layout.ai_can_drive) {
    return {
      text: "Test copy: your laps here are compared with the AI's, corner by corner.",
      aiCanDrive: true,
    };
  }
  const original = "Original layout: your laps are recorded, but not compared with the AI.";
  if (layout.test_copy !== "") {
    return {
      text: `${original} The AI drives only the test copy, ${layout.test_copy}; pick that to be compared.`,
      testCopy: layout.test_copy,
      aiCanDrive: false,
    };
  }
  if (layout.layout === "") {
    return {
      text: `${original} This track has no layout folder to make a test copy of, so it never can be.`,
      aiCanDrive: false,
    };
  }
  return {
    text: `${original} The AI drives only a test copy, which does not exist yet. Make it, then reload:`,
    command: `python tools/prepare_test_layout.py ${track.id} ${layout.layout}`,
    aiCanDrive: false,
  };
}

/** A layout's line in the picker. */
export function layoutLabel(layout: LauncherLayout): string {
  return layout.ai_can_drive ? `${layout.name} · compared with the AI` : layout.name;
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
    return "Waiting for the last start: the game has not opened and closed yet (the launcher gives up waiting for it to open after 3 minutes).";
  }
  return undefined;
}

/** Experiments in these states have their setups on the car (core's `occupying`). */
const ON_THE_CAR: readonly Experiment["state"][] = [
  "running",
  "evaluating",
  "tradeoff",
  "inconclusive",
];

function sameCar(a: string, b: string): boolean {
  return a.trim().toLowerCase() === b.trim().toLowerCase();
}

/**
 * What is open on a car that a person starting the game should know before driving: an
 * experiment for the AI (the AI does not take the car by itself, and the person's laps are not
 * counted in it), a training program (it waits), an experiment the person drives (its blocks).
 */
export function openWorkNotes(
  experiments: Experiment[],
  programs: TrainingProgram[],
  carName: string,
): string[] {
  const notes: string[] = [];
  for (const e of experiments) {
    if (!sameCar(e.car_name, carName) || !ON_THE_CAR.includes(e.state)) continue;
    notes.push(
      e.driver === "ai"
        ? `Experiment #${String(e.id)} is open on this car for the AI to drive. The game starts with the car in your hands: the AI does not start by itself, and your laps are not counted in it. In the game's Paddock window, "Let the AI drive this plan" hands the car over; to drop the experiment, abort it under Experiments.`
        : `Experiment #${String(e.id)} is open on this car for you to drive. Stop in the pits and the game's Paddock window names its next block; laps count towards it only on its setup.`,
    );
  }
  for (const p of programs) {
    if (!sameCar(p.car_name, carName) || (p.state !== "running" && p.state !== "waiting")) continue;
    notes.push(
      `An AI training program is ${p.state} on this car. Its experiments are for the AI to drive and the AI does not take the car by itself, so the program waits while you drive. "Stop training" under Experiments (or in the game) ends it.`,
    );
  }
  return notes;
}
