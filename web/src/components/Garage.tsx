import { fetchSensitivity, type SensitivityEffect, type SensitivityProfile } from "../api/client";
import { clicksText, signed } from "../experimentText";
import { useLoaded } from "../hooks/useLoaded";

interface GarageProps {
  /** Injected in tests; the real client by default. */
  loadProfiles?: () => Promise<SensitivityProfile[]>;
}

const READING: Record<SensitivityEffect["effect_class"], string> = {
  negative: "Lowers it",
  positive: "Raises it",
  negligible: "Negligible",
  inconclusive: "Inconclusive",
};

const BAR_WIDTH = 160;
const BAR_HALF = 76;

/** The largest bound of any effect on a channel: every bar of that channel shares one scale. */
function scales(effects: SensitivityEffect[]): Map<string, number> {
  const largest = new Map<string, number>();
  for (const effect of effects) {
    const reach = Math.max(Math.abs(effect.ci_low), Math.abs(effect.ci_high));
    largest.set(effect.channel, Math.max(largest.get(effect.channel) ?? 0, reach));
  }
  return largest;
}

/** The interval as a bar around zero. The numbers beside it say the same thing in text. */
function IntervalBar({ effect, scale }: { effect: SensitivityEffect; scale: number }) {
  const at = (value: number) => BAR_WIDTH / 2 + (scale === 0 ? 0 : (value / scale) * BAR_HALF);
  const kind = effect.is_prior ? "prior" : effect.effect_class;
  return (
    <svg className={`interval interval-${kind}`} width={BAR_WIDTH} height="16" aria-hidden="true">
      <line className="zero" x1={BAR_WIDTH / 2} x2={BAR_WIDTH / 2} y1="1" y2="15" />
      <line x1={at(effect.ci_low)} x2={at(effect.ci_high)} y1="8" y2="8" />
      <circle cx={at(effect.estimate)} cy="8" r="3.5" />
    </svg>
  );
}

/** Where the number comes from. A prior is never worded as a measurement. */
function basisText(effect: SensitivityEffect): string {
  if (effect.is_prior) return `Prior from ${effect.prior_source}, not measured on this car`;
  const count = effect.experiment_ids.length;
  const experiments = `${String(count)} ${count === 1 ? "experiment" : "experiments"}`;
  return `Measured: ${experiments}, ${String(effect.n_passes)} passes`;
}

interface EffectRowProps {
  effect: SensitivityEffect;
  label: string;
  scale: number;
}

function EffectRow({ effect, label, scale }: EffectRowProps) {
  return (
    <tr className={effect.is_prior ? "prior" : undefined}>
      <td>
        {label} {clicksText(effect.clicks)}
      </td>
      <td>
        <code>{effect.channel}</code>, {effect.corner_class} corners
      </td>
      <td>
        {signed(effect.estimate)} [{signed(effect.ci_low)}, {signed(effect.ci_high)}]
      </td>
      <td>
        <IntervalBar effect={effect} scale={scale} />
      </td>
      <td>
        {READING[effect.effect_class]}
        {effect.nonlinear && <span className="badge">non-linear</span>}
      </td>
      <td>
        {basisText(effect)}
        {effect.nonlinear && <p className="note">{effect.note}</p>}
      </td>
    </tr>
  );
}

function CarProfile({ profile }: { profile: SensitivityProfile }) {
  const scale = scales(profile.effects);
  const judged = `${String(profile.experiments)} judged ${profile.experiments === 1 ? "experiment" : "experiments"}`;
  return (
    <section aria-label={profile.car_name}>
      <h3>{profile.car_name}</h3>
      <p className="note">
        {judged}. Method: <code>{profile.model_version}</code>.
      </p>
      {profile.effects.length === 0 ? (
        <p>Nothing measured yet. Finish an experiment that changes one item on this car.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th scope="col">Change</th>
              <th scope="col">Channel</th>
              <th scope="col">Effect per corner pass</th>
              <th scope="col">Interval</th>
              <th scope="col">Reading</th>
              <th scope="col">Basis</th>
            </tr>
          </thead>
          <tbody>
            {profile.effects.map((effect) => (
              <EffectRow
                key={`${effect.parameter} ${String(effect.clicks)} ${effect.channel} ${effect.corner_class}`}
                effect={effect}
                label={profile.labels[effect.parameter] ?? effect.parameter}
                scale={scale.get(effect.channel) ?? 0}
              />
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

/** What each setup item does on each car, with how sure the ledger is about it. */
export function Garage({ loadProfiles = fetchSensitivity }: GarageProps) {
  const state = useLoaded(loadProfiles);

  if (state.kind === "loading") return <p>Loading the garage…</p>;
  if (state.kind === "failed") return <p role="alert">The garage could not be loaded.</p>;
  if (state.value.length === 0) return <p>No cars yet. Record a session first.</p>;
  return (
    <>
      <p>
        Effects are the variant minus the baseline, with a 90 % interval. An effect borrowed from
        another car is a prior: a reason to test, never a result.
      </p>
      {state.value.map((profile) => (
        <CarProfile key={profile.car_id} profile={profile} />
      ))}
    </>
  );
}
