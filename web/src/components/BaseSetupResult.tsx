import type { BaseSetup, BaseSetupChange, BaseSetupReply } from "../api/baseSetup";
import { clicksText } from "../experimentText";
import { experimentHref } from "../hooks/useRoute";

/** Seconds per corner pass as milliseconds with their sign: "−4.2 ms". */
export function millis(value: number): string {
  const sign = value > 0 ? "+" : value < 0 ? "−" : "";
  return `${sign}${Math.abs(value * 1000).toFixed(1)} ms`;
}

function spanText(row: { estimate: number; ci_low: number; ci_high: number }): string {
  return `${millis(row.estimate)} [${millis(row.ci_low)}, ${millis(row.ci_high)}]`;
}

function ChangeItem({ change }: { change: BaseSetupChange }) {
  const experiments = change.experiment_ids.map(String).join(", ");
  return (
    <li>
      <strong>
        {change.label === "" ? change.parameter : change.label} {clicksText(change.clicks)}
      </strong>
      : {String(change.from_value)} → {String(change.to_value)}. Measured {spanText(change)} per
      corner pass over {change.n_passes} passes ({change.effect_class}), experiments {experiments}.
    </li>
  );
}

function Prediction({ setup }: { setup: BaseSetup }) {
  const prediction = setup.prediction;
  if (prediction === undefined) return null;
  return (
    <p>
      Prediction, written down before a lap is driven: {spanText(prediction)} per corner pass on{" "}
      {prediction.channel} at {Math.round(prediction.level * 100)} %. {prediction.note}
    </p>
  );
}

function LeftOut({ setup }: { setup: BaseSetup }) {
  if (setup.left_out.length === 0) return null;
  return (
    <details>
      <summary>Left out ({setup.left_out.length})</summary>
      <ul>
        {setup.left_out.map((entry) => (
          <li key={`${entry.parameter} ${String(entry.clicks)}`}>
            {entry.label === "" ? entry.parameter : entry.label} {clicksText(entry.clicks)}:{" "}
            {entry.reason}
          </li>
        ))}
      </ul>
    </details>
  );
}

function Measured({ setup }: { setup: BaseSetup }) {
  if (setup.measured.length === 0) return null;
  return (
    <ul aria-label="Measured on this car">
      {setup.measured.map((row) => (
        <li key={`${row.parameter} ${String(row.clicks)}`}>
          {row.label === "" ? row.parameter : row.label} {clicksText(row.clicks)}:{" "}
          {row.effect_class}, {spanText(row)}
        </li>
      ))}
    </ul>
  );
}

/** What core built: the changes with their evidence and the prediction, or why there are none. */
export function BaseSetupResult({
  sessionId,
  reply,
}: {
  sessionId: number;
  reply: BaseSetupReply;
}) {
  const setup = reply.base_setup;
  if (reply.experiment_id === undefined) {
    return (
      <div className="base-setup-refused">
        <p>{reply.error ?? "No measured change qualifies for a base setup."}</p>
        <Measured setup={setup} />
        <LeftOut setup={setup} />
      </div>
    );
  }
  return (
    <div className="base-setup-built">
      <p>
        Base setup for {reply.car_name}: {setup.changes.length} measured{" "}
        {setup.changes.length === 1 ? "change" : "changes"}, judged as experiment{" "}
        {reply.experiment_id} against the current setup.{" "}
        <a className="action-link" href={experimentHref(sessionId, reply.experiment_id)}>
          Open experiment {reply.experiment_id}
        </a>
      </p>
      <ul aria-label="Changes">
        {setup.changes.map((change) => (
          <ChangeItem key={change.parameter} change={change} />
        ))}
      </ul>
      <Prediction setup={setup} />
      <LeftOut setup={setup} />
    </div>
  );
}
