import type { Experiment } from "./api/client";

type Verdict = NonNullable<Experiment["verdict"]>;
export type FindingClass = NonNullable<Verdict["finding_class"]>;
export type Recommendation = NonNullable<Verdict["recommendation"]>;
type Confounder = NonNullable<Verdict["confounders"]>[number];

/** What the experiment found, in plain words (docs/adr/0029-block-level-inference-and-finding-classes.md). */
export const FINDING_LABEL: Record<FindingClass, string> = {
  invalid: "Invalid: the experiment cannot be judged",
  confirmed_improvement: "Confirmed improvement",
  confirmed_regression: "Confirmed regression",
  neutral: "No meaningful effect (neutral)",
  context_dependent: "Depends on the corner: better in some, worse in others",
  likely_improvement: "Probably better, not yet by enough to be sure",
  likely_regression: "Probably worse",
  insufficient_evidence: "Not enough evidence either way",
};

/** What to do next, in plain words. */
export const RECOMMENDATION_LABEL: Record<Recommendation, string> = {
  continue: "Drive two more blocks: the next look may settle it.",
  larger_perturbation:
    "Try a bigger step of the same setting: the effect is small but points one way.",
  investigate_context: "Look at the speed classes: the change acts differently in them.",
  switch_metric: "Judge it on the car's response instead: the time did not move, the car did.",
  stop_neutral: "Stop: there is nothing worth having to gain.",
  stop_invalid: "Stop: fix what made the experiment invalid and drive it again.",
};

const CONFOUNDER_LABEL: Record<Confounder["kind"], string> = {
  covariate_left_out: "not adjusted for",
  covariate_between_blocks: "changes between blocks",
  driver_input_shifted: "driver input shifted",
  session_tangled: "game sessions tangled with the setups",
  setup_mismatch: "passes on another setup left out",
  source_mismatch: "passes from another driver left out",
  speed_class_interaction: "effect differs between",
};

export function confounderText(confounder: Confounder): string {
  const count = confounder.count === undefined ? "" : ` (${String(confounder.count)})`;
  return `${CONFOUNDER_LABEL[confounder.kind]}: ${confounder.name}${count}`;
}

/** The look the verdict was judged at, and how sure a reader can be of it. */
export function lookText(verdict: Verdict): string {
  const parts: string[] = [];
  if (verdict.look !== undefined) {
    parts.push(
      verdict.look === 0
        ? "Judged before the first look (fewer than 8 blocks)."
        : `Judged at look ${String(verdict.look)} of 3.`,
    );
  }
  if (verdict.flags?.includes("uninformative") === true) {
    parts.push("The interval is too wide to say much.");
  }
  if (verdict.finding_derived === true) {
    parts.push("Read from a verdict judged before findings existed, by an older method.");
  }
  return parts.join(" ");
}
