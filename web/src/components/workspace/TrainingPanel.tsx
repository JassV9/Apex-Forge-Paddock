import { useCallback, useEffect, useState } from "react";

import {
  fetchTraining,
  type SessionSummary,
  startTraining,
  stopTraining,
  type TrainingProgram,
} from "../../api/client";

/** How many experiments a program asks for by default: an evening of AI laps. */
const DEFAULT_EXPERIMENTS = 4;
/** A running program changes every few minutes; look again this often. */
const REFRESH_MS = 15_000;

interface TrainingPanelProps {
  session: SessionSummary;
  readOnly: boolean;
  /** Injected in tests; the real client by default. */
  api?: {
    list: () => Promise<TrainingProgram[]>;
    start: (sessionId: number, experiments: number) => Promise<TrainingProgram>;
    stop: (id: number) => Promise<TrainingProgram>;
  };
  refreshMs?: number;
}

const realApi = { list: fetchTraining, start: startTraining, stop: stopTraining };

function isOpen(program: TrainingProgram): boolean {
  return program.state === "running" || program.state === "waiting";
}

function stepText(step: TrainingProgram["steps"][number]): string {
  const clicks = step.clicks > 0 ? `+${String(step.clicks)}` : String(step.clicks);
  return `#${String(step.experiment_id)} ${step.parameter} ${clicks}: ${step.state}`;
}

function ProgramView({ program, onStop }: { program: TrainingProgram; onStop?: () => void }) {
  return (
    <div className="training-program">
      <p>
        <strong>{program.state}</strong> · {program.steps.length} of {program.target} experiments ·{" "}
        {program.message}
      </p>
      {program.steps.length > 0 && (
        <ul className="training-steps">
          {program.steps.map((step) => (
            <li key={step.experiment_id}>{stepText(step)}</li>
          ))}
        </ul>
      )}
      {onStop !== undefined && (
        <button type="button" onClick={onStop}>
          Stop training
        </button>
      )}
    </div>
  );
}

/**
 * Train the AI on this session's car: a queue of AI-driven experiments, each judged when its
 * laps are in (docs/adr/0020-ai-training-program.md).
 */
export function TrainingPanel({
  session,
  readOnly,
  api = realApi,
  refreshMs = REFRESH_MS,
}: TrainingPanelProps) {
  const [programs, setPrograms] = useState<TrainingProgram[] | null>(null);
  const [error, setError] = useState("");
  const [experiments, setExperiments] = useState(DEFAULT_EXPERIMENTS);

  const reload = useCallback(() => {
    api.list().then(setPrograms, () => {
      setError("Training programs could not be loaded.");
    });
  }, [api]);

  useEffect(() => {
    reload();
    const timer = window.setInterval(reload, refreshMs);
    return () => {
      window.clearInterval(timer);
    };
  }, [reload, refreshMs]);

  const act = (work: Promise<TrainingProgram>) => {
    setError("");
    work.then(reload, (err: unknown) => {
      setError(err instanceof Error ? err.message : String(err));
    });
  };

  const mine = (programs ?? []).filter((p) => p.car_name === session.car_name);
  const open = mine.find(isOpen);
  const last = mine.find((p) => !isOpen(p));
  return (
    <section className="card" aria-label="Train the AI">
      <h3>Train the AI · {session.car_name}</h3>
      <p className="note">
        The AI drives this session's best-ranked experiments one after another on its calibrated
        grip, and each is judged when its laps are in. Calibrate it in the game first.
      </p>
      {error !== "" && <p role="alert">{error}</p>}
      {open !== undefined && (
        <ProgramView
          program={open}
          {...(readOnly
            ? {}
            : {
                onStop: () => {
                  act(api.stop(open.id));
                },
              })}
        />
      )}
      {open === undefined && !readOnly && (
        <form
          className="training-start"
          onSubmit={(event) => {
            event.preventDefault();
            act(api.start(session.id, experiments));
          }}
        >
          <label>
            Experiments{" "}
            <input
              type="number"
              min={1}
              max={20}
              value={experiments}
              onChange={(event) => {
                setExperiments(Number(event.target.value));
              }}
            />
          </label>
          <button type="submit">Start training</button>
        </form>
      )}
      {open === undefined && last !== undefined && <ProgramView program={last} />}
    </section>
  );
}
