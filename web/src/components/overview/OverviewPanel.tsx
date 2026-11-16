import type { SessionPasses } from "../../api/client";
import { ConditionsChart } from "./ConditionsChart";
import { CornerGrid } from "./CornerGrid";
import { NEAR, OFF, type Span, sessionFacts } from "./overviewData";

function spanText(span: Span | undefined, unit: string, digits = 1): string {
  if (span === undefined) return "not recorded";
  const first = span.first.toFixed(digits);
  const last = span.last.toFixed(digits);
  return first === last ? `${first} ${unit}` : `${first} → ${last} ${unit}`;
}

function Tile({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="tile">
      <dt>{label}</dt>
      <dd>
        {value}
        {note !== undefined && <small>{note}</small>}
      </dd>
    </div>
  );
}

function Legend() {
  const percent = (share: number) => `${String(share * 100)} %`;
  return (
    <p className="legend">
      <span className="key tone-best">session best</span>
      <span className="key tone-near">within {percent(NEAR)}</span>
      <span className="key tone-off">within {percent(OFF)}</span>
      <span className="key tone-far">slower</span>
      <span className="key tone-invalid">not counted</span>
      <span className="note">
        Zone time from zone entry to exit. L1 is the first lap driven. A cell opens the pass.
      </span>
    </p>
  );
}

/** The session at a glance: every corner of every lap, and the conditions it was driven in. */
export function OverviewPanel({ data }: { data: SessionPasses }) {
  const facts = sessionFacts(data.passes);
  if (facts.passes === 0) {
    return <p className="empty">No corner passes were recorded in this session.</p>;
  }
  return (
    <div className="panel-overview">
      <dl className="tiles">
        <Tile label="Passes" value={String(facts.passes)} note={`${String(facts.valid)} valid`} />
        <Tile
          label="Laps"
          value={String(facts.laps)}
          note={`${String(data.corners.length)} corners`}
        />
        <Tile label="Fuel" value={spanText(facts.fuel, "L")} />
        <Tile label="Road" value={spanText(facts.road, "°C")} />
        <Tile label="Air" value={spanText(facts.air, "°C")} />
        <Tile
          label="Grip"
          value={
            facts.grip === undefined
              ? "not recorded"
              : spanText({ first: facts.grip.first * 100, last: facts.grip.last * 100 }, "%")
          }
        />
      </dl>
      <section aria-label="Zone times" className="card">
        <h3>Zone times</h3>
        <CornerGrid sessionId={data.session_id} corners={data.corners} passes={data.passes} />
        <Legend />
      </section>
      <section aria-label="Conditions" className="card">
        <h3>Conditions through the session</h3>
        <ConditionsChart passes={data.passes} />
      </section>
    </div>
  );
}
