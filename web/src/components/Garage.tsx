import { fetchSensitivity, type SensitivityProfile } from "../api/client";
import { useLoaded } from "../hooks/useLoaded";
import { EffectRow, PriorRow, scales } from "./GarageRows";

interface GarageProps {
  /** The car of the session on screen; only its profile is shown. */
  carName: string;
  /** Injected in tests; the real client by default. */
  loadProfiles?: () => Promise<SensitivityProfile[]>;
}

function Measured({ profile, scale }: { profile: SensitivityProfile; scale: Map<string, number> }) {
  if (profile.effects.length === 0) {
    return <p>Nothing measured yet. Finish an experiment that changes one item on this car.</p>;
  }
  return (
    <table>
      <thead>
        <tr>
          <th scope="col">Change</th>
          <th scope="col">Channel</th>
          <th scope="col">Measured under</th>
          <th scope="col">Effect per corner pass</th>
          <th scope="col">Interval</th>
          <th scope="col">Knowledge</th>
          <th scope="col">Provenance</th>
        </tr>
      </thead>
      <tbody>
        {profile.effects.map((effect) => (
          <EffectRow
            key={`${effect.parameter} ${String(effect.clicks)} ${effect.channel} ${effect.corner_class} ${effect.phase} ${effect.track} ${effect.driver} ${effect.hint_bucket} ${String(effect.baseline_value)}`}
            effect={effect}
            label={profile.labels[effect.parameter] ?? effect.parameter}
            scale={scale.get(effect.channel) ?? 0}
          />
        ))}
      </tbody>
    </table>
  );
}

/** Other cars' effects, apart and labelled: a reason to test, never a result. */
function Priors({ profile, scale }: { profile: SensitivityProfile; scale: Map<string, number> }) {
  if (profile.priors.length === 0) return null;
  return (
    <section aria-label="Priors from other cars">
      <h4>Priors from other cars (not measured on this car)</h4>
      <table>
        <thead>
          <tr>
            <th scope="col">Change</th>
            <th scope="col">Channel</th>
            <th scope="col">Prior effect</th>
            <th scope="col">Interval</th>
            <th scope="col">Source</th>
          </tr>
        </thead>
        <tbody>
          {profile.priors.map((prior) => (
            <PriorRow
              key={`${prior.parameter} ${String(prior.clicks)} ${prior.channel} ${prior.corner_class} ${prior.phase}`}
              prior={prior}
              label={profile.labels[prior.parameter] ?? prior.parameter}
              scale={scale.get(prior.channel) ?? 0}
            />
          ))}
        </tbody>
      </table>
    </section>
  );
}

function Excluded({ profile }: { profile: SensitivityProfile }) {
  if (profile.excluded.length === 0) return null;
  return (
    <ul className="note" aria-label="Experiments not pooled">
      {profile.excluded.map((entry) => (
        <li key={`${String(entry.experiment_id)} ${entry.kind}`}>
          Experiment {entry.experiment_id} not pooled ({entry.kind}): {entry.reason}
        </li>
      ))}
    </ul>
  );
}

function CarProfile({ profile }: { profile: SensitivityProfile }) {
  const scale = scales([...profile.effects, ...profile.priors]);
  const judged = `${String(profile.experiments)} judged ${profile.experiments === 1 ? "experiment" : "experiments"}`;
  return (
    <section aria-label={profile.car_name}>
      <h3>{profile.car_name}</h3>
      <p className="note">
        {judged}. Method: <code>{profile.model_version}</code>.
      </p>
      <Measured profile={profile} scale={scale} />
      <Excluded profile={profile} />
      <Priors profile={profile} scale={scale} />
    </section>
  );
}

/** What each setup item does on each car, with how sure the ledger is about it. */
export function Garage({ carName, loadProfiles = fetchSensitivity }: GarageProps) {
  const state = useLoaded(loadProfiles);

  if (state.kind === "loading") return <p>Loading the garage…</p>;
  if (state.kind === "failed") return <p role="alert">The garage could not be loaded.</p>;
  const profile = state.value.find((candidate) => candidate.car_name === carName);
  if (profile === undefined) return <p className="empty">Core has no profile of this car yet.</p>;
  return (
    <>
      <p className="note">
        Effects are the variant minus the baseline, pooled from this car&apos;s own experiments per
        track, driver and AI hint, with a 90 % interval. Confirmed and neutral need at least three
        experiments. An effect borrowed from another car is a prior, listed apart: a reason to test,
        never a result. What a change is expected to move in general (the parameter-to-observable
        map) is a hypothesis and is never shown here as a finding.
      </p>
      <CarProfile profile={profile} />
    </>
  );
}
