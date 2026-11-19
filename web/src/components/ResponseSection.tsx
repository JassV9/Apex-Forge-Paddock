import type { Experiment } from "../api/client";

type Response = NonNullable<Experiment["responses"]>[number];

/** What each response finding says, in plain words (docs/adr/0034-vehicle-response-in-verdicts.md). */
export const RESPONSE_FINDING_LABEL: Record<Response["finding"], string> = {
  increase: "went up",
  decrease: "went down",
  neutral: "no meaningful change",
  likely_increase: "probably up, not by the margin",
  likely_decrease: "probably down, not by the margin",
  insufficient: "unclear",
};

const EXPECTED_LABEL: Record<Response["expected"], string> = {
  direct: "direct",
  proxy: "proxy",
};

/** A small signed number with three significant digits: "+0.108", "−0.0140". */
function signedSmall(value: number): string {
  const sign = value > 0 ? "+" : value < 0 ? "−" : "";
  return sign + Math.abs(value).toPrecision(3);
}

function intervalText(response: Response): string {
  return `${signedSmall(response.estimate)} [${signedSmall(response.ci_low)}, ${signedSmall(response.ci_high)}]`;
}

/**
 * What the car did: the cells the changed setting is expected to move, each with its own
 * finding, apart from the time verdict. Direct measurements and proxies are labelled.
 */
export function ResponseSection({ experiment }: { experiment: Experiment }) {
  const responses = experiment.responses ?? [];
  if (responses.length === 0) return null;
  return (
    <section aria-label="What the car did">
      <h3>What the car did: variant minus baseline</h3>
      <p>
        The channels this setting is expected to move, judged apart from the time. Holm across them
        keeps a false claim of a change to 5 %; a finding here never decides the experiment.
      </p>
      <table>
        <thead>
          <tr>
            <th scope="col">Channel</th>
            <th scope="col">Phase</th>
            <th scope="col">Corners</th>
            <th scope="col">Kind</th>
            <th scope="col">Effect and interval</th>
            <th scope="col">Margin</th>
            <th scope="col">Finding</th>
          </tr>
        </thead>
        <tbody>
          {responses.map((response) => (
            <tr key={`${response.metric}-${response.phase}-${response.regime}`}>
              <td>
                <code>{response.metric}</code>
              </td>
              <td>{response.phase}</td>
              <td>{response.regime}</td>
              <td>{EXPECTED_LABEL[response.expected]}</td>
              <td>{intervalText(response)}</td>
              <td>±{response.margin}</td>
              <td>{RESPONSE_FINDING_LABEL[response.finding]}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
