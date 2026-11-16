import { useMemo } from "react";
import type uPlot from "uplot";

import type { PassRecord } from "../../api/client";
import { axis, baseOptions, INK, TYRES } from "../charts/chartTheme";
import { UPlotChart } from "../charts/UPlotChart";

const TYRE_NAMES = ["FL", "FR", "RL", "RR"];

function line(label: string, stroke: string, scale = "c"): uPlot.Series {
  return { label, stroke, scale, width: 1.5, spanGaps: true, points: { show: false } };
}

const OPTIONS: Omit<uPlot.Options, "width" | "height"> = {
  ...baseOptions(),
  scales: { x: { time: false }, c: {}, l: {} },
  series: [
    {},
    ...TYRE_NAMES.map((name, i) => line(`Core ${name}`, TYRES[i] ?? INK.axis)),
    { ...line("Fuel", INK.axis, "l"), dash: [4, 4] },
  ],
  axes: [
    axis("Pass", { size: 36 }),
    axis("Tyre core (°C)", { scale: "c", size: 48 }),
    // uPlot's Side is a const enum, which isolated modules cannot name: 1 is the right side.
    // eslint-disable-next-line @typescript-eslint/no-unsafe-enum-assignment
    axis("Fuel (L)", { scale: "l", side: 1, size: 48, grid: { show: false } }),
  ],
};

/** The four tyre core temperatures and the fuel load at the end of each pass. */
export function ConditionsChart({ passes }: { passes: PassRecord[] }) {
  const data = useMemo<uPlot.AlignedData>(() => {
    const tyre = (i: number) => passes.map((pass) => pass.conditions?.tyre_core_c[i] ?? null);
    return [
      passes.map((pass) => pass.seq),
      tyre(0),
      tyre(1),
      tyre(2),
      tyre(3),
      passes.map((pass) => pass.conditions?.fuel_l ?? null),
    ];
  }, [passes]);
  if (passes.every((pass) => pass.conditions === undefined)) {
    return <p className="empty">No conditions were recorded in this session.</p>;
  }
  return (
    <figure className="conditions">
      <figcaption>
        Tyre core temperature
        {TYRE_NAMES.map((name, i) => (
          <span key={name} className="key">
            <span className="swatch" style={{ background: TYRES[i] }} />
            {name}
          </span>
        ))}
        <span className="key">
          <span className="swatch dashed" />
          fuel
        </span>
      </figcaption>
      <UPlotChart
        label="Tyre core temperatures FL, FR, RL, RR and fuel by pass"
        options={OPTIONS}
        data={data}
        height={180}
      />
    </figure>
  );
}
