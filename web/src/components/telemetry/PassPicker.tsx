import type { CornerSummary, PassRecord } from "../../api/client";
import { lapLabel, seconds, signedSeconds } from "../../format";

interface PassPickerProps {
  passes: PassRecord[];
  summary: CornerSummary | undefined;
  picked: number | undefined;
  shown: Set<number>;
  colors: Map<number, string>;
  hrefOf: (seq: number) => string;
  onToggle: (seq: number) => void;
}

function deltaText(pass: PassRecord, summary: CornerSummary | undefined): string {
  if (!pass.valid) return pass.invalid_reason ?? "invalid";
  if (summary?.best_s === undefined) return "";
  if (pass.seq === summary.best_seq) return "best";
  return signedSeconds(pass.duration_s - summary.best_s);
}

interface PassRowProps extends Omit<PassPickerProps, "passes" | "shown" | "colors"> {
  pass: PassRecord;
  shown: boolean;
  color: string | undefined;
}

function PassRow({ pass, summary, picked, shown, color, hrefOf, onToggle }: PassRowProps) {
  const isPicked = pass.seq === picked;
  const classes = [pass.seq === summary?.best_seq ? "is-best" : "", isPicked ? "is-picked" : ""];
  return (
    <tr className={classes.join(" ").trim() || undefined}>
      <td>
        <input
          type="checkbox"
          aria-label={`Overlay pass ${String(pass.seq)}`}
          checked={shown}
          onChange={() => {
            onToggle(pass.seq);
          }}
          style={{ accentColor: color }}
        />
      </td>
      <td>
        <a href={hrefOf(pass.seq)} aria-current={isPicked ? "true" : undefined}>
          {lapLabel(pass.lap)}
        </a>
      </td>
      <td className="num">{seconds(pass.duration_s)}</td>
      <td className={`num ${pass.valid ? "" : "bad"}`}>{deltaText(pass, summary)}</td>
    </tr>
  );
}

/** The corner's passes: tick one to overlay it, follow its lap to look at it closely. */
export function PassPicker({ passes, shown, colors, ...rest }: PassPickerProps) {
  return (
    <table className="pass-picker">
      <caption>Passes through this corner</caption>
      <thead>
        <tr>
          <th scope="col">Show</th>
          <th scope="col">Lap</th>
          <th scope="col">Zone time</th>
          <th scope="col">To best</th>
        </tr>
      </thead>
      <tbody>
        {passes.map((pass) => (
          <PassRow
            key={pass.seq}
            pass={pass}
            shown={shown.has(pass.seq)}
            color={colors.get(pass.seq)}
            {...rest}
          />
        ))}
      </tbody>
    </table>
  );
}
