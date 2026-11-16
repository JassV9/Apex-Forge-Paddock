import { useState } from "react";

import { type BaseSetupDriver, type BaseSetupReply, buildBaseSetup } from "../api/baseSetup";
import type { SessionSummary } from "../api/client";
import { BaseSetupResult } from "./BaseSetupResult";

interface BaseSetupPanelProps {
  session: SessionSummary;
  readOnly: boolean;
  /** Core's POST /sessions/{id}/base-setup; injected in tests. */
  build?: (sessionId: number, driver: BaseSetupDriver) => Promise<BaseSetupReply>;
}

type Outcome =
  | { kind: "idle" }
  | { kind: "working" }
  | { kind: "done"; reply: BaseSetupReply }
  | { kind: "failed"; message: string };

const DRIVERS: { value: BaseSetupDriver; label: string }[] = [
  { value: "human", label: "I drive" },
  { value: "ai", label: "AI drives" },
];

function DriverChoice(props: { driver: BaseSetupDriver; onChange: (d: BaseSetupDriver) => void }) {
  return (
    <fieldset>
      <legend>Who drives the experiment</legend>
      {DRIVERS.map((option) => (
        <label key={option.value}>
          <input
            type="radio"
            name="base-setup-driver"
            value={option.value}
            checked={props.driver === option.value}
            onChange={() => {
              props.onChange(option.value);
            }}
          />{" "}
          {option.label}
        </label>
      ))}
    </fieldset>
  );
}

/** "Build a base setup": every change this car measured as faster, judged together. */
export function BaseSetupPanel({ session, readOnly, build = buildBaseSetup }: BaseSetupPanelProps) {
  const [driver, setDriver] = useState<BaseSetupDriver>("ai");
  const [outcome, setOutcome] = useState<Outcome>({ kind: "idle" });
  const start = () => {
    setOutcome({ kind: "working" });
    build(session.id, driver).then(
      (reply) => {
        setOutcome({ kind: "done", reply });
      },
      (error: unknown) => {
        setOutcome({ kind: "failed", message: error instanceof Error ? error.message : "" });
      },
    );
  };
  return (
    <section className="card" aria-label="Base setup">
      <div className="card-head">
        <h3>Base setup · {session.car_name}</h3>
      </div>
      <p className="note">
        Takes each change this car measured as faster on corner time, one per item, and proposes
        them together as one experiment against the current setup. Nothing is borrowed from other
        cars and no language model is asked.
      </p>
      {readOnly ? (
        <p className="empty">Base setups cannot be built in the read-only demo.</p>
      ) : (
        <>
          <DriverChoice driver={driver} onChange={setDriver} />
          <button type="button" onClick={start} disabled={outcome.kind === "working"}>
            Build a base setup
          </button>
        </>
      )}
      {outcome.kind === "working" && <p>Building…</p>}
      {outcome.kind === "failed" && (
        <p role="alert">The base setup could not be built. {outcome.message}</p>
      )}
      {outcome.kind === "done" && <BaseSetupResult sessionId={session.id} reply={outcome.reply} />}
    </section>
  );
}
