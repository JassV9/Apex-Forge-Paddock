import type { SensitivityEffect, SensitivityProfile } from "../api/client";
import { clicksText, signed } from "../experimentText";

export type SensitivityPrior = SensitivityProfile["priors"][number];
type Interval = Pick<SensitivityEffect, "estimate" | "ci_low" | "ci_high">;

const KNOWLEDGE: Record<SensitivityEffect["knowledge_level"], string> = {
  confirmed: "Confirmed on this car",
  preliminary: "Preliminary",
  neutral: "Neutral",
  insufficient: "Insufficient evidence",
};

const READING: Record<SensitivityEffect["effect_class"], string> = {
  negative: "lowers it",
  positive: "raises it",
  negligible: "inside the margin",
  inconclusive: "unclear",
};

const METHOD: Record<SensitivityEffect["method"], string> = {
  single: "one experiment",
  half_normal: "random effects, half-normal prior on tau",
  hksj: "random effects, HKSJ",
};

const BAR_WIDTH = 160;
const BAR_HALF = 76;

/** The largest bound of any row on a channel: every bar of that channel shares one scale. */
export function scales(rows: (Interval & { channel: string })[]): Map<string, number> {
  const largest = new Map<string, number>();
  for (const row of rows) {
    const reach = Math.max(Math.abs(row.ci_low), Math.abs(row.ci_high));
    largest.set(row.channel, Math.max(largest.get(row.channel) ?? 0, reach));
  }
  return largest;
}

/** The interval as a bar around zero. The numbers beside it say the same thing in text. */
function IntervalBar({ row, scale, kind }: { row: Interval; scale: number; kind: string }) {
  const at = (value: number) => BAR_WIDTH / 2 + (scale === 0 ? 0 : (value / scale) * BAR_HALF);
  return (
    <svg className={`interval interval-${kind}`} width={BAR_WIDTH} height="16" aria-hidden="true">
      <line className="zero" x1={BAR_WIDTH / 2} x2={BAR_WIDTH / 2} y1="1" y2="15" />
      <line x1={at(row.ci_low)} x2={at(row.ci_high)} y1="8" y2="8" />
      <circle cx={at(row.estimate)} cy="8" r="3.5" />
    </svg>
  );
}

function spanText(row: Interval): string {
  return `${signed(row.estimate)} [${signed(row.ci_low)}, ${signed(row.ci_high)}]`;
}

function where(row: { channel: string; corner_class: string; phase: string }): string {
  const phase = row.phase === "corner" ? "whole corner" : `${row.phase} phase`;
  return `${row.corner_class} corners, ${phase}`;
}

/** What the row was measured under: the experiments it pools never mix these. */
export function stratumText(effect: SensitivityEffect): string {
  const driver = effect.driver === "ai" ? `AI, hint ${effect.hint_bucket}` : "a person";
  const base = effect.baseline_value === undefined ? "" : `, from ${String(effect.baseline_value)}`;
  return `${effect.track_name || effect.track}, ${driver}${base}`;
}

/** Where the number comes from: the experiments, the method and the engines. */
export function provenanceText(effect: SensitivityEffect): string {
  const count = effect.experiments;
  const ids = effect.experiment_ids.map(String).join(", ");
  const engines = effect.engine_versions.length
    ? `engine ${effect.engine_versions.join(", ")}`
    : "ledger numbers, no engine version";
  const tau = effect.tau === undefined ? "" : `, tau ${effect.tau.toFixed(4)}`;
  const label = count === 1 ? "experiment" : "experiments";
  return `${String(count)} ${label} (${ids}), ${METHOD[effect.method]}${tau}; ${engines}; ${String(effect.n_passes)} passes`;
}

interface EffectRowProps {
  effect: SensitivityEffect;
  label: string;
  scale: number;
}

export function EffectRow({ effect, label, scale }: EffectRowProps) {
  return (
    <tr className={effect.stale ? "stale" : undefined}>
      <td>
        {label} {clicksText(effect.clicks)}
      </td>
      <td>
        <code>{effect.channel}</code>, {where(effect)}
      </td>
      <td>{stratumText(effect)}</td>
      <td>{spanText(effect)}</td>
      <td>
        <IntervalBar row={effect} scale={scale} kind={effect.effect_class} />
      </td>
      <td>
        <strong>{KNOWLEDGE[effect.knowledge_level]}</strong>: {READING[effect.effect_class]}
        {effect.nonlinear && <span className="badge">non-linear</span>}
        {effect.stale && <span className="badge">stale</span>}
      </td>
      <td>
        {provenanceText(effect)}
        {effect.note && <p className="note">{effect.note}</p>}
      </td>
    </tr>
  );
}

interface PriorRowProps {
  prior: SensitivityPrior;
  label: string;
  scale: number;
}

export function PriorRow({ prior, label, scale }: PriorRowProps) {
  return (
    <tr className="prior">
      <td>
        {label} {clicksText(prior.clicks)}
      </td>
      <td>
        <code>{prior.channel}</code>, {where(prior)}
      </td>
      <td>{spanText(prior)}</td>
      <td>
        <IntervalBar row={prior} scale={scale} kind="prior" />
      </td>
      <td>
        Prior from {prior.source_car}, not measured on this car ({String(prior.source_experiments)}{" "}
        {prior.source_experiments === 1 ? "experiment" : "experiments"} there, kept at{" "}
        {String(prior.weight)})
      </td>
    </tr>
  );
}
