import { fetchSessions, type SessionSummary } from "../api/client";
import { useLoaded } from "../hooks/useLoaded";
import { sessionHref } from "../hooks/useRoute";

interface SessionListProps {
  /** Injected in tests; the real client by default. */
  loadSessions?: () => Promise<SessionSummary[]>;
}

const SOURCE_LABEL: Record<SessionSummary["source"], string> = {
  human: "Driven",
  ai: "Game AI",
  ghost: "Ghost Driver",
  demo: "Demo data",
};

function formatStart(startedAt: string): string {
  return new Date(startedAt).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

function SessionRow({ session }: { session: SessionSummary }) {
  const track =
    session.layout === "" ? session.track_name : `${session.track_name} (${session.layout})`;
  return (
    <tr>
      <td>{formatStart(session.started_at)}</td>
      <td>{session.car_name}</td>
      <td>{track}</td>
      <td>{SOURCE_LABEL[session.source]}</td>
      <td>{session.passes}</td>
      <td>{session.corners}</td>
      <td>
        <a href={sessionHref(session.id)}>Diagnosis</a>
      </td>
    </tr>
  );
}

/** Recorded sessions as core lists them, newest first. */
export function SessionList({ loadSessions = fetchSessions }: SessionListProps) {
  const state = useLoaded(loadSessions);

  if (state.kind === "loading") return <p>Loading sessions…</p>;
  if (state.kind === "failed") return <p role="alert">Sessions could not be loaded.</p>;
  if (state.value.length === 0) {
    return <p>No sessions yet. Drive with the Paddock agent enabled, or run `make demo`.</p>;
  }
  return (
    <table>
      <thead>
        <tr>
          <th scope="col">Started</th>
          <th scope="col">Car</th>
          <th scope="col">Track</th>
          <th scope="col">Source</th>
          <th scope="col">Corner passes</th>
          <th scope="col">Corners</th>
          <th scope="col">What it says</th>
        </tr>
      </thead>
      <tbody>
        {state.value.map((session) => (
          <SessionRow key={session.id} session={session} />
        ))}
      </tbody>
    </table>
  );
}
