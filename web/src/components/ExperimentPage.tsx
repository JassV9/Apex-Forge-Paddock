import { useCallback, useEffect, useState } from "react";

import {
  actOnExperiment,
  decideExperiment,
  type Experiment,
  type ExperimentAction,
  fetchExperiment,
} from "../api/client";
import { availableSteps, changeText, type Step } from "../experimentText";
import {
  HistorySection,
  PlanSection,
  ResultsSection,
  SetupsSection,
  TestSection,
  VerdictSection,
} from "./ExperimentDetails";
import { StateBadge } from "./Ledger";

/** The calls the page makes; injected in tests, the real client by default. */
export interface ExperimentApi {
  load: (id: number) => Promise<Experiment>;
  act: (id: number, action: ExperimentAction) => Promise<Experiment>;
  decide: (id: number, accept: boolean) => Promise<Experiment>;
}

const coreApi: ExperimentApi = {
  load: fetchExperiment,
  act: actOnExperiment,
  decide: decideExperiment,
};

interface ExperimentPageProps {
  id: number;
  api?: ExperimentApi;
}

function send(api: ExperimentApi, id: number, step: Step): Promise<Experiment> {
  return "action" in step.request
    ? api.act(id, step.request.action)
    : api.decide(id, step.request.accept);
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "The request failed.";
}

interface StepsProps {
  experiment: Experiment;
  busy: boolean;
  onStep: (step: Step) => void;
  onRefresh: () => void;
}

function Steps({ experiment, busy, onStep, onRefresh }: StepsProps) {
  return (
    <section aria-label="Next steps" className="steps">
      {availableSteps(experiment).map((step) => (
        <button
          key={step.label}
          title={step.hint}
          disabled={busy}
          onClick={() => {
            onStep(step);
          }}
        >
          {step.label}
        </button>
      ))}
      <button className="quiet" disabled={busy} onClick={onRefresh}>
        Refresh
      </button>
    </section>
  );
}

/** One experiment: its test, setups, run plan, results, verdict and every state it was in. */
export function ExperimentPage({ id, api = coreApi }: ExperimentPageProps) {
  const [experiment, setExperiment] = useState<Experiment | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);

  const run = useCallback((request: Promise<Experiment>) => {
    setBusy(true);
    request
      .then((next) => {
        setExperiment(next);
        setProblem("");
      })
      .catch((error: unknown) => {
        setProblem(messageOf(error));
      })
      .finally(() => {
        setBusy(false);
      });
  }, []);

  useEffect(() => {
    let cancelled = false;
    api.load(id).then(
      (loaded) => {
        if (!cancelled) setExperiment(loaded);
      },
      (error: unknown) => {
        if (!cancelled) setProblem(messageOf(error));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, id]);

  if (experiment === null) {
    return problem === "" ? <p>Loading the experiment…</p> : <p role="alert">{problem}</p>;
  }
  return (
    <article>
      <h2>
        Experiment #{experiment.id} <StateBadge state={experiment.state} />
      </h2>
      <p>
        {experiment.car_name}, {experiment.track_name}. {changeText(experiment)}.
      </p>
      <blockquote>{experiment.statement}</blockquote>
      <Steps
        experiment={experiment}
        busy={busy}
        onStep={(step) => {
          run(send(api, id, step));
        }}
        onRefresh={() => {
          run(api.load(id));
        }}
      />
      {problem !== "" && <p role="alert">{problem}</p>}
      <TestSection experiment={experiment} />
      <SetupsSection experiment={experiment} />
      <PlanSection experiment={experiment} />
      <ResultsSection experiment={experiment} />
      <VerdictSection experiment={experiment} />
      <HistorySection experiment={experiment} />
    </article>
  );
}
