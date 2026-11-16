import "uplot/dist/uPlot.min.css";

import { useEffect, useRef } from "react";
import uPlot from "uplot";

export interface UPlotChartProps {
  /** What the chart shows, for screen readers; the numbers are in the tables beside it. */
  label: string;
  /** Everything but the size: the chart is as wide as its box. */
  options: Omit<uPlot.Options, "width" | "height">;
  data: uPlot.AlignedData;
  height: number;
}

/**
 * A uPlot chart that fills its box and follows its width (docs/adr/0013-telemetry-workspace.md).
 * It draws what it is given and computes nothing.
 */
export function UPlotChart({ label, options, data, height }: UPlotChartProps) {
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const box = host.current;
    if (box === null) return;
    const chart = new uPlot({ ...options, width: box.clientWidth, height }, data, box);
    const resize = new ResizeObserver(() => {
      chart.setSize({ width: box.clientWidth, height });
    });
    resize.observe(box);
    return () => {
      resize.disconnect();
      chart.destroy();
    };
  }, [options, data, height]);

  return <div className="chart" role="img" aria-label={label} ref={host} />;
}
