import { useCallback, useMemo } from "react";

import {
  fetchPasses,
  fetchReference,
  fetchTrace,
  type PassTrace,
  type SessionPasses,
  type SessionReference,
  type SessionSummary,
} from "../../api/client";
import { type Loaded, useLoaded } from "../../hooks/useLoaded";
import type { Route } from "../../hooks/useRoute";
import { DiagnosisPage } from "../DiagnosisPage";
import { Garage } from "../Garage";
import { OverviewPanel } from "../overview/OverviewPanel";
import { ReferencePanel } from "../reference/ReferencePanel";
import { useAiSource } from "../reference/useAiSource";
import type { AiPassSource } from "../telemetry/AiOverlay";
import { TelemetryPanel } from "../telemetry/TelemetryPanel";
import { cachedLoader } from "../telemetry/useTraces";
import { ExperimentsPanel } from "./ExperimentsPanel";
import { hasReference, SessionHeader } from "./SessionHeader";

/** What the session view reads of a session; injected in tests, the real client by default. */
export interface SessionApi {
  passes: (sessionId: number) => Promise<SessionPasses>;
  trace: (sessionId: number, seq: number) => Promise<PassTrace>;
  reference: (sessionId: number, setupHash?: string) => Promise<SessionReference>;
}

const coreApi: SessionApi = { passes: fetchPasses, trace: fetchTrace, reference: fetchReference };

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
  ai,
}: {
  route: Route;
  data: SessionPasses;
  loadTrace: (seq: number) => Promise<PassTrace>;
  ai: AiPassSource | undefined;
}) {
  if (route.tab === "overview") return <OverviewPanel data={data} />;
  return (
    <TelemetryPanel
      sessionId={data.session_id}
      data={data}
      {...(route.corner === undefined ? {} : { corner: route.corner })}
      {...(route.pass === undefined ? {} : { pass: route.pass })}
      loadTrace={loadTrace}
      {...(ai === undefined ? {} : { ai })}
    />
  );
}

interface TabPanelProps extends Omit<SessionViewProps, "api"> {
  api: SessionApi;
  passes: Loaded<SessionPasses>;
  loadTrace: (seq: number) => Promise<PassTrace>;
  ai: AiPassSource | undefined;
}

function TabPanel({ session, route, readOnly, api, passes, loadTrace, ai }: TabPanelProps) {
  switch (route.tab) {
    case "overview":
    case "telemetry":
      if (passes.kind === "loading") return <p className="empty">Loading the passes…</p>;
      if (passes.kind === "failed") return <p role="alert">The passes could not be loaded.</p>;
      return <PassesPanel route={route} data={passes.value} loadTrace={loadTrace} ai={ai} />;
    case "reference":
      if (!hasReference(session)) {
        return (
          <p className="empty">The AI drove this session: it has no AI to be compared with.</p>
        );
      }
      return <ReferencePanel key={session.id} sessionId={session.id} load={api.reference} />;
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
  const ai = useAiSource(api, session.id, route.tab === "telemetry" && hasReference(session));
  return (
    <>
      <SessionHeader session={session} tab={route.tab} />
      <div className="panel">
        <TabPanel
          session={session}
          route={route}
          readOnly={readOnly}
          api={api}
          passes={passes}
          loadTrace={loadTrace}
          ai={ai}
        />
      </div>
    </>
  );
}
