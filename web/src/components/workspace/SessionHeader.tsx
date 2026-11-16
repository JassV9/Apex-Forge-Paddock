import type { SessionSummary } from "../../api/client";
import { dayOf, plural, timeOf } from "../../format";
import { sessionHref, type Tab, TABS } from "../../hooks/useRoute";
import { SOURCE_LABEL, trackText } from "./SessionRail";

const TAB_LABEL: Record<Tab, string> = {
  overview: "Overview",
  telemetry: "Telemetry",
  diagnosis: "Diagnosis",
  experiments: "Experiments",
  garage: "Garage",
};

/** The session on screen, and the views of it. Experiments and the garage are per car. */
export function SessionHeader({ session, tab }: { session: SessionSummary; tab: Tab }) {
  return (
    <header className="session-header">
      <div className="session-title">
        <p className="eyebrow">
          Session #{session.id} · {SOURCE_LABEL[session.source]}
        </p>
        <h1>{session.car_name}</h1>
        <p className="session-meta">
          <span>{trackText(session)}</span>
          <span>
            <time dateTime={session.started_at}>
              {dayOf(session.started_at)} {timeOf(session.started_at)}
            </time>
          </span>
          <span>
            {plural(session.passes, "pass", "passes")} over {plural(session.corners, "corner")}
          </span>
        </p>
      </div>
      <nav className="tabs" aria-label="Views">
        {TABS.map((name) => (
          <a
            key={name}
            href={sessionHref(session.id, name)}
            aria-current={name === tab ? "page" : undefined}
          >
            {TAB_LABEL[name]}
            {(name === "experiments" || name === "garage") && <small> · car</small>}
          </a>
        ))}
      </nav>
    </header>
  );
}
