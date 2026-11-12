import { useEffect, useState } from "react";

import {
  type Diagnosis,
  fetchDiagnosis,
  fetchProposals,
  type Proposals,
  runDiagnosis,
} from "../api/client";
import { ExplanationSection, HypothesesSection } from "./DiagnosisDetails";
import { ProposalsSection } from "./ProposalsSection";

/** The calls the page makes; injected in tests, the real client by default. */
export interface DiagnosisApi {
  load: (sessionId: number) => Promise<Diagnosis | null>;
  run: (sessionId: number) => Promise<Diagnosis>;
  proposals: (sessionId: number) => Promise<Proposals | null>;
}

const coreApi: DiagnosisApi = {
  load: fetchDiagnosis,
  run: runDiagnosis,
  proposals: fetchProposals,
};

interface DiagnosisPageProps {
  sessionId: number;
  api?: DiagnosisApi;
  /** The public demo: the stored diagnosis is shown and cannot be run again. */
  readOnly?: boolean;
}

type State = { kind: "loading" } | { kind: "none" } | { kind: "ready"; diagnosis: Diagnosis };

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "The request failed.";
}

/** What a session's passes say about the car: hypotheses to test, never a fix to apply. */
export function DiagnosisPage({ sessionId, api = coreApi, readOnly = false }: DiagnosisPageProps) {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  // A new diagnosis may name other experiments: the ranking is loaded again.
  const [runs, setRuns] = useState(0);

  useEffect(() => {
    let cancelled = false;
    api.load(sessionId).then(
      (diagnosis) => {
        if (!cancelled)
          setState(diagnosis === null ? { kind: "none" } : { kind: "ready", diagnosis });
      },
      (error: unknown) => {
        if (!cancelled) {
          setState({ kind: "none" });
          setProblem(messageOf(error));
        }
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, sessionId]);

  const diagnose = () => {
    setBusy(true);
    api
      .run(sessionId)
      .then((diagnosis) => {
        setState({ kind: "ready", diagnosis });
        setProblem("");
        setRuns((count) => count + 1);
      })
      .catch((error: unknown) => {
        setProblem(messageOf(error));
      })
      .finally(() => {
        setBusy(false);
      });
  };

  if (state.kind === "loading") return <p>Loading the diagnosis…</p>;
  return (
    <article>
      <h2>Session #{sessionId}: diagnosis</h2>
      {state.kind === "none" && <p>This session has not been diagnosed yet.</p>}
      {!readOnly && (
        <section className="steps">
          <button disabled={busy} onClick={diagnose}>
            {state.kind === "ready" ? "Diagnose again" : "Diagnose this session"}
          </button>
        </section>
      )}
      {problem !== "" && <p role="alert">{problem}</p>}
      {state.kind === "ready" && (
        <>
          <ExplanationSection diagnosis={state.diagnosis} />
          <ProposalsSection
            key={runs}
            sessionId={sessionId}
            diagnosis={state.diagnosis}
            load={api.proposals}
          />
          <HypothesesSection diagnosis={state.diagnosis} sessionId={sessionId} />
        </>
      )}
    </article>
  );
}
