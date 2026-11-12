import { CoreStatus } from "./components/CoreStatus";
import { ExperimentPage } from "./components/ExperimentPage";
import { Ledger } from "./components/Ledger";
import { ProposeForm } from "./components/ProposeForm";
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
    case "ledger":
      return (
        <>
          <h2>Ledger</h2>
          <Ledger />
        </>
      );
    case "propose":
      return (
        <>
          <h2>Propose an experiment</h2>
          <ProposeForm
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
          <a href="#/propose">Propose</a>
        </nav>
        <CoreStatus />
      </header>
      <Page route={route} />
    </main>
  );
}
