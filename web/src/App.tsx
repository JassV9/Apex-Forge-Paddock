import { CoreStatus } from "./components/CoreStatus";
import { DiagnosisPage } from "./components/DiagnosisPage";
import { ExperimentPage } from "./components/ExperimentPage";
import { Garage } from "./components/Garage";
import { Ledger } from "./components/Ledger";
import { ProposeForm } from "./components/ProposeForm";
import { Scorecard } from "./components/Scorecard";
import { SessionList } from "./components/SessionList";
import { experimentHref, type Route, useRoute } from "./hooks/useRoute";

function Page({ route }: { route: Route }) {
  switch (route.page) {
    case "sessions":
      return (
        <>
          <h2>Sessions</h2>
          <SessionList />
        </>
      );
    case "session":
      return <DiagnosisPage key={route.id} sessionId={route.id} />;
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

export function App() {
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
          <a href="#/propose">Propose</a>
        </nav>
        <CoreStatus />
      </header>
      <Page route={route} />
    </main>
  );
}
