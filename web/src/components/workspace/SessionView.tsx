import { useCallback, useMemo } from "react";

import {
  fetchPasses,
  fetchTrace,
  type PassTrace,
  type SessionPasses,
  type SessionSummary,
} from "../../api/client";
import { type Loaded, useLoaded } from "../../hooks/useLoaded";
import type { Route } from "../../hooks/useRoute";
import { DiagnosisPage } from "../DiagnosisPage";
import { Garage } from "../Garage";
import { OverviewPanel } from "../overview/OverviewPanel";
import { TelemetryPanel } from "../telemetry/TelemetryPanel";
import { cachedLoader } from "../telemetry/useTraces";
import { ExperimentsPanel } from "./ExperimentsPanel";
import { SessionHeader } from "./SessionHeader";

/** What the session view reads of a session; injected in tests, the real client by default. */
export interface SessionApi {
  passes: (sessionId: number) => Promise<SessionPasses>;
  trace: (sessionId: number, seq: number) => Promise<PassTrace>;
}

const coreApi: SessionApi = { passes: fetchPasses, trace: fetchTrace };

interface SessionViewProps {
  session: SessionSummary;
  route: Route;
  readOnly: boolean;
  api?: SessionApi;
}

function PassesPanel({
  route,
  data,
  loadTrace,
}: {
  route: Route;
  data: SessionPasses;
  loadTrace: (seq: number) => Promise<PassTrace>;
}) {
  if (route.tab === "overview") return <OverviewPanel data={data} />;
  return (
    <TelemetryPanel
      sessionId={data.session_id}
      data={data}
      {...(route.corner === undefined ? {} : { corner: route.corner })}
      {...(route.pass === undefined ? {} : { pass: route.pass })}
      loadTrace={loadTrace}
    />
  );
}

interface TabPanelProps extends Omit<SessionViewProps, "api"> {
  passes: Loaded<SessionPasses>;
  loadTrace: (seq: number) => Promise<PassTrace>;
}

function TabPanel({ session, route, readOnly, passes, loadTrace }: TabPanelProps) {
  switch (route.tab) {
    case "overview":
    case "telemetry":
      if (passes.kind === "loading") return <p className="empty">Loading the passes…</p>;
      if (passes.kind === "failed") return <p role="alert">The passes could not be loaded.</p>;
      return <PassesPanel route={route} data={passes.value} loadTrace={loadTrace} />;
    case "diagnosis":
      return <DiagnosisPage key={session.id} sessionId={session.id} readOnly={readOnly} />;
    case "experiments":
      return (
        <ExperimentsPanel
          session={session}
          experimentId={route.experimentId}
          propose={route.propose}
          readOnly={readOnly}
        />
      );
    case "garage":
      return (
        <div className="panel-garage card">
          <Garage carName={session.car_name} />
        </div>
      );
  }
}

/** One session: its header, its tabs, and the panel of the tab on screen. */
export function SessionView({ session, route, readOnly, api = coreApi }: SessionViewProps) {
  const loadPasses = useCallback(() => api.passes(session.id), [api, session.id]);
  const passes = useLoaded(loadPasses);
  // Samples loaded once stay loaded while the session is on screen.
  const loadTrace = useMemo(
    () => cachedLoader((seq) => api.trace(session.id, seq)),
    [api, session.id],
  );
  return (
    <>
      <SessionHeader session={session} tab={route.tab} />
      <div className="panel">
        <TabPanel
          session={session}
          route={route}
          readOnly={readOnly}
          passes={passes}
          loadTrace={loadTrace}
        />
      </div>
    </>
  );
}
