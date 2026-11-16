import { useState } from "react";

import { fetchSessions, type SessionSummary } from "../../api/client";
import { useLoaded } from "../../hooks/useLoaded";
import type { Route } from "../../hooks/useRoute";
import { CoreStatus } from "../CoreStatus";
import { DrivePanel } from "./DrivePanel";
import { SessionRail } from "./SessionRail";
import { type SessionApi, SessionView } from "./SessionView";
import type { LauncherApi } from "./useLauncher";

interface WorkspaceProps {
  route: Route;
  readOnly: boolean;
  /** Injected in tests; the real client by default. */
  loadSessions?: () => Promise<SessionSummary[]>;
  sessionApi?: SessionApi;
  launcherApi?: LauncherApi;
}

function TopBar({ railOpen, onRail }: { railOpen: boolean; onRail: () => void }) {
  return (
    <header className="topbar">
      <button
        className="rail-toggle"
        aria-expanded={railOpen}
        aria-controls="rail"
        onClick={onRail}
      >
        Sessions
      </button>
      <p className="brand">
        <strong>Apex Forge</strong> Paddock
      </p>
      <CoreStatus />
    </header>
  );
}

function Main({
  sessions,
  route,
  readOnly,
  sessionApi,
}: {
  sessions: SessionSummary[];
  route: Route;
  readOnly: boolean;
  sessionApi: SessionApi | undefined;
}) {
  if (sessions.length === 0) {
    return (
      <p className="empty">
        No sessions yet. Drive with the Paddock agent enabled, or run `make demo`.
      </p>
    );
  }
  const session = sessions.find((s) => s.id === route.sessionId) ?? sessions[0];
  if (session === undefined) return null;
  if (route.sessionId !== undefined && session.id !== route.sessionId) {
    return <p role="alert">Session #{route.sessionId} does not exist.</p>;
  }
  return (
    <SessionView
      key={session.id}
      session={session}
      route={route}
      readOnly={readOnly}
      {...(sessionApi === undefined ? {} : { api: sessionApi })}
    />
  );
}

/** One page: the sessions on the left, the session on screen and its views on the right. */
export function Workspace({
  route,
  readOnly,
  loadSessions = fetchSessions,
  sessionApi,
  launcherApi,
}: WorkspaceProps) {
  const sessions = useLoaded(loadSessions);
  const [railOpen, setRailOpen] = useState(false);
  const list = sessions.kind === "ready" ? sessions.value : [];
  const current = route.sessionId ?? list[0]?.id;
  return (
    <div className={railOpen ? "workspace rail-open" : "workspace"}>
      <TopBar
        railOpen={railOpen}
        onRail={() => {
          setRailOpen(!railOpen);
        }}
      />
      <aside
        className="rail"
        id="rail"
        onClick={() => {
          setRailOpen(false);
        }}
      >
        {!readOnly && <DrivePanel {...(launcherApi === undefined ? {} : { api: launcherApi })} />}
        {sessions.kind === "ready" && (
          <SessionRail sessions={list} current={current} tab={route.tab} />
        )}
      </aside>
      <main className="main">
        {readOnly && (
          <p role="note" className="demo-note">
            Read-only demo on recorded sessions. Proposing and running experiments needs the game
            and a local install.
          </p>
        )}
        {sessions.kind === "loading" && <p className="empty">Loading sessions…</p>}
        {sessions.kind === "failed" && <p role="alert">Sessions could not be loaded.</p>}
        {sessions.kind === "ready" && (
          <Main sessions={list} route={route} readOnly={readOnly} sessionApi={sessionApi} />
        )}
      </main>
    </div>
  );
}
