import { fetchScorecard, type Scorecard as Card } from "../api/client";
import { useLoaded } from "../hooks/useLoaded";

interface ScorecardProps {
  /** Injected in tests; the real client by default. */
  loadScorecard?: () => Promise<Card>;
}

const SOURCE: Record<Card["by_source"][number]["source"], string> = {
  rules: "Diagnosis rules",
  llm: "Language model",
  human: "Driver",
};

function hitText(hit: number, total: number): string {
  if (total === 0) return "none scored yet";
  return `${String(hit)} of ${String(total)} (${String(Math.round((100 * hit) / total))} %)`;
}

/** How often the predictions written down before an experiment came true. */
export function Scorecard({ loadScorecard = fetchScorecard }: ScorecardProps) {
  const state = useLoaded(loadScorecard);

  if (state.kind === "loading") return null;
  if (state.kind === "failed") return <p role="alert">The scorecard could not be loaded.</p>;
  const card = state.value;
  if (card.experiments === 0) return null;
  return (
    <section aria-label="Scorecard">
      <p>
        Predictions that came true:{" "}
        <strong>{hitText(card.predictions_hit, card.predictions_total)}</strong>, over {card.judged}{" "}
        judged of {card.experiments} experiments.
      </p>
      <table>
        <thead>
          <tr>
            <th scope="col">Hypotheses from</th>
            <th scope="col">Experiments</th>
            <th scope="col">Predictions that came true</th>
          </tr>
        </thead>
        <tbody>
          {card.by_source.map((score) => (
            <tr key={score.source}>
              <td>{SOURCE[score.source]}</td>
              <td>{score.experiments}</td>
              <td>{hitText(score.predictions_hit, score.predictions_total)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
