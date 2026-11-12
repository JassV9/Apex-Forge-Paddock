import type { Experiment } from "../api/client";
import { effectText, formatTime, isBlind, STATE_LABEL, type State } from "../experimentText";

/** The test the experiment must pass. It was stored when the experiment was proposed. */
export function TestSection({ experiment }: { experiment: Experiment }) {
  const test = experiment.success_test;
  return (
    <section aria-label="Test">
      <h3>The test, fixed before driving</h3>
      <ul>
        <li>
          <code>{test.channel}</code> must be {test.better} by more than {test.rope} at{" "}
          {Math.round(test.level * 100)} % confidence.
        </li>
        {experiment.guardrails.map((guardrail) => (
          <li key={guardrail.channel}>
            Guardrail: <code>{guardrail.channel}</code> must not get {guardrail.worse} by more than{" "}
            {guardrail.tolerance}.
          </li>
        ))}
        {experiment.predictions.map((prediction) => (
          <li key={prediction.channel}>
            Prediction: <code>{prediction.channel}</code> will {prediction.direction} by{" "}
            {prediction.low} to {prediction.high}.
          </li>
        ))}
      </ul>
    </section>
  );
}

/** The two setups by code name. Which one is the baseline is shown only after the verdict. */
export function SetupsSection({ experiment }: { experiment: Experiment }) {
  const blind = isBlind(experiment);
  return (
    <section aria-label="Setups">
      <h3>Setups</h3>
      <ul>
        {experiment.arms.map((arm) => (
          <li key={arm.label}>
            {arm.label}
            {!blind && `: ${arm.is_baseline ? "baseline" : "variant"}, value ${String(arm.value)}`}
          </li>
        ))}
      </ul>
      {blind && <p>Which setup is the baseline is shown after the verdict.</p>}
    </section>
  );
}

function PlanSummary({ experiment }: { experiment: Experiment }) {
  const plan = experiment.plan;
  if (plan === undefined) return <p>Not planned yet.</p>;
  return (
    <p>
      Scatter {plan.sigma.toFixed(3)} ({plan.sigma_source}), {plan.passes_per_arm} passes per setup
      and corner. Detects effects of {plan.mde_detect.toFixed(3)} or more; decides effects of{" "}
      {plan.mde_decide.toFixed(3)} or more.
    </p>
  );
}

/** The run plan: what it can detect, and how far each block has been driven. */
export function PlanSection({ experiment }: { experiment: Experiment }) {
  return (
    <section aria-label="Run plan">
      <h3>Run plan</h3>
      <PlanSummary experiment={experiment} />
      {experiment.blocks.length > 0 && (
        <table>
          <thead>
            <tr>
              <th scope="col">Block</th>
              <th scope="col">Setup</th>
              <th scope="col">Push laps</th>
              <th scope="col">Corner passes</th>
            </tr>
          </thead>
          <tbody>
            {experiment.blocks.map((block) => (
              <tr key={block.order}>
                <td>{block.order}</td>
                <td>{block.label}</td>
                <td>
                  {block.laps_target === 0
                    ? "back on the car, no laps"
                    : `${String(block.laps_done)} of ${String(block.laps_target)}`}
                </td>
                <td>{block.passes}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

/** What the engine measured: variant minus baseline, with its interval, per channel. */
export function ResultsSection({ experiment }: { experiment: Experiment }) {
  if (experiment.results.length === 0) return null;
  return (
    <section aria-label="Results">
      <h3>Results: variant minus baseline</h3>
      <table>
        <thead>
          <tr>
            <th scope="col">Channel</th>
            <th scope="col">Role</th>
            <th scope="col">Corners</th>
            <th scope="col">Effect and interval</th>
            <th scope="col">Passes</th>
            <th scope="col">Reading</th>
          </tr>
        </thead>
        <tbody>
          {experiment.results.map((result) => (
            <tr key={`${result.channel}-${result.role}-${result.corner_class}`}>
              <td>
                <code>{result.channel}</code>
              </td>
              <td>{result.role.replace("_", " ")}</td>
              <td>{result.corner_class}</td>
              <td>{effectText(result)}</td>
              <td>{result.n_passes}</td>
              <td>{result.effect_class}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

export function VerdictSection({ experiment }: { experiment: Experiment }) {
  const verdict = experiment.verdict;
  if (verdict === undefined) return null;
  return (
    <section aria-label="Verdict">
      <h3>Verdict</h3>
      <p>
        <strong>{STATE_LABEL[verdict.decision]}</strong>, decided by the {verdict.decided_by} on{" "}
        {formatTime(verdict.decided_at)}. {verdict.reason}
      </p>
      {verdict.prediction_total > 0 && (
        <p>
          Predictions that came true: {verdict.prediction_hits} of {verdict.prediction_total}.
        </p>
      )}
    </section>
  );
}

function stateName(state: string): string {
  return state in STATE_LABEL ? STATE_LABEL[state as State] : state;
}

/** Every state the experiment has been in, in order, with when and why it moved. */
export function HistorySection({ experiment }: { experiment: Experiment }) {
  return (
    <section aria-label="History">
      <h3>History</h3>
      <ol>
        {experiment.events.map((event, index) => (
          <li key={index}>
            <time dateTime={event.at}>{formatTime(event.at)}</time>{" "}
            <strong>
              {event.from === "" ? "" : `${stateName(event.from)} → `}
              {stateName(event.to)}
            </strong>
            : {event.reason}
          </li>
        ))}
      </ol>
    </section>
  );
}
