import { fetchCoreHealth, type Health } from "../api/client";
import { type Loaded, useLoaded } from "../hooks/useLoaded";

interface CoreStatusProps {
  /** Injected in tests; the real client by default. */
  fetchHealth?: () => Promise<Health>;
}

function describe(state: Loaded<Health>): string {
  switch (state.kind) {
    case "loading":
      return "Checking core…";
    case "failed":
      return "Core is not reachable. Is `make dev` running?";
    case "ready":
      return state.value.status === "ok"
        ? `Core ${state.value.version} is running.`
        : `Core ${state.value.version} is running, but its database is unavailable.`;
  }
}

/** Shows whether the dashboard can reach core. It reports core's answer and decides nothing. */
export function CoreStatus({ fetchHealth = fetchCoreHealth }: CoreStatusProps) {
  return <p role="status">{describe(useLoaded(fetchHealth))}</p>;
}
