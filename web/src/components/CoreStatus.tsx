import { useEffect, useState } from "react";

import { fetchCoreHealth, type Health } from "../api/client";

type State = { kind: "loading" } | { kind: "ready"; health: Health } | { kind: "unreachable" };

interface CoreStatusProps {
  /** Injected in tests; the real client by default. */
  fetchHealth?: () => Promise<Health>;
}

function describe(state: State): string {
  switch (state.kind) {
    case "loading":
      return "Checking core…";
    case "unreachable":
      return "Core is not reachable. Is `make dev` running?";
    case "ready":
      return state.health.status === "ok"
        ? `Core ${state.health.version} is running.`
        : `Core ${state.health.version} is running, but its database is unavailable.`;
  }
}

/** Shows whether the dashboard can reach core. It reports core's answer and decides nothing. */
export function CoreStatus({ fetchHealth = fetchCoreHealth }: CoreStatusProps) {
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    fetchHealth().then(
      (health) => {
        if (!cancelled) setState({ kind: "ready", health });
      },
      () => {
        if (!cancelled) setState({ kind: "unreachable" });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [fetchHealth]);

  return <p role="status">{describe(state)}</p>;
}
