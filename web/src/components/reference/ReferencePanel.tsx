import { useCallback, useState } from "react";

import type { CornerReference, SessionReference } from "../../api/client";
import { seconds, signedSeconds } from "../../format";
import { useLoaded } from "../../hooks/useLoaded";
import { telemetryHref } from "../../hooks/useRoute";
import { byTimeLost, CLASS_LABEL, differenceText, matchNote } from "./referenceData";

export type ReferenceLoader = (sessionId: number, setupHash?: string) => Promise<SessionReference>;

function AiCell({ corner }: { corner: CornerReference }) {
  if (corner.ai_median_s === undefined) return <td className="num">–</td>;
  return (
    <td className="num" title={`${String(corner.ai_n)} AI passes`}>
      {seconds(corner.ai_median_s)}
      <small> ±{seconds(corner.ai_spread_s ?? 0)}</small>
    </td>
  );
}

function CornerRow({ sessionId, corner }: { sessionId: number; corner: CornerReference }) {
  const note = matchNote(corner);
  return (
    <tr className={`class-${corner.class}`}>
      <th scope="row">
        <a href={telemetryHref(sessionId, corner.corner)}>
          {corner.name === "" ? `C${String(corner.corner)}` : corner.name}
        </a>
      </th>
      <td className="num">{seconds(corner.human_best_s)}</td>
      <td className="num" title={`${String(corner.human_n)} passes`}>
        {seconds(corner.human_median_s)}
      </td>
      <AiCell corner={corner} />
      <td className="num">
        {corner.delta_median_s === undefined ? "–" : signedSeconds(corner.delta_median_s)}
      </td>
      <td title={corner.reason}>{CLASS_LABEL[corner.class]}</td>
      <td>
        {corner.differences.map((d) => (
          <div key={d.metric}>{differenceText(d)}</div>
        ))}
        {note !== "" && <small>{note}</small>}
      </td>
    </tr>
  );
}

function SetupChooser(props: { data: SessionReference; onPick: (hash: string) => void }) {
  const { data, onPick } = props;
  if (data.setups.length < 2) return null;
  return (
    <label className="setup-chooser">
      Setup{" "}
      <select
        value={data.setup_hash ?? ""}
        onChange={(e) => {
          onPick(e.target.value);
        }}
      >
        {data.setups.map((s) => (
          <option key={s.setup_hash} value={s.setup_hash}>
            {s.setup_hash} ({String(s.passes)} passes)
          </option>
        ))}
      </select>
    </label>
  );
}

function ReferenceTable({ data }: { data: SessionReference }) {
  return (
    <table className="reference-table">
      <caption>Corners by time lost to the AI on the typical pass</caption>
      <thead>
        <tr>
          <th scope="col">Corner</th>
          <th scope="col">Your best</th>
          <th scope="col">Your typical</th>
          <th scope="col">AI</th>
          <th scope="col">Lost</th>
          <th scope="col">Verdict</th>
          <th scope="col">What differs</th>
        </tr>
      </thead>
      <tbody>
        {byTimeLost(data.corners).map((corner) => (
          <CornerRow key={corner.corner} sessionId={data.session_id} corner={corner} />
        ))}
      </tbody>
    </table>
  );
}

/**
 * The session against AC's AI on the same car and setup: where you lose time the AI does not
 * (practise it), and where the AI is just as slow (the car or the setup).
 */
export function ReferencePanel({ sessionId, load }: { sessionId: number; load: ReferenceLoader }) {
  const [setupHash, setSetupHash] = useState<string | undefined>(undefined);
  const loader = useCallback(() => load(sessionId, setupHash), [load, sessionId, setupHash]);
  const reference = useLoaded(loader);
  if (reference.kind === "loading") return <p className="empty">Loading the comparison…</p>;
  if (reference.kind === "failed") return <p role="alert">The comparison could not be loaded.</p>;
  const data = reference.value;
  return (
    <section className="panel-reference card" aria-label="Against the AI">
      <h3>Against the AI</h3>
      <p className="note">
        The AI drives the same car on the same setup, repeatably, below its own grip limit: a corner
        where you are slower than the AI by more than its spread is one to practise; one where you
        match it or beat it is what the car and setup allow, as far as the AI shows.{" "}
        {String(data.ai_passes)} AI passes on this car and track.
      </p>
      <SetupChooser data={data} onPick={setSetupHash} />
      {data.corners.length === 0 ? (
        <p className="empty">This session has no valid corner pass on this setup.</p>
      ) : (
        <ReferenceTable data={data} />
      )}
    </section>
  );
}
