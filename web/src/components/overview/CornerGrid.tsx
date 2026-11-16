import type { CornerSummary, PassRecord } from "../../api/client";
import { lapLabel, seconds } from "../../format";
import { telemetryHref } from "../../hooks/useRoute";
import { lapGrid, toneOf } from "./overviewData";

interface CornerGridProps {
  sessionId: number;
  corners: CornerSummary[];
  passes: PassRecord[];
}

function cornerName(corner: CornerSummary): string {
  return corner.name === "" ? `C${String(corner.corner)}` : corner.name;
}

function cellTitle(pass: PassRecord): string {
  const base = `Pass ${String(pass.seq)}, ${seconds(pass.duration_s)} s`;
  return pass.valid ? base : `${base}, not counted: ${pass.invalid_reason ?? "invalid"}`;
}

function Cell({
  sessionId,
  pass,
  summary,
}: {
  sessionId: number;
  pass: PassRecord | undefined;
  summary: CornerSummary;
}) {
  if (pass === undefined) return <td className="cell empty-cell" />;
  return (
    <td className={`cell tone-${toneOf(pass, summary)}`}>
      <a href={telemetryHref(sessionId, pass.corner, pass.seq)} title={cellTitle(pass)}>
        {seconds(pass.duration_s)}
      </a>
    </td>
  );
}

function SummaryRow({
  label,
  corners,
  value,
}: {
  label: string;
  corners: CornerSummary[];
  value: (corner: CornerSummary) => number | undefined;
}) {
  return (
    <tr className="summary-row">
      <th scope="row">{label}</th>
      {corners.map((corner) => {
        const number = value(corner);
        return (
          <td key={corner.corner} className="num">
            {number === undefined ? "–" : seconds(number)}
          </td>
        );
      })}
    </tr>
  );
}

/** Zone times, laps down and corners across, like a timing screen; a cell opens the pass. */
export function CornerGrid({ sessionId, corners, passes }: CornerGridProps) {
  const rows = lapGrid(passes);
  return (
    <div className="grid-scroll" tabIndex={0} role="region" aria-label="Zone times by lap">
      <table className="corner-grid">
        <thead>
          <tr>
            <th scope="col">Lap</th>
            {corners.map((corner) => (
              <th scope="col" key={corner.corner}>
                <a href={telemetryHref(sessionId, corner.corner)}>{cornerName(corner)}</a>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.lap ?? "unknown"}>
              <th scope="row">{lapLabel(row.lap)}</th>
              {corners.map((corner) => (
                <Cell
                  key={corner.corner}
                  sessionId={sessionId}
                  pass={row.byCorner.get(corner.corner)}
                  summary={corner}
                />
              ))}
            </tr>
          ))}
        </tbody>
        <tfoot>
          <SummaryRow label="Best" corners={corners} value={(c) => c.best_s} />
          <SummaryRow label="Median" corners={corners} value={(c) => c.median_s} />
          <SummaryRow label="Spread σ" corners={corners} value={(c) => c.spread_s} />
        </tfoot>
      </table>
    </div>
  );
}
