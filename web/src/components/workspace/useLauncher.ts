import { useEffect, useState } from "react";

import {
  fetchLauncherContent,
  fetchLauncherStatus,
  type LauncherContent,
  type LauncherStatus,
  type LaunchRequest,
  startDriving,
} from "../../api/launcher";

/** The launcher calls the Drive panel makes; injected in tests, the real client by default. */
export interface LauncherApi {
  status: () => Promise<LauncherStatus>;
  content: () => Promise<LauncherContent>;
  launch: (body: LaunchRequest) => Promise<LauncherStatus>;
}

export const launcherClient: LauncherApi = {
  status: fetchLauncherStatus,
  content: fetchLauncherContent,
  launch: startDriving,
};

export type Reach =
  { kind: "looking" } | { kind: "unreachable" } | { kind: "ready"; status: LauncherStatus };

/** Polls the launcher's status (the next poll starts when the last answered) and loads the
 * installed content once the launcher answers. */
export function useLauncher(api: LauncherApi, pollMs: number) {
  const [reach, setReach] = useState<Reach>({ kind: "looking" });
  const [content, setContent] = useState<LauncherContent | undefined>(undefined);
  const reachable = reach.kind === "ready";

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = () => {
      void api
        .status()
        .then(
          (status) => {
            if (!cancelled) setReach({ kind: "ready", status });
          },
          () => {
            if (!cancelled) setReach({ kind: "unreachable" });
          },
        )
        .finally(() => {
          if (!cancelled) timer = setTimeout(poll, pollMs);
        });
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [api, pollMs]);

  useEffect(() => {
    if (!reachable || content !== undefined) return;
    let cancelled = false;
    api.content().then(
      (loaded) => {
        if (!cancelled) setContent(loaded);
      },
      () => undefined,
    );
    return () => {
      cancelled = true;
    };
  }, [api, reachable, content]);

  const showStatus = (status: LauncherStatus) => {
    setReach({ kind: "ready", status });
  };
  return { reach, content, showStatus };
}
