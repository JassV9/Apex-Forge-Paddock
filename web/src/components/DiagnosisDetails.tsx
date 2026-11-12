import type { Diagnosis, Evidence, Hypothesis } from "../api/client";
import { proposeHref } from "../hooks/useRoute";

const WRITER: Record<Diagnosis["explanation"]["provider"], string> = {
  none: "Written from templates, without a language model.",
  gemini: "Written by Gemini from the evidence below, and checked against it.",
  openrouter: "Written by a model on OpenRouter from the evidence below, and checked against it.",
};

const ALTERNATIVE: Record<Hypothesis["alternatives"][number]["kind"], string> = {
  driver_input: "Driver input",
  conditions: "Conditions",
};

function clicksText(clicks: number): string {
  const size = Math.abs(clicks);
  return `${clicks < 0 ? "−" : "+"}${String(size)} ${size === 1 ? "click" : "clicks"}`;
}

/** The diagnosis in words, who wrote them, and why a model's reply was thrown away. */
export function ExplanationSection({ diagnosis }: { diagnosis: Diagnosis }) {
  const explanation = diagnosis.explanation;
  return (
    <section aria-label="Explanation">
      <h3>In words</h3>
      <p>{explanation.summary}</p>
      <p className="note">{WRITER[explanation.provider]}</p>
      {explanation.rejected !== "" && <p role="status">{explanation.rejected}</p>}
    </section>
  );
}

function EvidenceList({ ids, pack }: { ids: string[]; pack: Map<string, Evidence> }) {
  return (
    <ul className="evidence">
      {ids.map((id) => {
        const reading = pack.get(id);
        return (
          <li key={id}>
            <code>{id}</code>
            {reading !== undefined &&
              `: ${reading.value.toFixed(2)} over ${String(reading.passes)} passes`}
          </li>
        );
      })}
    </ul>
  );
}

function Alternatives({
  hypothesis,
  pack,
}: {
  hypothesis: Hypothesis;
  pack: Map<string, Evidence>;
}) {
  return (
    <>
      <h4>What else could explain it</h4>
      <ul>
        {hypothesis.alternatives.map((alternative) => (
          <li key={alternative.kind}>
            <strong>{ALTERNATIVE[alternative.kind]}</strong>
            {alternative.supported && <span className="badge">the data speaks for this</span>}
            <p>{alternative.statement}</p>
            <EvidenceList ids={alternative.evidence} pack={pack} />
          </li>
        ))}
      </ul>
    </>
  );
}

function Candidates({ hypothesis, sessionId }: { hypothesis: Hypothesis; sessionId: number }) {
  return (
    <>
      <h4>Experiments that would test it</h4>
      {hypothesis.candidates.length === 0 && <p>None on this car's setup.</p>}
      <ul>
        {hypothesis.candidates.map((candidate) => (
          <li key={candidate.parameter}>
            {candidate.label === "" ? candidate.parameter : candidate.label}{" "}
            {clicksText(candidate.clicks)}: {candidate.why} (a rule of thumb, not a measurement).{" "}
            <a
              href={proposeHref({
                sessionId,
                parameter: candidate.parameter,
                clicks: candidate.clicks,
                hypothesisKey: hypothesis.key,
                statement: `${hypothesis.statement} Test: ${candidate.why}.`,
              })}
            >
              Test this
            </a>
          </li>
        ))}
      </ul>
      {hypothesis.not_testable.length > 0 && (
        <details>
          <summary>Not testable here ({hypothesis.not_testable.length})</summary>
          <ul>
            {hypothesis.not_testable.map((reason) => (
              <li key={reason}>{reason}</li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}

interface HypothesisCardProps {
  hypothesis: Hypothesis;
  pack: Map<string, Evidence>;
  sessionId: number;
}

function HypothesisCard({ hypothesis, pack, sessionId }: HypothesisCardProps) {
  return (
    <li aria-label={hypothesis.key}>
      <h3>{hypothesis.statement}</h3>
      <EvidenceList ids={hypothesis.evidence} pack={pack} />
      <Alternatives hypothesis={hypothesis} pack={pack} />
      <Candidates hypothesis={hypothesis} sessionId={sessionId} />
    </li>
  );
}

/** The hypotheses, strongest first, each with its evidence, alternatives and experiments. */
export function HypothesesSection({
  diagnosis,
  sessionId,
}: {
  diagnosis: Diagnosis;
  sessionId: number;
}) {
  const pack = new Map(diagnosis.evidence.map((reading) => [reading.id, reading]));
  if (diagnosis.hypotheses.length === 0) return null;
  return (
    <section aria-label="Hypotheses">
      <p>Hypotheses, strongest first. None is a verdict until an experiment has tested it.</p>
      <ol className="hypotheses">
        {diagnosis.hypotheses.map((hypothesis) => (
          <HypothesisCard
            key={hypothesis.key}
            hypothesis={hypothesis}
            pack={pack}
            sessionId={sessionId}
          />
        ))}
      </ol>
    </section>
  );
}
