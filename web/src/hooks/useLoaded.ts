import { useEffect, useState } from "react";

export type Loaded<T> = { kind: "loading" } | { kind: "ready"; value: T } | { kind: "failed" };

/** Calls `load` once and reports loading, the value, or failure. */
export function useLoaded<T>(load: () => Promise<T>): Loaded<T> {
  const [state, setState] = useState<Loaded<T>>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    load().then(
      (value) => {
        if (!cancelled) setState({ kind: "ready", value });
      },
      () => {
        if (!cancelled) setState({ kind: "failed" });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [load]);

  return state;
}
