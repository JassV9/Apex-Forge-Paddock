import { useEffect, useMemo, useState } from "react";

import type { PassTrace, SessionReference } from "../../api/client";
import type { AiPassSource } from "../telemetry/AiOverlay";

interface ReferenceApi {
  reference: (sessionId: number) => Promise<SessionReference>;
  trace: (sessionId: number, seq: number) => Promise<PassTrace>;
}

/**
 * The AI's best passes of a session's corners, for the telemetry overlay. Asks core only while
 * `enabled`; without an answer there is nothing to overlay and the charts are as before.
 */
export function useAiSource(
  api: ReferenceApi,
  sessionId: number,
  enabled: boolean,
): AiPassSource | undefined {
  const [reference, setReference] = useState<SessionReference | undefined>(undefined);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    api.reference(sessionId).then(
      (answer) => {
        if (!cancelled) setReference(answer);
      },
      () => undefined, // no reference: the overlay is simply not offered
    );
    return () => {
      cancelled = true;
    };
  }, [api, sessionId, enabled]);

  return useMemo(() => {
    if (reference?.session_id !== sessionId) return undefined;
    return {
      corner: (corner: number) => reference.corners.find((c) => c.corner === corner),
      load: (session: number, seq: number) => api.trace(session, seq),
    };
  }, [reference, sessionId, api]);
}
