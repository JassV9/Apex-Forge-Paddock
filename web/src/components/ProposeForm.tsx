import { useEffect, useState } from "react";

import {
  createExperiment,
  type Experiment,
  type ExperimentCreate,
  fetchSetupItems,
  type SetupItem,
} from "../api/client";
import type { Prefill } from "../hooks/useRoute";

/** The calls the form makes; injected in tests, the real client by default. */
export interface ProposeApi {
  items: (sessionId: number) => Promise<SetupItem[]>;
  create: (body: ExperimentCreate) => Promise<Experiment>;
}

const coreApi: ProposeApi = {
  items: fetchSetupItems,
  create: createExperiment,
};

interface ProposeFormProps {
  api?: ProposeApi;
  /** The session the experiment starts from: its car, track and setup. */
  sessionId: number;
  /** A candidate experiment of a diagnosis to start the form from. */
  prefill?: Prefill;
  /** Called with the stored experiment. */
  onProposed: (experiment: Experiment) => void;
}

interface Draft {
  parameter: string;
  clicks: number;
  statement: string;
  /** Expected gain per corner pass in seconds, low to high; empty for no prediction. */
  gainLow: string;
  gainHigh: string;
  /** Key of the diagnosis hypothesis under test; empty for the driver's own. */
  hypothesisKey: string;
}

const EMPTY: Draft = {
  parameter: "",
  clicks: -1,
  statement: "",
  gainLow: "",
  gainHigh: "",
  hypothesisKey: "",
};

function draftFrom(prefill: Prefill | undefined): Draft {
  if (prefill === undefined) return EMPTY;
  const { parameter, clicks, statement, hypothesisKey, gain } = prefill;
  const predicted =
    gain === undefined ? {} : { gainLow: String(gain.low), gainHigh: String(gain.high) };
  return { ...EMPTY, parameter, clicks, statement, hypothesisKey, ...predicted };
}

/** The request body for a draft. The prediction is optional and is on corner time. */
export function toRequest(sessionId: number, draft: Draft): ExperimentCreate {
  const low = Number(draft.gainLow);
  const high = Number(draft.gainHigh);
  const predicted = draft.gainLow !== "" && draft.gainHigh !== "" && low >= 0 && high >= low;
  return {
    session_id: sessionId,
    parameter: draft.parameter,
    clicks: draft.clicks,
    driver: "human",
    measure_only: false,
    statement: draft.statement.trim(),
    predictions: predicted ? [{ channel: "corner_time_s", direction: "decrease", low, high }] : [],
    ...(draft.hypothesisKey === "" ? {} : { hypothesis_key: draft.hypothesisKey }),
  };
}

/** Label and id: on many cars several items share a label ("Bump" on each corner). A linked
 * group's label already names its items ("Rear Wing (WING_1 + WING_9)", ADR 0026). */
function itemLabel(item: SetupItem): string {
  const named = item.label === "" || item.label.includes(item.name);
  const name = named ? item.label || item.name : `${item.label} [${item.name}]`;
  return `${name}: now ${String(item.value)}, ${String(item.min)} to ${String(item.max)}`;
}

function useSetupItems(api: ProposeApi, sessionId: number): SetupItem[] {
  const [items, setItems] = useState<SetupItem[]>([]);
  useEffect(() => {
    let cancelled = false;
    api.items(sessionId).then(
      (loaded) => {
        if (!cancelled) setItems(loaded);
      },
      () => {
        if (!cancelled) setItems([]);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, sessionId]);
  return items;
}

interface FieldsProps {
  draft: Draft;
  items: SetupItem[];
  set: (change: Partial<Draft>) => void;
}

function ChangeFields({ draft, items, set }: FieldsProps) {
  return (
    <>
      <label>
        Setup item
        <select
          value={draft.parameter}
          onChange={(e) => {
            set({ parameter: e.target.value });
          }}
        >
          {items.map((item) => (
            <option key={item.name} value={item.name}>
              {itemLabel(item)}
            </option>
          ))}
        </select>
      </label>
      <label>
        Clicks (negative lowers the value)
        <input
          type="number"
          step={1}
          value={draft.clicks}
          onChange={(e) => {
            set({ clicks: Math.trunc(Number(e.target.value)) });
          }}
        />
      </label>
    </>
  );
}

function HypothesisFields({ draft, set }: Omit<FieldsProps, "items">) {
  return (
    <>
      <label>
        Hypothesis
        <textarea
          required
          value={draft.statement}
          placeholder="A softer front bar gives the front more grip in slow corners."
          onChange={(e) => {
            set({ statement: e.target.value });
          }}
        />
      </label>
      <label>
        Predicted gain per corner pass, seconds, from
        <input
          type="number"
          min={0}
          step="any"
          value={draft.gainLow}
          onChange={(e) => {
            set({ gainLow: e.target.value });
          }}
        />
      </label>
      <label>
        to
        <input
          type="number"
          min={0}
          step="any"
          value={draft.gainHigh}
          onChange={(e) => {
            set({ gainHigh: e.target.value });
          }}
        />
      </label>
    </>
  );
}

/** Proposes a one-item experiment against the session on screen. Nothing is driven yet. */
export function ProposeForm({ api = coreApi, sessionId, prefill, onProposed }: ProposeFormProps) {
  const [edited, setEdited] = useState(() => draftFrom(prefill));
  const [problem, setProblem] = useState("");
  const items = useSetupItems(api, sessionId);
  // Until the driver picks an item, the first one of the list is selected.
  const draft = { ...edited, parameter: edited.parameter || (items[0]?.name ?? "") };

  const set = (change: Partial<Draft>) => {
    setEdited({ ...edited, ...change });
  };
  return (
    <form
      className="propose-form"
      onSubmit={(event) => {
        event.preventDefault();
        api.create(toRequest(sessionId, draft)).then(onProposed, (error: unknown) => {
          setProblem(error instanceof Error ? error.message : "The proposal failed.");
        });
      }}
    >
      {draft.hypothesisKey !== "" && (
        <p className="note">
          Tests hypothesis <code>{draft.hypothesisKey}</code> of the session's diagnosis. Its
          evidence and alternatives are stored with the experiment.
        </p>
      )}
      <ChangeFields draft={draft} items={items} set={set} />
      <HypothesisFields draft={draft} set={set} />
      <button type="submit" disabled={draft.parameter === "" || draft.clicks === 0}>
        Propose experiment
      </button>
      {problem !== "" && <p role="alert">{problem}</p>}
    </form>
  );
}
