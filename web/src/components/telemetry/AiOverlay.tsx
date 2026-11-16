import { useEffect, useState } from "react";

import type { CornerReference, PassTrace } from "../../api/client";
import { seconds } from "../../format";
import { INK } from "../charts/chartTheme";
import type { ShownPass } from "./TraceCharts";

/** Where the AI's best pass of a corner comes from (docs/adr/0019). */
export interface AiPassSource {
  /** The comparison of a corner, when core has one. */
  corner: (corner: number) => CornerReference | undefined;
  /** The samples of a pass of another session: the AI's. */
  load: (sessionId: number, seq: number) => Promise<PassTrace>;
}

/** A seq no pass of the session has, so the AI's pass has its own row and line. */
const AI_SEQ = -1;

type Loading = { kind: "idle" } | { kind: "ready"; trace: PassTrace } | { kind: "failed" };

/** The AI's best pass of this corner on the same setup, overlaid when the owner ticks it. */
export function useAiOverlay(source: AiPassSource | undefined, corner: number) {
  const reference = source?.corner(corner);
  const sessionId = reference?.match === "same" ? reference.ai_best_session_id : undefined;
  const seq = reference?.ai_best_seq;
  const [on, setOn] = useState(false);
  const [loading, setLoading] = useState<Loading>({ kind: "idle" });

  useEffect(() => {
    if (!on || source === undefined || sessionId === undefined || seq === undefined) return;
    let cancelled = false;
    source.load(sessionId, seq).then(
      (trace) => {
        if (!cancelled) setLoading({ kind: "ready", trace });
      },
      () => {
        if (!cancelled) setLoading({ kind: "failed" });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [on, source, sessionId, seq]);

  if (sessionId === undefined || seq === undefined || reference === undefined) {
    return { control: null, shown: undefined };
  }
  const label = `AI best ${seconds(reference.ai_best_s ?? 0)}`;
  const control = (
    <p className="ai-overlay">
      <label>
        <input
          type="checkbox"
          checked={on}
          onChange={() => {
            setOn(!on);
          }}
          style={{ accentColor: INK.ai }}
        />{" "}
        Overlay the AI&apos;s best pass on this setup ({String(reference.ai_n)} AI passes)
      </label>
      {on && loading.kind === "failed" && (
        <span role="alert"> Its samples could not be loaded.</span>
      )}
    </p>
  );
  const shown: ShownPass | undefined =
    on && loading.kind === "ready"
      ? { seq: AI_SEQ, label, color: INK.ai, trace: loading.trace }
      : undefined;
  return { control, shown };
}
