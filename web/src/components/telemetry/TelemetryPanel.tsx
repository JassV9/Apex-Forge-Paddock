import { useState } from "react";

import type { CornerSummary, PassRecord, SessionPasses } from "../../api/client";
import { telemetryHref } from "../../hooks/useRoute";
import { PassPicker } from "./PassPicker";
import { colorsOf, passLabel, pickCorner, pickPass } from "./picks";
import { type ShownPass, TraceCharts } from "./TraceCharts";
import type { XAxis } from "./traceData";
import { type TraceLoader, useTraces } from "./useTraces";

interface TelemetryPanelProps {
  sessionId: number;
  data: SessionPasses;
  corner?: number;
  pass?: number;
  loadTrace: TraceLoader;
}

function CornerChooser({
  sessionId,
  corners,
  current,
}: {
  sessionId: number;
  corners: CornerSummary[];
  current: number;
}) {
  return (
    <nav className="corner-chooser" aria-label="Corners">
      {corners.map((corner) => (
        <a
          key={corner.corner}
          href={telemetryHref(sessionId, corner.corner)}
          aria-current={corner.corner === current ? "true" : undefined}
          title={`${String(corner.passes)} passes, ${String(corner.valid_passes)} valid`}
        >
          {corner.name === "" ? `C${String(corner.corner)}` : corner.name}
        </a>
      ))}
    </nav>
  );
}

function AxisSwitch({ value, onChange }: { value: XAxis; onChange: (axis: XAxis) => void }) {
  return (
    <fieldset className="segmented">
      <legend>Against</legend>
      {(["distance", "time"] as const).map((axis) => (
        <label key={axis}>
          <input
            type="radio"
            name="x-axis"
            checked={value === axis}
            onChange={() => {
              onChange(axis);
            }}
          />
          {axis === "distance" ? "Distance" : "Time"}
        </label>
      ))}
    </fieldset>
  );
}

interface CornerViewProps extends Omit<TelemetryPanelProps, "corner" | "data"> {
  corner: number;
  passes: PassRecord[];
  summary: CornerSummary | undefined;
  trackLength: number;
}

/** Which passes are overlaid: the best and the picked one always, the rest when ticked. */
function useOverlay(passes: PassRecord[], summary: CornerSummary | undefined, asked?: number) {
  const best = summary?.best_seq;
  const picked = pickPass(passes, summary, asked);
  const [ticked, setTicked] = useState(
    () => new Set([best, picked].filter((s) => s !== undefined)),
  );
  const order = passes.filter((p) => ticked.has(p.seq) || p.seq === picked).map((p) => p.seq);
  const colors = colorsOf(
    passes.map((p) => p.seq),
    best,
    picked,
  );
  const toggle = (seq: number) => {
    setTicked((now) => {
      const next = new Set(now);
      if (!next.delete(seq)) next.add(seq);
      return next;
    });
  };
  return { best, picked, order, colors, toggle };
}

/** One corner: which passes to overlay, and their channels. */
function CornerView(props: CornerViewProps) {
  const { sessionId, corner, passes, summary, pass, loadTrace, trackLength } = props;
  const { best, picked, order, colors, toggle } = useOverlay(passes, summary, pass);
  const [xAxis, setXAxis] = useState<XAxis>("distance");
  const traces = useTraces(order, loadTrace);
  // A new list redraws the charts; it changes only when a pass is added, dropped or loaded.
  const drawn: ShownPass[] = passes.flatMap((p) => {
    const trace = traces.bySeq.get(p.seq);
    if (!order.includes(p.seq) || trace === undefined) return [];
    return [{ seq: p.seq, label: passLabel(p, best), color: colors.get(p.seq) ?? "", trace }];
  });
  return (
    <div className="telemetry">
      <aside className="telemetry-side">
        <AxisSwitch value={xAxis} onChange={setXAxis} />
        <PassPicker
          passes={passes}
          summary={summary}
          picked={picked}
          shown={new Set(order)}
          colors={colors}
          hrefOf={(seq) => telemetryHref(sessionId, corner, seq)}
          onToggle={toggle}
        />
        {traces.failed.size > 0 && (
          <p role="alert">Samples of {traces.failed.size} pass(es) could not be loaded.</p>
        )}
      </aside>
      <section className="telemetry-main" aria-label="Channels" aria-busy={traces.pending}>
        {drawn.length === 0 ? (
          <p className="empty">{traces.pending ? "Loading samples…" : "Pick a pass to show."}</p>
        ) : (
          <TraceCharts passes={drawn} axis={xAxis} trackLength={trackLength} />
        )}
      </section>
    </div>
  );
}

/** A corner's passes overlaid: speed, pedals, steering and slip per axle, best against picked. */
export function TelemetryPanel({ sessionId, data, corner, pass, loadTrace }: TelemetryPanelProps) {
  const current = pickCorner(data, corner);
  if (current === undefined) return <p className="empty">This session has no corner passes.</p>;
  const passes = data.passes.filter((p) => p.corner === current);
  const summary = data.corners.find((c) => c.corner === current);
  return (
    <div className="panel-telemetry">
      <CornerChooser sessionId={sessionId} corners={data.corners} current={current} />
      <CornerView
        key={current}
        sessionId={sessionId}
        corner={current}
        passes={passes}
        summary={summary}
        {...(pass === undefined ? {} : { pass })}
        loadTrace={loadTrace}
        trackLength={data.track_length_m}
      />
    </div>
  );
}
