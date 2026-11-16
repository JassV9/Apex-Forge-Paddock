import createClient from "openapi-fetch";

import type { components, paths } from "./launcher-schema";

export type LauncherContent = components["schemas"]["LauncherContent"];
export type LauncherCar = components["schemas"]["LauncherCar"];
export type LauncherTrack = components["schemas"]["LauncherTrack"];
export type LauncherLayout = components["schemas"]["LauncherLayout"];
export type LaunchRequest = components["schemas"]["LaunchRequest"];
export type LauncherStatus = components["schemas"]["LauncherStatus"];
export type ControlsCheck = components["schemas"]["ControlsCheck"];

/**
 * paddock-launcher runs on the Windows host, not behind the dev server's /api proxy (that
 * lives in Docker, which cannot start the game), so the browser calls it directly.
 */
export const LAUNCHER_URL =
  (import.meta.env.VITE_PADDOCK_LAUNCHER_URL as string | undefined) ?? "http://127.0.0.1:8090";

/** Typed client for the launcher; every path and body comes from docs/api/launcher.yaml.
 * fetch is looked up per call so tests can replace it. */
const launcherApi = createClient<paths>({
  baseUrl: LAUNCHER_URL,
  fetch: (request) => globalThis.fetch(request),
});

function unwrap<T>(reply: { data?: T; error?: unknown }, request: string): T {
  if (reply.data !== undefined) return reply.data;
  const problem = reply.error as { error?: unknown } | undefined;
  const said = typeof problem?.error === "string" ? problem.error : "";
  throw new Error(said === "" ? `the launcher did not answer ${request}` : said);
}

export async function fetchLauncherStatus(): Promise<LauncherStatus> {
  return unwrap(await launcherApi.GET("/status"), "GET /status");
}

export async function fetchLauncherContent(): Promise<LauncherContent> {
  return unwrap(await launcherApi.GET("/content"), "GET /content");
}

/** Asks the launcher to start Practice from the pits; it refuses while the game runs. */
export async function startDriving(body: LaunchRequest): Promise<LauncherStatus> {
  return unwrap(await launcherApi.POST("/launch", { body }), "POST /launch");
}
