import { CoreStatus } from "./components/CoreStatus";
import { DiagnosisPage } from "./components/DiagnosisPage";
import { ExperimentPage } from "./components/ExperimentPage";
import { Garage } from "./components/Garage";
import { Ledger } from "./components/Ledger";
import { ProposeForm } from "./components/ProposeForm";
import { Scorecard } from "./components/Scorecard";
import { SessionList } from "./components/SessionList";
import { READ_ONLY } from "./config";
import { experimentHref, type Route, useRoute } from "./hooks/useRoute";

function Page({ route, readOnly }: { route: Route; readOnly: boolean }) {
  switch (route.page) {
    case "sessions":
      return (
        <>
          <h2>Sessions</h2>
          <SessionList />
        </>
      );
    case "session":
      return <DiagnosisPage key={route.id} sessionId={route.id} readOnly={readOnly} />;
    case "ledger":
      return (
        <>
          <h2>Ledger</h2>
          <Scorecard />
          <Ledger />
        </>
      );
    case "garage":
      return (
        <>
          <h2>Garage</h2>
          <Garage />
        </>
      );
    case "propose":
      if (readOnly) return <p>Experiments cannot be proposed in the read-only demo.</p>;
      return (
        <>
          <h2>Propose an experiment</h2>
          <ProposeForm
            key={window.location.hash}
            {...(route.prefill === undefined ? {} : { prefill: route.prefill })}
            onProposed={(experiment) => {
              window.location.hash = experimentHref(experiment.id);
            }}
          />
        </>
      );
    case "experiment":
      return <ExperimentPage key={route.id} id={route.id} />;
  }
}

/** `readOnly` is the public demo: recorded data, and nothing a visitor can change. */
export function App({ readOnly = READ_ONLY }: { readOnly?: boolean }) {
  const route = useRoute();
  return (
    <main>
      <header>
        <h1>Apex Forge: Paddock</h1>
        <p>Every setup change is an experiment.</p>
        <nav aria-label="Pages">
          <a href="#/">Sessions</a>
          <a href="#/experiments">Ledger</a>
          <a href="#/garage">Garage</a>
          {!readOnly && <a href="#/propose">Propose</a>}
        </nav>
        <CoreStatus />
        {readOnly && (
          <p role="note">
            Read-only demo on a recorded session. Proposing and running experiments needs the game
            and a local install.
          </p>
        )}
      </header>
      <Page route={route} readOnly={readOnly} />
    </main>
  );
}
