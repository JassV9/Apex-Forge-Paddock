import type { SessionSummary } from "../../api/client";
import { experimentHref, type Prefill, proposeHref, sessionHref } from "../../hooks/useRoute";
import { BaseSetupPanel } from "../BaseSetupPanel";
import { ExperimentPage } from "../ExperimentPage";
import { Ledger } from "../Ledger";
import { ProposeForm } from "../ProposeForm";
import { Scorecard } from "../Scorecard";

interface ExperimentsPanelProps {
  session: SessionSummary;
  experimentId: number | undefined;
  propose: Prefill | "blank" | undefined;
  readOnly: boolean;
}

function ProposePanel({ session, prefill }: { session: SessionSummary; prefill?: Prefill }) {
  return (
    <section className="card" aria-label="Propose an experiment">
      <div className="card-head">
        <h3>Propose an experiment</h3>
        <a className="quiet-link" href={sessionHref(session.id, "experiments")}>
          Close
        </a>
      </div>
      <p className="note">
        Starts from this session's car, track and setup. The in-game app will take this over; until
        then it is here.
      </p>
      <ProposeForm
        key={window.location.hash}
        sessionId={session.id}
        {...(prefill === undefined ? {} : { prefill })}
        onProposed={(experiment) => {
          window.location.hash = experimentHref(session.id, experiment.id);
        }}
      />
    </section>
  );
}

/** The ledger of the session's car, with an experiment or the propose form open beside it. */
export function ExperimentsPanel({
  session,
  experimentId,
  propose,
  readOnly,
}: ExperimentsPanelProps) {
  const proposing = propose !== undefined && !readOnly;
  return (
    <div className="panel-experiments">
      <section className="card" aria-label="Ledger">
        <div className="card-head">
          <h3>Ledger · {session.car_name}</h3>
          {!readOnly && !proposing && (
            <a className="action-link" href={proposeHref(session.id)}>
              Propose an experiment
            </a>
          )}
        </div>
        <Ledger
          sessionId={session.id}
          carName={session.car_name}
          {...(experimentId === undefined ? {} : { selected: experimentId })}
        />
      </section>
      <BaseSetupPanel session={session} readOnly={readOnly} />
      {propose !== undefined && readOnly && (
        <p className="empty">Experiments cannot be proposed in the read-only demo.</p>
      )}
      {proposing && (
        <ProposePanel session={session} {...(propose === "blank" ? {} : { prefill: propose })} />
      )}
      {experimentId !== undefined && (
        <section className="card" aria-label={`Experiment ${String(experimentId)}`}>
          <ExperimentPage key={experimentId} id={experimentId} />
        </section>
      )}
      <Scorecard />
    </div>
  );
}
