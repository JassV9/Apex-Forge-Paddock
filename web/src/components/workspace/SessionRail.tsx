import { useState } from "react";

import type { SessionSummary } from "../../api/client";
import { dayOf, plural, timeOf } from "../../format";
import { sessionHref, type Tab } from "../../hooks/useRoute";

export const SOURCE_LABEL: Record<SessionSummary["source"], string> = {
  human: "Driven",
  ai: "Game AI",
  ghost: "Ghost Driver",
  demo: "Demo data",
};

export function trackText(session: SessionSummary): string {
  return session.layout === "" ? session.track_name : `${session.track_name} · ${session.layout}`;
}

interface SessionRailProps {
  sessions: SessionSummary[];
  current: number | undefined;
  /** The tab a session link opens, so switching sessions keeps the view. */
  tab: Tab;
}

/** Sessions grouped by the day they were driven, newest first. */
function byDay(sessions: SessionSummary[]): [string, SessionSummary[]][] {
  const days = new Map<string, SessionSummary[]>();
  for (const session of sessions) {
    const day = dayOf(session.started_at);
    days.set(day, [...(days.get(day) ?? []), session]);
  }
  return [...days.entries()];
}

function RailItem({
  session,
  current,
  tab,
}: {
  session: SessionSummary;
  current: boolean;
  tab: Tab;
}) {
  return (
    <li>
      <a
        href={sessionHref(session.id, tab)}
        aria-current={current ? "page" : undefined}
        className={session.passes === 0 ? "rail-item is-empty" : "rail-item"}
      >
        <span className="rail-car">{session.car_name}</span>
        <span className="rail-track">{trackText(session)}</span>
        <span className="rail-meta">
          <time dateTime={session.started_at}>{timeOf(session.started_at)}</time>
          <span>{plural(session.passes, "pass", "passes")}</span>
          {session.source !== "human" && (
            <span className="badge">{SOURCE_LABEL[session.source]}</span>
          )}
        </span>
      </a>
    </li>
  );
}

/** Every recorded session; the one on screen is marked. Sessions without a single pass (the
 * game started and left, or the agent reloaded) are folded away unless one is on screen. */
export function SessionRail({ sessions, current, tab }: SessionRailProps) {
  const [showEmpty, setShowEmpty] = useState(false);
  const empty = sessions.filter((s) => s.passes === 0 && s.id !== current).length;
  const shown = showEmpty ? sessions : sessions.filter((s) => s.passes > 0 || s.id === current);
  return (
    <nav className="rail-list" aria-label="Sessions">
      {byDay(shown).map(([day, list]) => (
        <section key={day} aria-label={day}>
          <h2 className="rail-day">{day}</h2>
          <ul>
            {list.map((session) => (
              <RailItem
                key={session.id}
                session={session}
                current={session.id === current}
                tab={tab}
              />
            ))}
          </ul>
        </section>
      ))}
      {empty > 0 && (
        <button
          type="button"
          className="rail-empty-toggle"
          onClick={() => {
            setShowEmpty(!showEmpty);
          }}
        >
          {showEmpty ? "Hide" : "Show"} {plural(empty, "empty session", "empty sessions")}
        </button>
      )}
    </nav>
  );
}
