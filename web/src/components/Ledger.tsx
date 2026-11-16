import { type Experiment, fetchExperiments } from "../api/client";
import { changeText, formatTime, STATE_LABEL } from "../experimentText";
import { useLoaded } from "../hooks/useLoaded";
import { experimentHref } from "../hooks/useRoute";

interface LedgerProps {
  /** The session on screen: links open an experiment next to it. */
  sessionId: number;
  /** Only this car's experiments are listed. */
  carName: string;
  /** The experiment open beside the ledger. */
  selected?: number;
  /** Injected in tests; the real client by default. */
  loadExperiments?: () => Promise<Experiment[]>;
}

/** Why the experiment is where it is: the verdict, or the reason of its latest transition. */
function outcome(experiment: Experiment): string {
  if (experiment.verdict !== undefined) return experiment.verdict.reason;
  return experiment.events.at(-1)?.reason ?? "";
}

function predictions(experiment: Experiment): string {
  const verdict = experiment.verdict;
  if (verdict === undefined || verdict.prediction_total === 0) return "–";
  return `${String(verdict.prediction_hits)} of ${String(verdict.prediction_total)}`;
}

export function StateBadge({ state }: { state: Experiment["state"] }) {
  return <span className={`state state-${state}`}>{STATE_LABEL[state]}</span>;
}

interface LedgerRowProps {
  experiment: Experiment;
  sessionId: number;
  selected: boolean;
}

function LedgerRow({ experiment, sessionId, selected }: LedgerRowProps) {
  return (
    <tr className={selected ? "is-picked" : undefined}>
      <td>
        <a
          href={experimentHref(sessionId, experiment.id)}
          aria-current={selected ? "true" : undefined}
        >
          #{experiment.id}
        </a>
      </td>
      <td>{formatTime(experiment.created_at)}</td>
      <td>{experiment.track_name}</td>
      <td>{changeText(experiment)}</td>
      <td>
        <StateBadge state={experiment.state} />
      </td>
      <td>{outcome(experiment)}</td>
      <td>{predictions(experiment)}</td>
    </tr>
  );
}

/** Every experiment on the car, newest first, whatever its outcome: failed ones are kept. */
export function Ledger(props: LedgerProps) {
  const { sessionId, carName, selected, loadExperiments = fetchExperiments } = props;
  const state = useLoaded(loadExperiments);

  if (state.kind === "loading") return <p>Loading the ledger…</p>;
  if (state.kind === "failed") return <p role="alert">The ledger could not be loaded.</p>;
  const mine = state.value.filter((experiment) => experiment.car_name === carName);
  if (mine.length === 0) {
    return <p className="empty">No experiments on this car yet.</p>;
  }
  return (
    <table className="ledger">
      <thead>
        <tr>
          <th scope="col">Experiment</th>
          <th scope="col">Proposed</th>
          <th scope="col">Track</th>
          <th scope="col">Change</th>
          <th scope="col">State</th>
          <th scope="col">Why</th>
          <th scope="col">Predictions hit</th>
        </tr>
      </thead>
      <tbody>
        {mine.map((experiment) => (
          <LedgerRow
            key={experiment.id}
            experiment={experiment}
            sessionId={sessionId}
            selected={experiment.id === selected}
          />
        ))}
      </tbody>
    </table>
  );
}
