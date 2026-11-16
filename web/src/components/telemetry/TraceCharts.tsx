import { useMemo, useState } from "react";
import type uPlot from "uplot";

import type { PassTrace } from "../../api/client";
import { axis, baseOptions } from "../charts/chartTheme";
import { UPlotChart } from "../charts/UPlotChart";
import {
  CHANNELS,
  type Channel,
  type Frame,
  frameOf,
  nearestIndex,
  seriesOf,
  type XAxis,
} from "./traceData";

/** A pass as the charts draw it. */
export interface ShownPass {
  seq: number;
  label: string;
  color: string;
  trace: PassTrace;
}

interface TraceChartsProps {
  passes: ShownPass[];
  axis: XAxis;
  trackLength: number;
}

const X_LABEL: Record<XAxis, string> = {
  distance: "Distance from the start line (m)",
  time: "Time since zone entry (s)",
};

interface ChannelChartProps {
  channel: Channel;
  frame: Frame;
  passes: ShownPass[];
  xAxis: XAxis;
  last: boolean;
  onCursor: (x: number | null) => void;
}

/** The chart of one channel: a line per pass, the x axis written out only under the last. */
function chartOptions(props: ChannelChartProps): Omit<uPlot.Options, "width" | "height"> {
  const { channel, frame, passes, xAxis, last, onCursor } = props;
  return {
    ...baseOptions("telemetry"),
    scales: { x: { time: false } },
    series: [
      {},
      ...passes.map((pass) => ({
        label: pass.label,
        stroke: pass.color,
        width: 1.5,
        spanGaps: true,
        points: { show: false },
      })),
    ],
    axes: [
      last
        ? axis(X_LABEL[xAxis], { size: 40 })
        : axis("", { size: 6, labelSize: 0, values: (_, splits) => splits.map(() => "") }),
      axis(`${channel.title} (${channel.unit})`, { size: 52 }),
    ],
    hooks: {
      setCursor: [
        (chart: uPlot) => {
          const index = chart.cursor.idx;
          onCursor(index === null || index === undefined ? null : (frame.x[index] ?? null));
        },
      ],
    },
  };
}

function ChannelChart(props: ChannelChartProps) {
  const { channel, frame, passes, xAxis, last, onCursor } = props;
  const options = useMemo(
    () => chartOptions({ channel, frame, passes, xAxis, last, onCursor }),
    [channel, frame, passes, xAxis, last, onCursor],
  );
  const data = useMemo<uPlot.AlignedData>(
    () => [
      frame.x,
      ...seriesOf(
        frame,
        passes.map((pass) => pass.trace),
        channel,
      ),
    ],
    [frame, passes, channel],
  );
  const names = passes.map((pass) => pass.label).join(", ");
  return (
    <UPlotChart
      label={`${channel.title} in ${channel.unit} against ${xAxis}: ${names}`}
      options={options}
      data={data}
      height={last ? 140 : 104}
    />
  );
}

function format(value: number): string {
  return Math.abs(value) >= 100 ? value.toFixed(0) : value.toFixed(1);
}

/** The value of every channel of every pass at the sample nearest the cursor. */
function Readout({ passes, frame, at }: { passes: ShownPass[]; frame: Frame; at: number | null }) {
  return (
    <table className="readout" aria-label="Values at the cursor">
      <thead>
        <tr>
          <th scope="col">Pass</th>
          {CHANNELS.map((channel) => (
            <th scope="col" key={channel.key} title={`${channel.title} (${channel.unit})`}>
              {channel.title}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {passes.map((pass, index) => {
          const xs = frame.xs[index] ?? [];
          const sample = at === null ? null : nearestIndex(xs, at);
          return (
            <tr key={pass.seq}>
              <th scope="row">
                <span className="swatch" style={{ background: pass.color }} />
                {pass.label}
              </th>
              {CHANNELS.map((channel) => (
                <td key={channel.key}>
                  {sample === null ? "–" : format(channel.value(pass.trace, sample))}
                </td>
              ))}
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

/** The overlaid passes, one chart per channel, with one cursor across all of them. */
export function TraceCharts({ passes, axis: xAxis, trackLength }: TraceChartsProps) {
  const frame = useMemo(
    () =>
      frameOf(
        passes.map((pass) => pass.trace),
        xAxis,
        trackLength,
      ),
    [passes, xAxis, trackLength],
  );
  const [at, setAt] = useState<number | null>(null);
  const unit = xAxis === "distance" ? "m" : "s";
  return (
    <div className="traces">
      <p className="readout-at" aria-live="off">
        {at === null ? "Move over a chart to read the values." : `At ${format(at)} ${unit}`}
      </p>
      <Readout passes={passes} frame={frame} at={at} />
      <div className="trace-charts">
        {CHANNELS.map((channel, index) => (
          <ChannelChart
            key={channel.key}
            channel={channel}
            frame={frame}
            passes={passes}
            xAxis={xAxis}
            last={index === CHANNELS.length - 1}
            onCursor={setAt}
          />
        ))}
      </div>
    </div>
  );
}
