import { type Experiment, fetchExperiments } from "../api/client";
import { changeText, formatTime, STATE_LABEL } from "../experimentText";
import { useLoaded } from "../hooks/useLoaded";
import { experimentHref } from "../hooks/useRoute";

interface LedgerProps {
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

function LedgerRow({ experiment }: { experiment: Experiment }) {
  return (
    <tr>
      <td>
        <a href={experimentHref(experiment.id)}>#{experiment.id}</a>
      </td>
      <td>{formatTime(experiment.created_at)}</td>
      <td>
        {experiment.car_name}, {experiment.track_name}
      </td>
      <td>{changeText(experiment)}</td>
      <td>
        <StateBadge state={experiment.state} />
      </td>
      <td>{outcome(experiment)}</td>
      <td>{predictions(experiment)}</td>
    </tr>
  );
}

/** Every experiment, newest first, whatever its outcome: failed ones are kept. */
export function Ledger({ loadExperiments = fetchExperiments }: LedgerProps) {
  const state = useLoaded(loadExperiments);

  if (state.kind === "loading") return <p>Loading the ledger…</p>;
  if (state.kind === "failed") return <p role="alert">The ledger could not be loaded.</p>;
  if (state.value.length === 0) {
    return <p>No experiments yet. Propose one from a recorded session.</p>;
  }
  return (
    <table>
      <thead>
        <tr>
          <th scope="col">Experiment</th>
          <th scope="col">Proposed</th>
          <th scope="col">Car and track</th>
          <th scope="col">Change</th>
          <th scope="col">State</th>
          <th scope="col">Why</th>
          <th scope="col">Predictions hit</th>
        </tr>
      </thead>
      <tbody>
        {state.value.map((experiment) => (
          <LedgerRow key={experiment.id} experiment={experiment} />
        ))}
      </tbody>
    </table>
  );
}
