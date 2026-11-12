import type { Experiment, ExperimentAction, ExperimentResult } from "./api/client";

export type State = Experiment["state"];

export const STATE_LABEL: Record<State, string> = {
  proposed: "Proposed",
  planned: "Planned",
  running: "Running",
  evaluating: "Evaluating",
  accepted: "Accepted",
  rejected: "Rejected",
  tradeoff: "Trade-off",
  inconclusive: "Inconclusive",
  reverted: "Reverted",
  aborted: "Aborted",
  discarded: "Discarded",
};

/** Most extensions core allows (experiments.MaxExtensions). Core enforces it; this hides the button. */
const MAX_EXTENSIONS = 2;

/** Before the verdict the driver must not know which code name is the baseline. */
const BLIND_STATES: readonly State[] = ["proposed", "planned", "running", "evaluating"];

export function isBlind(experiment: Experiment): boolean {
  return BLIND_STATES.includes(experiment.state);
}

/** What the experiment changes, in words. */
export function changeText(experiment: Experiment): string {
  if (experiment.kind === "recheck") return "Re-check: champion against the original baseline";
  const clicks = Math.abs(experiment.clicks);
  const direction = experiment.clicks < 0 ? "−" : "+";
  const name =
    experiment.parameter_label === "" ? experiment.parameter : experiment.parameter_label;
  return `${name} ${direction}${String(clicks)} ${clicks === 1 ? "click" : "clicks"}`;
}

/** A number with its sign, to three decimals. */
export function signed(value: number): string {
  const sign = value > 0 ? "+" : value < 0 ? "−" : "";
  return sign + Math.abs(value).toFixed(3);
}

/** An effect and its interval: "−0.045 [−0.060, −0.030]". */
export function effectText(result: ExperimentResult): string {
  return `${signed(result.estimate)} [${signed(result.ci_low)}, ${signed(result.ci_high)}]`;
}

/** One thing the driver can do with an experiment in its current state. */
export interface Step {
  label: string;
  /** What happens, said before the click. */
  hint: string;
  request: { action: ExperimentAction } | { accept: boolean };
}

const STEPS: Partial<Record<State, Step[]>> = {
  proposed: [
    {
      label: "Plan",
      hint: "Ask the engine what four blocks can detect.",
      request: { action: "plan" },
    },
    {
      label: "Discard",
      hint: "Drop the proposal; it stays in the ledger.",
      request: { action: "discard" },
    },
  ],
  planned: [
    {
      label: "Start",
      hint: "Write the run plan and both setups for the agent.",
      request: { action: "start" },
    },
    {
      label: "Discard",
      hint: "Drop the plan; it stays in the ledger.",
      request: { action: "discard" },
    },
  ],
  running: [
    {
      label: "Evaluate",
      hint: "Have the engine judge the driven laps.",
      request: { action: "evaluate" },
    },
    {
      label: "Abort",
      hint: "Stop the experiment and go back to the baseline.",
      request: { action: "abort" },
    },
  ],
  tradeoff: [
    {
      label: "Keep the variant",
      hint: "Accept the gain with its price.",
      request: { accept: true },
    },
    { label: "Back to the baseline", hint: "Decline the trade-off.", request: { accept: false } },
  ],
  inconclusive: [
    { label: "Extend", hint: "Drive two more blocks.", request: { action: "extend" } },
    {
      label: "Back to the baseline",
      hint: "Stop here; the baseline is kept.",
      request: { action: "revert" },
    },
  ],
  accepted: [
    {
      label: "Roll back",
      hint: "Make the baseline the champion again.",
      request: { action: "revert" },
    },
  ],
};

/** The steps core allows from the experiment's state. Finished experiments have none. */
export function availableSteps(experiment: Experiment): Step[] {
  const steps = STEPS[experiment.state] ?? [];
  if (experiment.state === "inconclusive" && experiment.extensions >= MAX_EXTENSIONS) {
    return steps.filter((step) => step.label !== "Extend");
  }
  return steps;
}

export function formatTime(at: string): string {
  return new Date(at).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "medium" });
}
