import { useCallback } from "react";

import type { Diagnosis, Proposal, Proposals } from "../api/client";
import { clicksText, signed } from "../experimentText";
import { useLoaded } from "../hooks/useLoaded";
import { type Prefill, proposeHref } from "../hooks/useRoute";

interface ProposalsSectionProps {
  sessionId: number;
  diagnosis: Diagnosis;
  /** The ranking of the session's candidates; null when core has none. */
  load: (sessionId: number) => Promise<Proposals | null>;
}

const STAGE: Record<Proposals["stage"], string> = {
  cold_start:
    "Fewer than 5 judged experiments on this car: the order leans on rules of thumb and on priors from other cars.",
  learning: "5 to 19 judged experiments on this car: its own measurements are taking over.",
  optimizing: "20 or more judged experiments on this car.",
};

function basisText(proposal: Proposal): string {
  if (proposal.basis === "measured") return "measured on this car";
  if (proposal.basis === "prior") {
    return `a prior from ${proposal.prior_source}, not measured on this car`;
  }
  return "nothing is known about it yet";
}

/** The predicted gain to start the form from: only when the whole interval is a gain. */
function gainOf(proposal: Proposal): Prefill["gain"] {
  if (proposal.gain_low <= 0) return undefined;
  return { low: Number(proposal.gain_low.toFixed(3)), high: Number(proposal.gain_high.toFixed(3)) };
}

function prefillOf(proposal: Proposal, sessionId: number, diagnosis: Diagnosis): Prefill {
  const key = proposal.hypotheses[0] ?? "";
  const tested = diagnosis.hypotheses.find((hypothesis) => hypothesis.key === key);
  const gain = gainOf(proposal);
  return {
    sessionId,
    parameter: proposal.parameter,
    clicks: proposal.clicks,
    hypothesisKey: key,
    statement: `${tested?.statement ?? ""} Test: ${proposal.why}.`.trim(),
    ...(gain === undefined ? {} : { gain }),
  };
}

function nameOf(candidate: { label: string; parameter: string; clicks: number }): string {
  const name = candidate.label === "" ? candidate.parameter : candidate.label;
  return `${name} ${clicksText(candidate.clicks)}`;
}

function RankedItem({ proposal, href }: { proposal: Proposal; href: string }) {
  return (
    <li>
      <strong>{nameOf(proposal)}</strong>: expected gain {proposal.expected_gain.toFixed(3)} s per
      corner pass (between {signed(proposal.gain_low)} and {signed(proposal.gain_high)}), about{" "}
      {proposal.laps_needed} laps to decide. Basis: {basisText(proposal)}.{" "}
      <a href={href}>Propose this</a>
    </li>
  );
}

/** Which of the diagnosis's experiments to run first, and which the car already answered. */
export function ProposalsSection({ sessionId, diagnosis, load }: ProposalsSectionProps) {
  const loadThis = useCallback(() => load(sessionId), [load, sessionId]);
  const state = useLoaded(loadThis);

  if (state.kind === "loading") return null;
  if (state.kind === "failed") return <p role="alert">The ranking could not be loaded.</p>;
  const proposals = state.value;
  if (proposals === null || proposals.ranked.length + proposals.skipped.length === 0) return null;
  return (
    <section aria-label="What to test next">
      <h3>What to test next</h3>
      <p className="note">
        Ordered by expected gain per lap of driving. An unknown item earns a look; one this car
        showed to do nothing does not. {STAGE[proposals.stage]}
      </p>
      <ol>
        {proposals.ranked.map((proposal) => (
          <RankedItem
            key={`${proposal.parameter} ${String(proposal.clicks)}`}
            proposal={proposal}
            href={proposeHref(prefillOf(proposal, sessionId, diagnosis))}
          />
        ))}
      </ol>
      {proposals.skipped.length > 0 && (
        <details>
          <summary>Already answered on this car ({proposals.skipped.length})</summary>
          <ul>
            {proposals.skipped.map((skipped) => (
              <li key={`${skipped.parameter} ${String(skipped.clicks)}`}>
                {nameOf(skipped)}: {skipped.reason}
              </li>
            ))}
          </ul>
        </details>
      )}
    </section>
  );
}
