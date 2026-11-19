import { useCallback, useEffect, useState } from "react";

import {
  fetchSetupItems,
  fetchSweeps,
  type SessionSummary,
  type SetupItem,
  startSweep,
  stopSweep,
  type Sweep,
  type SweepCreate,
} from "../../api/client";
import { context, curveSentence } from "./curveText";

/** Experiment D of the reliability investigation: one click down, three up. */
const DEFAULT_STEPS = "-1, 1, 2, 3";
/** A sweep's step takes most of an hour; look again this often. */
const REFRESH_MS = 15_000;

interface DiscoverPanelProps {
  session: SessionSummary;
  readOnly: boolean;
  /** Injected in tests; the real client by default. */
  api?: {
    list: () => Promise<Sweep[]>;
    items: (sessionId: number) => Promise<SetupItem[]>;
    start: (sessionId: number, body: SweepCreate) => Promise<Sweep>;
    stop: (id: number) => Promise<Sweep>;
  };
  refreshMs?: number;
}

const realApi = { list: fetchSweeps, items: fetchSetupItems, start: startSweep, stop: stopSweep };

function isOpen(sweep: Sweep): boolean {
  return sweep.state === "running" || sweep.state === "waiting";
}

/** "-1, 1, +2" as clicks; null when any part is not a whole number. */
export function parseSteps(text: string): number[] | null {
  const parts = text.split(/[\s,]+/).filter((part) => part !== "");
  const steps = parts.map(Number);
  return steps.length > 0 && steps.every(Number.isInteger) ? steps : null;
}

function signed(clicks: number): string {
  return clicks > 0 ? `+${String(clicks)}` : String(clicks);
}

function SweepView({ sweep, onStop }: { sweep: Sweep; onStop?: () => void }) {
  const curves = sweep.curve?.curves ?? [];
  return (
    <div className="sweep">
      <p>
        <strong>{sweep.parameter_label}</strong> · {sweep.state} · {sweep.steps.length} of{" "}
        {sweep.planned.length} steps ({sweep.planned.map(signed).join(", ")}) · hint{" "}
        {sweep.ai_hint_share} · {sweep.message}
      </p>
      {sweep.skipped.length > 0 && <p className="note">{sweep.skipped.join("; ")}</p>}
      {sweep.steps.length > 0 && (
        <ul className="sweep-steps">
          {sweep.steps.map((step) => (
            <li key={step.experiment_id}>
              {signed(step.clicks)}: #{step.experiment_id} {step.state}
              {step.finding_class === undefined
                ? ""
                : ` (${step.finding_class.replaceAll("_", " ")})`}
            </li>
          ))}
        </ul>
      )}
      {curves.length > 0 && (
        <ul className="sweep-curves" aria-label="How the car responds">
          {curves.map((curve) => (
            <li
              key={`${curve.source}-${curve.metric}-${curve.phase}-${curve.regime}-${curve.role ?? ""}`}
            >
              {curveSentence(curve)} <span className="note">({context(curve)})</span>
            </li>
          ))}
        </ul>
      )}
      {onStop !== undefined && (
        <button type="button" onClick={onStop}>
          Stop sweep
        </button>
      )}
    </div>
  );
}

interface StartFormProps {
  items: SetupItem[];
  onStart: (body: SweepCreate) => void;
}

function StartForm({ items, onStart }: StartFormProps) {
  const [parameter, setParameter] = useState("");
  const [steps, setSteps] = useState(DEFAULT_STEPS);
  const [share, setShare] = useState("");
  const parsed = parseSteps(steps);
  const hint = Number(share);
  const ready = parameter !== "" && parsed !== null && share !== "" && hint >= 0.5 && hint <= 1.5;
  return (
    <form
      className="sweep-start"
      onSubmit={(event) => {
        event.preventDefault();
        if (ready) onStart({ parameter, steps: parsed, ai_hint_share: hint });
      }}
    >
      <label>
        Setting{" "}
        <select
          value={parameter}
          onChange={(event) => {
            setParameter(event.target.value);
          }}
        >
          <option value="">choose…</option>
          {items.map((item) => (
            <option key={item.name} value={item.name}>
              {item.label} (now {item.value}, {item.min} to {item.max})
            </option>
          ))}
        </select>
      </label>{" "}
      <label>
        Steps (clicks){" "}
        <input
          value={steps}
          onChange={(event) => {
            setSteps(event.target.value);
          }}
        />
      </label>{" "}
      <label>
        Pinned AI hint share{" "}
        <input
          type="number"
          min={0.5}
          max={1.5}
          step={0.0001}
          value={share}
          onChange={(event) => {
            setShare(event.target.value);
          }}
        />
      </label>{" "}
      <button type="submit" disabled={!ready}>
        Start sweep
      </button>
    </form>
  );
}

/**
 * Discover how the car responds to one setting: the AI drives each step as its own experiment
 * against the same baseline at a pinned hint, and the curve of each observable is classified
 * when every step is in (docs/adr/0032-sensitivity-discovery-sweeps.md).
 */
export function DiscoverPanel({
  session,
  readOnly,
  api = realApi,
  refreshMs = REFRESH_MS,
}: DiscoverPanelProps) {
  const [sweeps, setSweeps] = useState<Sweep[] | null>(null);
  const [items, setItems] = useState<SetupItem[]>([]);
  const [error, setError] = useState("");

  const reload = useCallback(() => {
    api.list().then(setSweeps, () => {
      setError("Sweeps could not be loaded.");
    });
  }, [api]);

  useEffect(() => {
    reload();
    const timer = window.setInterval(reload, refreshMs);
    return () => {
      window.clearInterval(timer);
    };
  }, [reload, refreshMs]);

  useEffect(() => {
    if (readOnly) return;
    api.items(session.id).then(setItems, () => {
      setItems([]);
    });
  }, [api, readOnly, session.id]);

  const act = (work: Promise<Sweep>) => {
    setError("");
    work.then(reload, (err: unknown) => {
      setError(err instanceof Error ? err.message : String(err));
    });
  };

  const mine = (sweeps ?? []).filter((s) => s.car_name === session.car_name);
  const open = mine.find(isOpen);
  return (
    <section className="card" aria-label="Discover">
      <h3>Discover · {session.car_name}</h3>
      <p className="note">
        How the car responds to one setting, step by step, each step against the same baseline at a
        pinned AI hint. It never changes your setup. Calibrate the AI in the game first.
      </p>
      {error !== "" && <p role="alert">{error}</p>}
      {open === undefined && !readOnly && (
        <StartForm
          items={items}
          onStart={(body) => {
            act(api.start(session.id, body));
          }}
        />
      )}
      {mine.map((sweep) => (
        <SweepView
          key={sweep.id}
          sweep={sweep}
          {...(readOnly || !isOpen(sweep)
            ? {}
            : {
                onStop: () => {
                  act(api.stop(sweep.id));
                },
              })}
        />
      ))}
    </section>
  );
}
